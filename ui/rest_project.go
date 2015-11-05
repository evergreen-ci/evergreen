package ui

import (
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
