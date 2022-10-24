package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
)

func (as *APIServer) serviceStatusSimple(w http.ResponseWriter, r *http.Request) {
	out := struct {
		BuildID      string `json:"build_revision"`
		AgentVersion string `json:"agent_version"`
	}{
		BuildID:      evergreen.BuildRevision,
		AgentVersion: evergreen.AgentVersion,
	}

	env := evergreen.GetEnvironment()
	if env.ShutdownSequenceStarted() {
		gimlet.WriteJSONInternalError(w, &out)
		return
	}

	gimlet.WriteJSON(w, &out)
}
