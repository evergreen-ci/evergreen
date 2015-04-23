package ui

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/util"
	"fmt"
	"labix.org/v2/mgo/bson"
	"net/http"
)

func (uis *UIServer) loginPage(w http.ResponseWriter, r *http.Request) {
	uis.WriteHTML(w, http.StatusOK, nil, "base", "login.html", "base_angular.html")
}

func setLoginToken(token string, w http.ResponseWriter) {
	authTokenCookie := &http.Cookie{
		Name:     mci.AuthTokenCookie,
		Value:    token,
		HttpOnly: true,
		Path:     "/",
	}
	http.SetCookie(w, authTokenCookie)
}

func clearSession(w http.ResponseWriter) {
	authTokenCookie := &http.Cookie{
		Name:   mci.AuthTokenCookie,
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
		http.Error(w, fmt.Sprintf("Invalid username/password"), http.StatusUnauthorized)
		return
	}
	setLoginToken(token, w)
	uis.WriteJSON(w, http.StatusOK, map[string]string{"redirect": getRedirectPath(r.Referer())})
}

func (uis *UIServer) logout(w http.ResponseWriter, r *http.Request) {
	clearSession(w)
	loginURL := fmt.Sprintf("%v/login", uis.RootURL)
	http.Redirect(w, r, loginURL, http.StatusFound)
}

func (uis *UIServer) userSettingsPage(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r)
	if user == nil {
		panic("User required")
	}

	projCtx := MustHaveProjectContext(r)

	settingsData := user.Settings
	flashes := PopFlashes(uis.CookieStore, r, w)

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		Data        model.UserSettings
		User        *model.DBUser
		Flashes     []interface{}
	}{projCtx, settingsData, user, flashes}, "base",
		"settings.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) userSettingsModify(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r)
	if user == nil {
		panic("user is required")
	}

	userSettings := model.UserSettings{}

	if err := util.ReadJSONInto(r.Body, &userSettings); err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	update := bson.M{"$set": bson.M{"settings": userSettings}}

	if err := model.UpdateOneUser(user.Username(), update); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error saving user settings: %v", err))
		return
	}

	PushFlash(uis.CookieStore, r, w, NewSuccessFlash("New user settings were saved successfully."))
	uis.WriteJSON(w, http.StatusOK, "Updated user settings successfully")
}
