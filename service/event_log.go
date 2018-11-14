package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/gimlet"
)

func (uis *UIServer) fullEventLogs(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	resourceType := strings.ToUpper(vars["resource_type"])
	resourceID := vars["resource_id"]
	ctx := r.Context()
	u := gimlet.GetUser(ctx)

	var eventQuery db.Q
	switch resourceType {
	case event.ResourceTypeTask:
		eventQuery = event.MostRecentTaskEvents(resourceID, 100)
	case event.ResourceTypeScheduler:
		eventQuery = event.RecentSchedulerEvents(resourceID, 500)
	case event.ResourceTypeHost:
		if u == nil {
			uis.RedirectToLogin(w, r)
			return
		}
		eventQuery = event.MostRecentHostEvents(resourceID, 5000)
	case event.ResourceTypeDistro:
		if u == nil {
			uis.RedirectToLogin(w, r)
			return
		}
		eventQuery = event.MostRecentDistroEvents(resourceID, 200)
	case event.ResourceTypeAdmin:
		if u == nil {
			uis.RedirectToLogin(w, r)
			return
		}
		eventQuery = event.RecentAdminEvents(100)
	case model.ResourceTypeProject:
		if u == nil {
			uis.RedirectToLogin(w, r)
			return
		}

		project, err := model.FindOneProjectRef(resourceID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		if project == nil {
			http.Error(w, fmt.Sprintf("Unknown project: %v", resourceType), http.StatusBadRequest)
			return
		}

		authorized := isAdmin(u, project) || uis.isSuperUser(u)
		if !authorized {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		eventQuery = model.MostRecentProjectEvents(resourceID, 200)
	default:
		http.Error(w, fmt.Sprintf("Unknown resource: %v", resourceType), http.StatusBadRequest)
		return
	}

	loggedEvents, err := event.Find(event.AllLogCollection, eventQuery)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Data []event.EventLogEntry
		ViewData
	}{loggedEvents, uis.GetCommonViewData(w, r, false, false)}, "base", "event_log.html", "base_angular.html")
}
