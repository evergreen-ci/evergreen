package agent

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/evergreen-ci/evergreen"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// GetStatusRouter produces an HTTP router for the local status
// service.
func GetStatusRouter(opts Options) *mux.Router {
	r := mux.NewRouter().StrictSlash(false)

	r.HandleFunc("/status", statusHandler(opts)).Methods("GET")

	return r
}

// statusResponse is the structure of the response objects produced by
// the local status service.
type statusResponse struct {
	BuildId     string                 `json:"agent_revision"`
	AgentPid    int                    `json:"pid"`
	APIServer   string                 `json:"api_url"`
	HostId      string                 `json:"host_id"`
	SystemInfo  *message.SystemInfo    `json:"sys_info"`
	ProcessTree []*message.ProcessInfo `json:"ps_info"`

	// TODO (EVG-1440) include the current task ID when the service is part
	// of the agent itself.
}

// statusHandler is a function that produces the status handler.
func statusHandler(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		grip.Info("preparing status response")
		resp := buildResponse(&opts)

		// in the future we may want to use the same render
		// package used in the service, but doing this
		// manually is probably good enoug for now.
		out, err := json.MarshalIndent(resp, " ", " ")
		if err != nil {
			grip.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, err = w.Write(out)
		grip.CatchError(err)
	}
}

// buildResponse produces the response document for the current
// process, and is separate to facilitate testing.
func buildResponse(opts *Options) statusResponse {
	out := statusResponse{
		BuildId:    evergreen.BuildRevision,
		AgentPid:   os.Getpid(),
		APIServer:  opts.APIURL,
		HostId:     opts.HostId,
		SystemInfo: message.CollectSystemInfo().(*message.SystemInfo),
	}

	psTree := message.CollectProcessInfoSelfWithChildren()
	out.ProcessTree = make([]*message.ProcessInfo, len(psTree))
	for idx, p := range psTree {
		out.ProcessTree[idx] = p.(*message.ProcessInfo)
	}

	return out
}
