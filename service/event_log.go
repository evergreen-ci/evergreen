package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func (uis *UIServer) fullEventLogs(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	resourceType := strings.ToUpper(vars["resource_type"])
	resourceID := vars["resource_id"]
	ctx := r.Context()
	u := gimlet.GetUser(ctx)

	var loggedEvents []event.EventLogEntry
	var err error
	switch resourceType {
	case event.ResourceTypeTask:
		loggedEvents, err = event.Find(event.MostRecentTaskEvents(resourceID, 100))
	case event.ResourceTypeHost:
		if u == nil {
			uis.RedirectToLogin(w, r)
			return
		}
		var h *host.Host
		h, err = host.FindOneByIdOrTag(ctx, resourceID)
		if err != nil {
			http.Error(w, errors.Wrap(err, "error finding host '%s'").Error(), http.StatusInternalServerError)
			return
		}
		if h == nil {
			http.Error(w, "host '%s' not found", http.StatusBadRequest)
			return
		}
		hostEventsOpts := event.HostEventsOpts{
			ID:      h.Id,
			Tag:     h.Tag,
			Limit:   5000,
			SortAsc: false,
		}
		loggedEvents, err = event.Find(event.HostEvents(hostEventsOpts))
	case event.ResourceTypeAdmin:
		if u == nil {
			uis.RedirectToLogin(w, r)
			return
		}
		loggedEvents, err = event.Find(event.RecentAdminEvents(100))
	case event.EventResourceTypeProject:
		if u == nil {
			uis.RedirectToLogin(w, r)
			return
		}

		var project *model.ProjectRef
		project, err = model.FindBranchProjectRef(resourceID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		if project == nil {
			http.Error(w, fmt.Sprintf("Unknown project: %v", resourceType), http.StatusBadRequest)
			return
		}
		authorized := u.HasPermission(gimlet.PermissionOpts{
			Resource:      project.Id,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionProjectSettings,
			RequiredLevel: evergreen.ProjectSettingsEdit.Value,
		})
		if !authorized {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		var loggedProjectEvents model.ProjectChangeEvents
		loggedProjectEvents, err = model.MostRecentProjectEvents(resourceID, 200)
		for _, event := range loggedProjectEvents {
			loggedEvents = append(loggedEvents, event.EventLogEntry)
		}
	default:
		http.Error(w, fmt.Sprintf("Unknown resource: %v", resourceType), http.StatusBadRequest)
		return
	}

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Data []event.EventLogEntry
		ViewData
	}{loggedEvents, uis.GetCommonViewData(w, r, false, false)}, "base", "event_log.html", "base_angular.html")
}
