package proto

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/codegangsta/negroni"
	"github.com/evergreen-ci/evergreen"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func (agt *Agent) startStatusServer(port int) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	go func() {
		r := mux.NewRouter().StrictSlash(false)
		r.HandleFunc("/status", agt.statusHandler()).Methods("GET")
		r.HandleFunc("/terminate", terminateAgentHandler).Methods("DELETE")

		n := negroni.New()
		n.Use(negroni.NewRecovery())
		n.UseHandler(r)

		grip.CatchEmergencyFatal(http.ListenAndServe(addr, n))
	}()

	grip.Infoln("starting status service on:", addr)
}

// statusResponse is the structure of the response objects produced by
// the local status service.
type statusResponse struct {
	BuildId     string                 `json:"agent_revision"`
	AgentPid    int                    `json:"pid"`
	HostId      string                 `json:"host_id"`
	SystemInfo  *message.SystemInfo    `json:"sys_info"`
	ProcessTree []*message.ProcessInfo `json:"ps_info"`
	LegacyAgent bool                   `json:"legacy_agent"`
}

// statusHandler is a function that produces the status handler.
func (agt *Agent) statusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		grip.Debug("preparing status response")
		resp := buildResponse(agt.opts)

		// in the future we may want to use the same render
		// package used in the service, but doing this
		// manually is probably good enough for now.
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

func terminateAgentHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		_ = grip.GetSender().Close()
	}()

	msg := map[string]interface{}{
		"message": "terminating agent triggered",
		"host":    r.Host,
	}
	grip.Info(msg)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	out, err := json.MarshalIndent(msg, " ", " ")
	if err != nil {
		grip.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
	_, err = w.Write(out)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	grip.CatchError(err)

	// need to use os.exit rather than a panic because the panic
	// handler will recover.
	os.Exit(1)
}

// buildResponse produces the response document for the current
// process, and is separate to facilitate testing.
func buildResponse(opts Options) statusResponse {
	out := statusResponse{
		BuildId:    evergreen.BuildRevision,
		AgentPid:   os.Getpid(),
		HostId:     opts.HostID,
		SystemInfo: message.CollectSystemInfo().(*message.SystemInfo),
	}

	psTree := message.CollectProcessInfoSelfWithChildren()
	out.ProcessTree = make([]*message.ProcessInfo, len(psTree))
	for idx, p := range psTree {
		out.ProcessTree[idx] = p.(*message.ProcessInfo)
	}

	return out
}
