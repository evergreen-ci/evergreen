package ui

import (
	"10gen.com/mci/model"
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	"strings"
)

func (uis *UIServer) fullEventLogs(w http.ResponseWriter, r *http.Request) {
	resourceType := strings.ToUpper(mux.Vars(r)["resource_type"])
	resourceId := mux.Vars(r)["resource_id"]

	projCtx := MustHaveProjectContext(r)

	var eventFinder *model.EventFinder
	switch resourceType {
	case model.ResourceTypeTask:
		eventFinder = model.NewTaskEventFinder()
	case model.ResourceTypeHost:
		eventFinder = model.NewHostEventFinder()
	default:
		http.Error(w, fmt.Sprintf("Unknown resource: %v", resourceType), http.StatusBadRequest)
		return
	}

	loggedEvents, err := eventFinder.FindMostRecentEvents(resourceId, 100)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *model.DBUser
		Data        []model.Event
	}{projCtx, GetUser(r), loggedEvents}, "base", "event_log.html", "base_angular.html")
}
