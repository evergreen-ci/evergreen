package service

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func (uis *UIServer) loginPage(w http.ResponseWriter, r *http.Request) {
	if uis.UserManager.IsRedirect() {
		http.Redirect(w, r, "/login/redirect", http.StatusFound)
	}
	uis.render.WriteResponse(w, http.StatusOK, nil, "base", "login.html", "base_angular.html")
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
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error creating user token",
		}))
		http.Error(w, "Invalid username/password", http.StatusUnauthorized)
		return
	}

	uis.umconf.AttachCookie(token, w)
	gimlet.WriteJSON(w, map[string]string{})
}

func (uis *UIServer) logout(w http.ResponseWriter, r *http.Request) {
	uis.umconf.ClearCookie(w)
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
	gimlet.WriteJSON(w, struct {
		Key string `json:"key"`
	}{newKey})
}

func (uis *UIServer) clearUserToken(w http.ResponseWriter, r *http.Request) {
	u := MustHaveUser(r)
	if err := uis.UserManager.ClearUser(u, false); err != nil {
		gimlet.WriteJSONInternalError(w, struct {
			Error string `json:"error"`
		}{Error: err.Error()})
	} else {
		gimlet.WriteJSON(w, map[string]string{})
	}
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

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Data       user.UserSettings
		Config     confFile
		Binaries   []evergreen.ClientBinary
		GithubUser string
		GithubUID  int
		AuthIsLDAP bool
		ViewData
	}{settingsData, exampleConf, uis.clientConfig.ClientBinaries, currentUser.Settings.GithubUser.LastKnownAs, currentUser.Settings.GithubUser.UID, uis.umIsLDAP, uis.GetCommonViewData(w, r, true, true)},
		"base", "settings.html", "base_angular.html", "menu.html")
}
