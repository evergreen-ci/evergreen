package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/user"
)

func (uis *UIServer) adminSettings(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	data := struct {
		ProjectData projectContext
		User        *user.DBUser
	}{projCtx, GetUser(r)}
	uis.WriteHTML(w, http.StatusOK, data, "base", "admin.html", "base_angular.html", "menu.html")
}
