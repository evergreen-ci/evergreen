package service

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
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
	permissions := gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionAdminSettings,
		RequiredLevel: evergreen.AdminSettingsEdit.Value,
	}
	if DBUser != nil {
		if DBUser.HasPermission(permissions) {
			template = "admin.html"
		}
	}

	data := struct {
		ViewData
		CanClearTokens                         bool
		ValidDefaultHostAllocatorRoundingRules []string
		ValidDefaultHostAllocatorFeedbackRules []string
		ValidDefaultHostsOverallocatedRules    []string
	}{uis.GetCommonViewData(w, r, true, true), uis.env.UserManagerInfo().CanClearTokens, evergreen.ValidDefaultHostAllocatorRoundingRules, evergreen.ValidDefaultHostAllocatorFeedbackRules, evergreen.ValidDefaultHostsOverallocatedRules}

	uis.render.WriteResponse(w, http.StatusOK, data, "base", template, "base_angular.html", "menu.html")
}

func (uis *UIServer) adminEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	DBUser := gimlet.GetUser(ctx)
	template := "not_admin.html"
	permissions := gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionAdminSettings,
		RequiredLevel: evergreen.AdminSettingsEdit.Value,
	}
	if DBUser != nil {
		if DBUser.HasPermission(permissions) {
			template = "admin_events.html"
		}
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

func (uis *UIServer) clearAllUserTokens(w http.ResponseWriter, r *http.Request) {
	if err := uis.env.UserManager().ClearUser(nil, true); err != nil {
		gimlet.WriteJSONInternalError(w, struct {
			Error string `json:"error"`
		}{Error: err.Error()})
	} else {
		gimlet.WriteJSON(w, map[string]string{})
	}
}
