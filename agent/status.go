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
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/status", port))
	if err == nil {
		defer resp.Body.Close()
		return errors.Errorf("another process is running on localhost port %d", port)
	}
	app := gimlet.NewApp()
	if err = app.SetPort(ctx, port); err != nil {
		return errors.WithStack(err)
	}
	app.NoVersions = true

	app.AddMiddleware(gimlet.MakeRecoveryLogger())
	app.AddRoute("/status").Handler(a.statusHandler()).Get()
	app.AddRoute("/failure_metadata_tag").Handler(a.addMetadataTagHandler).Post()
	app.AddRoute("/task_status").Handler(a.endTaskHandler).Post()
	app.AddRoute("/virtual_tasks/test_results").Handler(a.uploadVirtualTaskTestResultsHandler).Post()
	app.AddRoute("/oom/clear").Handler(http.RedirectHandler("/jasper/v1/list/oom", http.StatusMovedPermanently).ServeHTTP).Delete()
	app.AddRoute("/oom/check").Handler(http.RedirectHandler("/jasper/v1/list/oom", http.StatusMovedPermanently).ServeHTTP).Get()

	jpmapp := remote.NewRESTService(a.jasper).App(ctx)
	jpmapp.SetPrefix("jasper")

	handler, err := gimlet.MergeApplications(app, jpmapp, gimlet.GetPProfApp())
	if err != nil {
		return errors.WithStack(err)
	}

	srv := &http.Server{
		Addr:        fmt.Sprintf("127.0.0.1:%d", port),
		Handler:     handler,
		ReadTimeout: 10 * time.Second,
		// Virtual task test result uploads can be slow and can queue if there's
		// many incoming requests, so the write timeout must be generous enough
		// to accommodate them.
		WriteTimeout: 5 * time.Minute,
	}
	grip.Infoln(ctx, "Starting status server on address:", srv.Addr)

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			if err.Error() == "http: Server closed" {
				grip.Info(ctx, err)
				return
			}
			grip.EmergencyFatal(ctx, err)
		}
	}()

	go func() {
		<-ctx.Done()
		grip.Info(ctx, "Shutting down status server.")
		grip.Critical(ctx, srv.Shutdown(ctx))
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
		grip.Debug(r.Context(), "Preparing status response.")
		resp := buildResponse(a.opts)

		// in the future we may want to use the same render
		// package used in the service, but doing this
		// manually is probably good enough for now.
		out, err := json.MarshalIndent(resp, " ", " ")
		if err != nil {
			grip.Error(r.Context(), errors.Wrap(err, "marshalling JSON for status handler response"))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, err = w.Write(out)
		grip.Error(r.Context(), errors.Wrap(err, "writing status handler response"))
	}
}

type triggerEndTaskResp struct {
	Description            string   `json:"desc,omitempty"`
	Status                 string   `json:"status,omitempty"`
	Type                   string   `json:"type,omitempty"`
	AddFailureMetadataTags []string `json:"add_failure_metadata_tags,omitempty"`
	ShouldContinue         bool     `json:"should_continue"`
}

type triggerAddMetadataTagResp struct {
	AddFailureMetadataTags []string `json:"add_failure_metadata_tags,omitempty"`
}

func (a *Agent) endTaskHandler(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeAgentHandlerError(w, http.StatusInternalServerError, errors.Wrap(err, "reading end task request body"))
		return
	}

	resp := triggerEndTaskResp{}
	if err := json.Unmarshal(payload, &resp); err != nil {
		writeAgentHandlerError(w, http.StatusBadRequest, errors.Wrap(err, "unmarshalling end task request body as JSON"))
		return
	}

	a.setEndTaskRespMutex.RLock()
	setEndTaskResp := a.setEndTaskResp
	a.setEndTaskRespMutex.RUnlock()

	if setEndTaskResp == nil {
		writeAgentHandlerError(w, http.StatusInternalServerError, errors.New("programmatic error: end task response setter is undefined"))
		return
	}

	setEndTaskResp(&resp)
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

func (a *Agent) addMetadataTagHandler(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeAgentHandlerError(w, http.StatusInternalServerError, errors.Wrap(err, "reading add metadata tag request body"))
		return
	}

	resp := triggerAddMetadataTagResp{}
	if err := json.Unmarshal(payload, &resp); err != nil {
		writeAgentHandlerError(w, http.StatusBadRequest, errors.Wrap(err, "unmarshalling add metadata tag request body as JSON"))
		return
	}

	a.addMetadataTagMutex.RLock()
	addMetadataTag := a.addMetadataTagResp
	a.addMetadataTagMutex.RUnlock()

	if addMetadataTag == nil {
		writeAgentHandlerError(w, http.StatusInternalServerError, errors.New("programmatic error: add metadata tag response setter is undefined"))
		return
	}

	addMetadataTag(&resp)
}

// writeAgentHandlerError writes a plain-text error response with the given
// status code for a local status server handler.
func writeAgentHandlerError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(err.Error()))
}
