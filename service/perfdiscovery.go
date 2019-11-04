package service

import (
	"net/http"
)

func (uis *UIServer) perfdiscoveryPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		uis.ProjectNotFound(w, r)
		return
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		ViewData
		JiraHost string
	}{
		uis.GetCommonViewData(w, r, false, true),
		uis.Settings.Jira.Host,
	},
		"base", "perfdiscovery.html", "base_angular.html", "menu.html")
}
