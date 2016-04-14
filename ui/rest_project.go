package ui

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/model"
	"net/http"
)

// Returns a JSON response of an array with the ref information for the requested project_id.
func (restapi restAPI) getProject(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	ref := projCtx.ProjectRef
	if ref == nil {
		restapi.WriteJSON(w, http.StatusNotFound, responseError{Message: "error finding project"})
		return
	}
	// unset alerts so we don't expose emails through the API
	ref.Alerts = nil
	restapi.WriteJSON(w, http.StatusOK, ref)
	return
}

// getProjectsIds returns a JSON response of an array of active project Ids.
// Users must use credentials to see private projects.
func (restapi restAPI) getProjectIds(w http.ResponseWriter, r *http.Request) {
	u := GetUser(r)
	refs, err := model.FindAllProjectRefs()
	if err != nil {
		restapi.WriteJSON(w, http.StatusNotFound, responseError{
			Message: fmt.Sprintf("error finding projects: %v", err),
		})
		return
	}
	projects := []string{}
	for _, r := range refs {
		if r.Enabled && (!r.Private || u != nil) {
			projects = append(projects, r.Identifier)
		}
	}
	restapi.WriteJSON(w, http.StatusOK, struct {
		Projects []string `json:"projects"`
	}{projects})
	return
}
