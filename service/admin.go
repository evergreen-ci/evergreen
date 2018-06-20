package service

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func (uis *UIServer) adminSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	template := "not_admin.html"
	DBUser := gimlet.GetUser(ctx)
	if DBUser != nil && uis.isSuperUser(DBUser) {
		template = "admin.html"
	}
	data := struct {
		ViewData
	}{uis.GetCommonViewData(w, r, true, true)}
	uis.render.WriteResponse(w, http.StatusOK, data, "base", template, "base_angular.html", "menu.html")
}

func (uis *UIServer) adminEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	DBUser := gimlet.GetUser(ctx)
	template := "not_admin.html"
	if DBUser != nil && uis.isSuperUser(DBUser) {
		template = "admin_events.html"
	}
	dc := &data.DBAdminConnector{}
	events, err := dc.GetAdminEventLog(time.Now(), 15)
	if err != nil {
		grip.Error(errors.Wrap(err, "unable to retrieve admin events"))
	}
	data := struct {
		Data []model.APIAdminEvent
		ViewData
	}{events, uis.GetCommonViewData(w, r, true, true)}
	uis.render.WriteResponse(w, http.StatusOK, data, "base", template, "base_angular.html", "menu.html")
}
