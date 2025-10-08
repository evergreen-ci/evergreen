package service

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	stagingEnvironmentCookieName = "evg-staging-environment"
)

func (uis *UIServer) loginPage(w http.ResponseWriter, r *http.Request) {
	if uis.env.UserManager().IsRedirect() {
		http.Redirect(w, r, "/login/redirect", http.StatusFound)
		return
	}
	uis.render.WriteResponse(w, http.StatusOK, nil, "base", "login.html", "base_angular.html")
}

func (uis *UIServer) login(w http.ResponseWriter, r *http.Request) {
	creds := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{}

	if err := utility.ReadJSON(utility.NewRequestReader(r), &creds); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if creds.Username == "" || creds.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	token, err := uis.env.UserManager().CreateUserToken(creds.Username, creds.Password)
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

func (uis *UIServer) userGetKey(w http.ResponseWriter, r *http.Request) {
	creds := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{}

	if err := utility.ReadJSON(utility.NewRequestReader(r), &creds); err != nil {
		gimlet.WriteResponse(w, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "malformed request",
			StatusCode: http.StatusBadRequest,
		}))
		return
	}

	if creds.Username == "" || creds.Password == "" {
		gimlet.WriteResponse(w, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "user not specified",
			StatusCode: http.StatusBadRequest,
		}))
		return
	}

	token, err := uis.env.UserManager().CreateUserToken(creds.Username, creds.Password)
	if err != nil {
		gimlet.WriteResponse(w, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "could not find user",
			StatusCode: http.StatusUnauthorized,
		}))
		return
	}
	uis.umconf.AttachCookie(token, w)

	u, err := uis.env.UserManager().GetUserByToken(r.Context(), token)
	if err != nil {
		gimlet.WriteResponse(w, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "could not find user",
			StatusCode: http.StatusUnauthorized,
		}))
		return
	}
	dbUser, ok := u.(*user.DBUser)
	if !ok {
		gimlet.WriteResponse(w, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "found user but not from the DB",
			StatusCode: http.StatusInternalServerError,
		}))
		return
	}

	key := u.GetAPIKey()
	if key == "" {
		key = utility.RandomString()
		if err := dbUser.UpdateAPIKey(r.Context(), key); err != nil {
			gimlet.WriteResponse(w, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message:    "could not generate key",
				StatusCode: http.StatusInternalServerError,
			}))
			return
		}
	}

	out := struct {
		User  string `json:"user" yaml:"user" `
		Key   string `json:"api_key" yaml:"api_key"`
		UI    string `json:"ui_server_host" yaml:"ui_server_host"`
		API   string `json:"api_server_host" yaml:"api_server_host"`
		OAuth struct {
			Issuer      string `json:"issuer" yaml:"issuer"`
			ClientID    string `json:"client_id" yaml:"client_id"`
			ConnectorID string `json:"connector_id" yaml:"connector_id"`
		}
	}{
		User: creds.Username,
		Key:  key,
		UI:   uis.RootURL,
		API:  uis.RootURL + "/api",
		OAuth: struct {
			Issuer      string `json:"issuer" yaml:"issuer"`
			ClientID    string `json:"client_id" yaml:"client_id"`
			ConnectorID string `json:"connector_id" yaml:"connector_id"`
		}{
			Issuer:      uis.Settings.AuthConfig.OAuth.Issuer,
			ClientID:    uis.Settings.AuthConfig.OAuth.ClientID,
			ConnectorID: uis.Settings.AuthConfig.OAuth.ConnectorID,
		},
	}

	if ct := r.Header.Get("content-type"); strings.Contains(ct, "yaml") {
		gimlet.WriteYAML(w, out)
	} else {
		gimlet.WriteJSON(w, out)
	}
}

func (uis *UIServer) logout(w http.ResponseWriter, r *http.Request) {
	uis.umconf.ClearCookie(w)
	if uis.Settings.Ui.StagingEnvironment != "" {
		http.SetCookie(w, &http.Cookie{
			Name:    stagingEnvironmentCookieName,
			Value:   uis.Settings.Ui.StagingEnvironment,
			Domain:  uis.Settings.Ui.LoginDomain,
			Expires: time.Now().Add(evergreen.LoginCookieTTL),
		})
	}

	loginURL := fmt.Sprintf("%v/login", uis.RootURL)
	http.Redirect(w, r, loginURL, http.StatusFound)
}

func (uis *UIServer) newAPIKey(w http.ResponseWriter, r *http.Request) {
	currentUser := MustHaveUser(r)
	newKey := utility.RandomString()
	if err := currentUser.UpdateAPIKey(r.Context(), newKey); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "failed saving key"))
		return
	}
	gimlet.WriteJSON(w, struct {
		Key string `json:"key"`
	}{newKey})
}

func (uis *UIServer) clearUserToken(w http.ResponseWriter, r *http.Request) {
	u := MustHaveUser(r)
	if err := uis.env.UserManager().ClearUser(u, false); err != nil {
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
		User    string   `json:"user"`
		APIKey  string   `json:"api_key"`
		APIHost string   `json:"api_server_host"`
		UIHost  string   `json:"ui_server_host"`
		Regions []string `json:"regions"`
	}
	regions := uis.Settings.Providers.AWS.AllowedRegions
	exampleConf := confFile{currentUser.Id, currentUser.APIKey, uis.Settings.Api.URL + "/api", uis.Settings.Ui.Url, regions}
	newUILink := ""
	if len(uis.Settings.Ui.UIv2Url) > 0 {
		newUILink = fmt.Sprintf("%s/preferences", uis.Settings.Ui.UIv2Url)
	}

	if _, ok := gimlet.GetUser(r.Context()).(*user.DBUser); !ok {
		uis.LoggedError(w, r, http.StatusNotFound, errors.New("User not found"))
		return
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Data           user.UserSettings
		Config         confFile
		Binaries       []evergreen.ClientBinary
		GithubUser     string
		GithubUID      int
		CanClearTokens bool
		NewUILink      string
		ViewData
	}{settingsData, exampleConf, uis.env.ClientConfig().ClientBinaries, currentUser.Settings.GithubUser.LastKnownAs,
		currentUser.Settings.GithubUser.UID, uis.env.UserManagerInfo().CanClearTokens, newUILink, uis.GetCommonViewData(w, r, true, true)},
		"base", "settings.html", "base_angular.html", "menu.html")
}
