package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/user"
)

func (uis *UIServer) adminSettings(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	DBUser := GetUser(r)
	template := "not_admin.html"
	if uis.isSuperUser(DBUser) {
		template = "admin.html"
	}
	data := struct {
		ProjectData projectContext
		User        *user.DBUser
	}{projCtx, DBUser}
	uis.WriteHTML(w, http.StatusOK, data, "base", template, "base_angular.html", "menu.html")
}
