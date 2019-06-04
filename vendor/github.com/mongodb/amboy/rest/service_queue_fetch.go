package rest

import (
	"net/http"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
)

// Fetch is an http handler that writes a job interchange object to a
// the response, and allows clients to retrieve jobs from the service.
func (s *QueueService) Fetch(w http.ResponseWriter, r *http.Request) {
	name := gimlet.GetVars(r)["name"]

	job, ok := s.queue.Get(r.Context(), name)
	if !ok {
		grip.Infof("job named %s does not exist in the queue", name)
		gimlet.WriteJSONResponse(w, http.StatusNotFound, nil)
		return
	}

	resp, err := registry.MakeJobInterchange(job, amboy.JSON)
	if err != nil {
		grip.Warningf("problem converting job %s to interchange format", name)
		gimlet.WriteJSONResponse(w, http.StatusInternalServerError, resp)
		return
	}

	gimlet.WriteJSON(w, resp)
}
