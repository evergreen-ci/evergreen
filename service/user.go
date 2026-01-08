package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const (
	stagingEnvironmentCookieName = "evg-staging-environment"
)

func (uis *UIServer) loginRedirect(w http.ResponseWriter, r *http.Request) {
	if uis.env.UserManager().IsRedirect() {
		http.Redirect(w, r, "/login/redirect", http.StatusFound)
		return
	}
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

func (uis *UIServer) logout(w http.ResponseWriter, r *http.Request) {
	uis.umconf.ClearCookie(w)
	if uis.Settings.Ui.StagingEnvironment != "" {
		http.SetCookie(w, &http.Cookie{
			Name:    stagingEnvironmentCookieName,
			Value:   uis.Settings.Ui.StagingEnvironment,
			Domain:  uis.Settings.Ui.LoginDomain,
			Expires: time.Now().Add(evergreen.LoginCookieTTL),
			Secure:  true,
		})
	}

	loginURL := fmt.Sprintf("%v/login", uis.RootURL)
	http.Redirect(w, r, loginURL, http.StatusFound)
}
