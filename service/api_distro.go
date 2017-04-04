package service

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/host"
)

// GetDistro loads the task's distro and sends it to the requester.
func (as *APIServer) GetDistro(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	// Get the distro for this task
	h, err := host.FindOne(host.ByRunningTaskId(t.Id))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if h == nil {
		message := fmt.Errorf("No host found running task %v", t.Id)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	// agent can't properly unmarshal provider settings map
	h.Distro.ProviderSettings = nil
	as.WriteJSON(w, http.StatusOK, h.Distro)
}
