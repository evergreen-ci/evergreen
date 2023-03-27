package service

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
)

// Returns a JSON response of an array with the ref information for the requested project_id.
func (restapi restAPI) getProjectRef(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	ref := projCtx.ProjectRef
	if ref == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{Message: "error finding project"})
		return
	}
	gimlet.WriteJSON(w, ref)
}

// getProjectsIds returns a JSON response of an array of active project Ids.
// Users must use credentials to see private projects.
func (restapi restAPI) getProjectIds(w http.ResponseWriter, r *http.Request) {
	refs, err := model.FindAllMergedProjectRefs()
	if err != nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{
			Message: fmt.Sprintf("error finding projects: %v", err),
		})
		return
	}

	ctx := r.Context()
	u := gimlet.GetUser(ctx)
	projects := []string{}
	for _, r := range refs {
		if r.Enabled && (!r.IsPrivate() || u != nil) {
			projects = append(projects, r.Id)
		}
	}
	gimlet.WriteJSON(w, struct {
		Projects []string `json:"projects"`
	}{projects})
}
