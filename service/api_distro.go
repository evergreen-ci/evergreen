package service

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/gimlet"
)

// TODO (EVG-13239): remove once agents have updated.
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

	// agent can't properly unmarshal provider settings
	h.Distro.ProviderSettingsList = nil
	gimlet.WriteJSON(w, h.Distro)
}

func (as *APIServer) GetDistroView(w http.ResponseWriter, r *http.Request) {
	h := MustHaveHost(r)

	dv := apimodels.DistroView{
		CloneMethod:         h.Distro.CloneMethod,
		DisableShallowClone: h.Distro.DisableShallowClone,
		WorkDir:             h.Distro.WorkDir,
	}
	gimlet.WriteJSON(w, dv)
}
