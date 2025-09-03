package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
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
	flags, err := evergreen.GetServiceFlags(r.Context())
	if err != nil {
		gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "retrieving admin settings")))
		return
	}
	projectRef, err := model.FindAllProjectRefs(ctx)
	if err != nil {
		grip.Error(errors.Wrap(err, "unable to retrieve project refs"))
	}
	projectRefData := make([]map[string]string, 0, len(projectRef))
	for _, ref := range projectRef {
		projectRefData = append(projectRefData, map[string]string{
			"id":          ref.Id,
			"displayName": ref.Identifier,
		})
	}
	repoRef, err := model.FindAllRepoRefs(ctx)
	if err != nil {
		grip.Error(errors.Wrap(err, "unable to retrieve repo refs"))
	}
	repoRefData := make([]map[string]string, 0, len(repoRef))
	for _, ref := range repoRef {
		repoRefData = append(repoRefData, map[string]string{
			"id":          ref.Id,
			"displayName": ref.Repo,
		})
	}
	spruceLink := fmt.Sprintf("%s/admin-settings/general", uis.Settings.Ui.UIv2Url)
	if flags.LegacyUIAdminPageDisabled {
		http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
		return
	}
	data := struct {
		ViewData
		CanClearTokens                         bool
		ShowOverridesSection                   bool
		ValidDefaultHostAllocatorRoundingRules []string
		ValidDefaultHostAllocatorFeedbackRules []string
		ValidDefaultHostsOverallocatedRules    []string
		ProjectRefData                         []map[string]string
		RepoRefData                            []map[string]string
	}{uis.GetCommonViewData(w, r, true, false), uis.env.UserManagerInfo().CanClearTokens, uis.env.SharedDB() != nil, evergreen.ValidDefaultHostAllocatorRoundingRules, evergreen.ValidDefaultHostAllocatorFeedbackRules, evergreen.ValidDefaultHostsOverallocatedRules, projectRefData, repoRefData}

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
	events, err := data.GetAdminEventLog(ctx, time.Now(), 15)
	if err != nil {
		grip.Error(errors.Wrap(err, "unable to retrieve admin events"))
	}
	data := struct {
		Data []restModel.APIAdminEvent
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
