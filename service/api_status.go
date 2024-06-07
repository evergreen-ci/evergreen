package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
)

func (as *APIServer) serviceStatusSimple(w http.ResponseWriter, r *http.Request) {
	env := evergreen.GetEnvironment()
	out := struct {
		BuildID      string `json:"build_revision"`
		BuildVersion string `json:"build_version"`
		AgentVersion string `json:"agent_version"`
	}{
		BuildID:      evergreen.BuildRevision,
		BuildVersion: env.BuildVersion(),
		AgentVersion: evergreen.AgentVersion,
	}

	if env.ShutdownSequenceStarted() {
		gimlet.WriteJSONInternalError(w, &out)
		return
	}

	gimlet.WriteJSON(w, &out)
}
