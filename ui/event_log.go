package ui

import (
	"10gen.com/mci/db"
	"10gen.com/mci/model/event"
	"10gen.com/mci/model/user"
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	"strings"
)

func (uis *UIServer) fullEventLogs(w http.ResponseWriter, r *http.Request) {
	resourceType := strings.ToUpper(mux.Vars(r)["resource_type"])
	resourceId := mux.Vars(r)["resource_id"]

	projCtx := MustHaveProjectContext(r)

	var eventQuery db.Q
	switch resourceType {
	case event.ResourceTypeTask:
		eventQuery = event.MostRecentTaskEvents(resourceId, 100)
	case event.ResourceTypeHost:
		eventQuery = event.MostRecentHostEvents(resourceId, 100)
	default:
		http.Error(w, fmt.Sprintf("Unknown resource: %v", resourceType), http.StatusBadRequest)
		return
	}

	loggedEvents, err := event.Find(eventQuery)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *user.DBUser
		Data        []event.Event
	}{projCtx, GetUser(r), loggedEvents}, "base", "event_log.html", "base_angular.html")
}
