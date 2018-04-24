package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
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

	if err := util.ReadJSONInto(util.NewRequestReader(r), &creds); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if creds.Username == "" || creds.Password == "" {
		http.Error(w, fmt.Sprintf("Username and password are required"), http.StatusBadRequest)
		return
	}

	token, err := uis.UserManager.CreateUserToken(creds.Username, creds.Password)
	if err != nil {
		http.Error(w, "Invalid username/password", http.StatusUnauthorized)
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
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "failed saving key"))
		return
	}
	uis.WriteJSON(w, http.StatusOK, struct {
		Key string `json:"key"`
	}{newKey})
}

func (uis *UIServer) userSettingsPage(w http.ResponseWriter, r *http.Request) {
	currentUser := MustHaveUser(r)
	settingsData := currentUser.Settings

	type confFile struct {
		User    string `json:"user"`
		APIKey  string `json:"api_key"`
		APIHost string `json:"api_server_host"`
		UIHost  string `json:"ui_server_host"`
	}
	exampleConf := confFile{currentUser.Id, currentUser.APIKey, uis.Settings.ApiUrl + "/api", uis.Settings.Ui.Url}

	uis.WriteHTML(w, http.StatusOK, struct {
		Data       user.UserSettings
		Config     confFile
		Binaries   []evergreen.ClientBinary
		GithubUser string
		GithubUID  int
		ViewData
	}{settingsData, exampleConf, uis.clientConfig.ClientBinaries, currentUser.Settings.GithubUser.LastKnownAs, currentUser.Settings.GithubUser.UID, uis.GetCommonViewData(w, r, true, true)},
		"base", "settings.html", "base_angular.html", "menu.html")
}

// TODO: remove this once the UI changes to use the restV2 version
func (uis *UIServer) userSettingsModify(w http.ResponseWriter, r *http.Request) {
	currentUser := MustHaveUser(r)
	userSettings := user.UserSettings{}

	if err := util.ReadJSONInto(util.NewRequestReader(r), &userSettings); err != nil {
		err = errors.Wrap(err, "JSON is invalid")
		uis.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	if len(userSettings.GithubUser.LastKnownAs) == 0 {
		userSettings.GithubUser = user.GithubUser{}

	} else if currentUser.Settings.GithubUser.LastKnownAs != userSettings.GithubUser.LastKnownAs {
		token, err := uis.Settings.GetGithubOauthToken()
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrap(err, "Error fetching user from Github"))
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		ghUser, err := thirdparty.GetGithubUser(ctx, token, userSettings.GithubUser.LastKnownAs)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrap(err, "Error fetching user from Github"))
			return
		}

		userSettings.GithubUser.LastKnownAs = *ghUser.Login
		userSettings.GithubUser.UID = *ghUser.ID

	} else {
		userSettings.GithubUser.UID = currentUser.Settings.GithubUser.UID
	}

	if err := model.SaveUserSettings(currentUser.Username(), userSettings); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrap(err, "Error saving user settings"))
		return
	}

	PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Settings were saved."))
	uis.WriteJSON(w, http.StatusOK, "Updated user settings successfully")
}
