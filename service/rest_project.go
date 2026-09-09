package service

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
)

// Returns a JSON response of an array with the ref information for the requested project_id.
func (restapi restAPI) getProjectRef(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	ref := projCtx.ProjectRef
	if ref == nil {
		gimlet.WriteJSONResponse(r.Context(), w, http.StatusNotFound, responseError{Message: "error finding project"})
		return
	}
	refForResponse := *ref
	secretOwnerID := ref.Id
	if ref.RepoRefId != "" {
		branchProject, err := model.FindBranchProjectRef(r.Context(), ref.Id)
		if err != nil {
			gimlet.WriteJSONResponse(r.Context(), w, http.StatusInternalServerError, responseError{Message: "error finding project"})
			return
		}
		if branchProject != nil && branchProject.TaskAnnotationSettings.FileTicketWebhook.Secret == "" {
			secretOwnerID = ref.RepoRefId
		}
	}
	usr := gimlet.GetUser(r.Context())
	if usr == nil || !usr.HasPermission(r.Context(), gimlet.PermissionOpts{
		Resource:      secretOwnerID,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsEdit.Value,
	}) {
		refForResponse.TaskAnnotationSettings.FileTicketWebhook.Secret = ""
	}
	gimlet.WriteJSON(r.Context(), w, &refForResponse)
}

// getProjectsIds returns a JSON response of an array of active project Ids.
// Users must use credentials to see private projects.
func (restapi restAPI) getProjectIds(w http.ResponseWriter, r *http.Request) {
	refs, err := model.FindAllMergedProjectRefs(r.Context())
	if err != nil {
		gimlet.WriteJSONResponse(r.Context(), w, http.StatusNotFound, responseError{
			Message: fmt.Sprintf("error finding projects: %v", err),
		})
		return
	}

	ctx := r.Context()
	projects := []string{}
	if u := gimlet.GetUser(ctx); u != nil {
		for _, r := range refs {
			if r.Enabled {
				projects = append(projects, r.Id)
			}
		}
	}

	gimlet.WriteJSON(r.Context(), w, struct {
		Projects []string `json:"projects"`
	}{projects})
}
