package ui

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
)

func (uis *UIServer) loginPage(w http.ResponseWriter, r *http.Request) {
	if uis.UserManager.IsRedirect() {
		http.Redirect(w, r, "/login/redirect", http.StatusFound)
	}
	uis.WriteHTML(w, http.StatusOK, nil, "base", "login.html", "base_angular.html")
}

func (uis *UIServer) setLoginToken(token string, w http.ResponseWriter) {
	authTokenCookie := &http.Cookie{
		Name:     evergreen.AuthTokenCookie,
		Value:    token,
		HttpOnly: true,
		Secure:   uis.Settings.Ui.SecureCookies,
		Path:     "/",
	}
	http.SetCookie(w, authTokenCookie)
}

func clearSession(w http.ResponseWriter) {
	authTokenCookie := &http.Cookie{
		Name:   evergreen.AuthTokenCookie,
		Value:  "",
		MaxAge: -1,
		Path:   "/",
	}
	http.SetCookie(w, authTokenCookie)
}

func (uis *UIServer) login(w http.ResponseWriter, r *http.Request) {
	creds := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{}

	if err := util.ReadJSONInto(r.Body, &creds); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if creds.Username == "" || creds.Password == "" {
		http.Error(w, fmt.Sprintf("Username and password are required"), http.StatusBadRequest)
		return
	}

	token, err := uis.UserManager.CreateUserToken(creds.Username, creds.Password)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid username/password: %v", err), http.StatusUnauthorized)
		return
	}
	uis.setLoginToken(token, w)
	uis.WriteJSON(w, http.StatusOK, map[string]string{})
}

func (uis *UIServer) logout(w http.ResponseWriter, r *http.Request) {
	clearSession(w)
	loginURL := fmt.Sprintf("%v/login", uis.RootURL)
	http.Redirect(w, r, loginURL, http.StatusFound)
}

func (uis *UIServer) newAPIKey(w http.ResponseWriter, r *http.Request) {
	currentUser := MustHaveUser(r)
	newKey := util.RandomString()
	if err := model.SetUserAPIKey(currentUser.Id, newKey); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("failed saving key: %v", err))
		return
	}
	uis.WriteJSON(w, http.StatusOK, struct {
		Key string `json:"key"`
	}{newKey})
}

func (uis *UIServer) userSettingsPage(w http.ResponseWriter, r *http.Request) {
	currentUser := MustHaveUser(r)
	projCtx := MustHaveProjectContext(r)

	settingsData := currentUser.Settings
	flashes := PopFlashes(uis.CookieStore, r, w)

	type confFile struct {
		User    string `json:"user"`
		APIKey  string `json:"api_key"`
		APIHost string `json:"api_server_host"`
		UIHost  string `json:"ui_server_host"`
	}
	exampleConf := confFile{currentUser.Id, currentUser.APIKey, uis.Settings.ApiUrl + "/api", uis.Settings.Ui.Url}

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		Data        user.UserSettings
		User        *user.DBUser
		Config      confFile
		Binaries    []evergreen.ClientBinary
		Flashes     []interface{}
	}{projCtx, settingsData, currentUser, exampleConf,
		uis.Settings.Api.Clients.ClientBinaries, flashes},
		"base", "settings.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) userSettingsModify(w http.ResponseWriter, r *http.Request) {
	currentUser := MustHaveUser(r)
	userSettings := user.UserSettings{}

	if err := util.ReadJSONInto(r.Body, &userSettings); err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	if err := model.SaveUserSettings(currentUser.Username(), userSettings); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error saving user settings: %v", err))
		return
	}

	PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Settings were saved."))
	uis.WriteJSON(w, http.StatusOK, "Updated user settings successfully")
}
