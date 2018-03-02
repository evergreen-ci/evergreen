package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func (uis *UIServer) adminSettings(w http.ResponseWriter, r *http.Request) {
	DBUser := GetUser(r)
	template := "not_admin.html"
	if uis.isSuperUser(DBUser) {
		template = "admin.html"
	}
	events, err := event.Find(event.AllLogCollection, event.RecentAdminEvents(15))
	if err != nil {
		grip.Error(errors.Wrap(err, "unable to retrieve admin events"))
	}
	data := struct {
		Data []event.EventLogEntry
		ViewData
	}{events, uis.GetCommonViewData(w, r, true, true)}
	uis.WriteHTML(w, http.StatusOK, data, "base", template, "base_angular.html", "menu.html")
}
