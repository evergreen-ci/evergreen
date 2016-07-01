package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/gorilla/mux"
)

func (uis *UIServer) getSchedulerLogs(w http.ResponseWriter, r *http.Request) {
	distroId := mux.Vars(r)["distro_id"]

	projCtx := MustHaveProjectContext(r)

	loggedEvents, err := event.Find(event.RecentSchedulerEvents(distroId, 500))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *user.DBUser
		Data        []event.Event
		DistroId    string
	}{projCtx, GetUser(r), loggedEvents, distroId}, "base", "scheduler_events.html", "base_angular.html", "menu.html")
}
