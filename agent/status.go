package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
)

func (a *Agent) startStatusServer(ctx context.Context, port int) error {
	// Although checking the error returned by `srv.ListenAndServe` is sufficient to exit if
	// another agent is running, it's possible for `startStatusServer` to return before
	// `grip.EmergencyFatal` runs, which means that later code, e.g., code that deletes files
	// that a running task depends on, could run.
	_, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/status", port))
	if err == nil {
		return errors.Errorf("another process is running on localhost port %d", port)
	}
	app := gimlet.NewApp()
	if err = app.SetPort(port); err != nil {
		return errors.WithStack(err)
	}
	app.NoVersions = true

	app.AddMiddleware(gimlet.MakeRecoveryLogger())
	app.AddRoute("/status").Handler(a.statusHandler()).Get()
	app.AddRoute("/task_status").Handler(a.endTaskHandler).Post()
	app.AddRoute("/oom/clear").Handler(http.RedirectHandler("/jasper/v1/list/oom", http.StatusMovedPermanently).ServeHTTP).Delete()
	app.AddRoute("/oom/check").Handler(http.RedirectHandler("/jasper/v1/list/oom", http.StatusMovedPermanently).ServeHTTP).Get()

	jpmapp := remote.NewRESTService(a.jasper).App(ctx)
	jpmapp.SetPrefix("jasper")

	handler, err := gimlet.MergeApplications(app, jpmapp, gimlet.GetPProfApp())
	if err != nil {
		return errors.WithStack(err)
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", port),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	grip.Infoln("Starting status server on address:", srv.Addr)

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			if err.Error() == "http: Server closed" {
				grip.Info(err)
				return
			}
			grip.EmergencyFatal(err)
		}
	}()

	go func() {
		<-ctx.Done()
		grip.Info("Shutting down status server.")
		grip.Critical(srv.Shutdown(ctx))
	}()

	return nil
}

// statusResponse is the structure of the response objects produced by
// the local status service.
type statusResponse struct {
	BuildRevision string                 `json:"agent_build"`
	AgentVersion  string                 `json:"agent_version"`
	AgentPid      int                    `json:"pid"`
	HostId        string                 `json:"host_id"`
	SystemInfo    *message.SystemInfo    `json:"sys_info"`
	ProcessTree   []*message.ProcessInfo `json:"ps_info"`
}

// statusHandler is a function that produces the status handler.
func (a *Agent) statusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		grip.Debug("Preparing status response.")
		resp := buildResponse(a.opts)

		// in the future we may want to use the same render
		// package used in the service, but doing this
		// manually is probably good enough for now.
		out, err := json.MarshalIndent(resp, " ", " ")
		if err != nil {
			grip.Error(errors.Wrap(err, "marshalling JSON for status handler response"))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, err = w.Write(out)
		grip.Error(errors.Wrap(err, "writing status handler response"))
	}
}

type TriggerEndTaskResp struct {
	Description    string `json:"desc,omitempty"`
	Status         string `json:"status,omitempty"`
	Type           string `json:"type,omitempty"`
	ShouldContinue bool   `json:"should_continue"`
}

func (a *Agent) endTaskHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		_ = grip.GetSender().Close()
	}()

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		_, _ = w.Write([]byte(errors.Wrap(err, "reading end task response body").Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := TriggerEndTaskResp{}
	if err := json.Unmarshal(payload, &resp); err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(errors.Wrap(err, "reading end task reply from response").Error()))

		return
	}

	a.endTaskResp = &resp
}

// buildResponse produces the response document for the current
// process, and is separate to facilitate testing.
func buildResponse(opts Options) statusResponse {
	out := statusResponse{
		AgentVersion:  evergreen.AgentVersion,
		BuildRevision: evergreen.BuildRevision,
		AgentPid:      os.Getpid(),
		HostId:        opts.HostID,
		SystemInfo:    message.CollectSystemInfo().(*message.SystemInfo),
	}

	psTree := message.CollectProcessInfoSelfWithChildren()
	out.ProcessTree = make([]*message.ProcessInfo, len(psTree))
	for idx, p := range psTree {
		out.ProcessTree[idx] = p.(*message.ProcessInfo)
	}

	return out
}
