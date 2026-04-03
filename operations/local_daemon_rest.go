package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/evergreen-ci/evergreen/agent/taskexec"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const ndjsonContentType = "application/x-ndjson"

// localDaemonREST implements an API for the local debugger daemon
type localDaemonREST struct {
	executor   *taskexec.LocalExecutor
	mu         sync.RWMutex
	conf       *ClientSettings
	configPath string
	port       int
}

// newLocalDaemonREST creates a new REST daemon
func newLocalDaemonREST(port int, conf *ClientSettings) *localDaemonREST {
	return &localDaemonREST{
		port: port,
		conf: conf,
	}
}

// Start starts the REST debug daemon
func (d *localDaemonREST) Start() error {
	router := mux.NewRouter()
	router.HandleFunc("/health", d.handleHealth).Methods("GET")
	router.HandleFunc("/config/load", d.handleLoadConfig).Methods("POST")
	router.HandleFunc("/task/select", d.handleSelectTask).Methods("POST")
	router.HandleFunc("/task/list-steps", d.handleListSteps).Methods("GET")
	router.HandleFunc("/step/next", d.handleStepNext).Methods("POST")
	router.HandleFunc("/step/run-all", d.handleRunAll).Methods("POST")
	router.HandleFunc("/step/run-until/{step}", d.handleRunUntil).Methods("POST")
	router.HandleFunc("/step/jump/{step}", d.handleJumpTo).Methods("POST")
	router.HandleFunc("/variable/set", d.handleSetVariable).Methods("POST")
	router.HandleFunc("/status", d.handleStatus).Methods("GET")

	if err := d.writeDaemonInfo(); err != nil {
		grip.Warning(context.Background(), errors.Wrap(err, "writing daemon info"))
	}

	grip.Infof(context.Background(), "Starting REST daemon on port %d", d.port)
	return http.ListenAndServe(fmt.Sprintf(":%d", d.port), router)
}

// handleHealth checks if the daemon is running
func (d *localDaemonREST) handleHealth(w http.ResponseWriter, r *http.Request) {
	grip.Error(r.Context(), json.NewEncoder(w).Encode(map[string]bool{"healthy": true}))
}

// handleLoadConfig loads a configuration file
func (d *localDaemonREST) handleLoadConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfigPath string `json:"config_path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, errors.Wrap(err, "loading config").Error(), http.StatusBadRequest)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	workDir := filepath.Dir(req.ConfigPath)

	opts := taskexec.LocalExecutorOptions{
		WorkingDir:  workDir,
		ServerURL:   d.conf.getApiServerHost(true),
		TaskID:      d.conf.TaskID,
		OAuthToken:  d.conf.OAuth.AccessToken,
		SpawnHostID: d.conf.SpawnHostID,
	}

	if opts.OAuthToken == "" {
		http.Error(w, "OAuth token is required", http.StatusUnauthorized)
		return
	}

	executor, err := taskexec.NewLocalExecutor(r.Context(), opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	project, err := executor.LoadProject(req.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := executor.SetupWorkingDirectory(workDir); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	d.executor = executor
	d.configPath = req.ConfigPath

	grip.Error(r.Context(), json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"task_count":    len(project.Tasks),
		"variant_count": len(project.BuildVariants),
	}))
}

// handleSelectTask selects a task for debugging
func (d *localDaemonREST) handleSelectTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskName string `json:"task_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, errors.Wrap(err, "selecting task").Error(), http.StatusBadRequest)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.executor == nil {
		http.Error(w, "no configuration loaded", http.StatusBadRequest)
		return
	}

	if err := d.executor.PrepareTask(r.Context(), req.TaskName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	state := d.executor.GetDebugState()
	grip.Error(r.Context(), json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"step_count": len(state.CommandList),
	}))
}

// writeDaemonInfo writes PID and port files
func (d *localDaemonREST) writeDaemonInfo() error {
	dir, err := getDaemonDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(err, "creating daemon directory")
	}

	pidFile := filepath.Join(dir, daemonPIDFile)
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
		return errors.Wrap(err, "writing PID file")
	}

	portFile := filepath.Join(dir, daemonPortFile)
	if err := os.WriteFile(portFile, []byte(fmt.Sprintf("%d", d.port)), 0644); err != nil {
		return errors.Wrap(err, "writing port file")
	}

	return nil
}

// handleJumpTo jumps to a specific step
func (d *localDaemonREST) handleJumpTo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	stepNum := vars["step"]

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.executor == nil {
		http.Error(w, "no configuration loaded", http.StatusBadRequest)
		return
	}

	state := d.executor.GetDebugState()
	index, err := state.ResolveStepNumber(stepNum)
	if err != nil {
		http.Error(w, errors.Wrap(err, "resolving step number").Error(), http.StatusBadRequest)
		return
	}

	if err := d.executor.JumpTo(index); err != nil {
		http.Error(w, errors.Wrap(err, "jumping to step").Error(), http.StatusBadRequest)
		return
	}

	grip.Error(r.Context(), json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"current_step": state.CurrentStepIndex,
	}))
}

// handleListSteps lists all steps in the current task
func (d *localDaemonREST) handleListSteps(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.executor == nil {
		http.Error(w, "no configuration loaded", http.StatusBadRequest)
		return
	}

	state := d.executor.GetDebugState()

	steps := []map[string]interface{}{}
	for i, cmd := range state.CommandList {
		executed, success := state.GetStepExecution(i)

		steps = append(steps, map[string]interface{}{
			"index":         i,
			"step_number":   cmd.FullStepNumber(),
			"command_type":  cmd.Command.Command,
			"display_name":  cmd.DisplayName,
			"is_function":   cmd.IsFunction,
			"function_name": cmd.FunctionName,
			"executed":      executed,
			"success":       success,
		})
	}

	grip.Error(r.Context(), json.NewEncoder(w).Encode(map[string]interface{}{
		"steps":        steps,
		"current_step": state.CurrentStepIndex,
	}))
}

// handleRunUntil runs until a specific step identified by step number string.
func (d *localDaemonREST) handleRunUntil(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	stepNum := vars["step"]

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.executor == nil {
		http.Error(w, "no configuration loaded", http.StatusBadRequest)
		return
	}

	state := d.executor.GetDebugState()
	index, err := state.ResolveStepNumber(stepNum)
	if err != nil {
		http.Error(w, errors.Wrap(err, "resolving step number").Error(), http.StatusBadRequest)
		return
	}

	d.withStreaming(r.Context(), w, func(ctx context.Context) error {
		return d.executor.RunUntil(ctx, index)
	})
}

// handleRunAll runs all remaining steps with streaming output.
func (d *localDaemonREST) handleRunAll(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.executor == nil {
		http.Error(w, "no configuration loaded", http.StatusBadRequest)
		return
	}

	d.withStreaming(r.Context(), w, func(ctx context.Context) error {
		return d.executor.RunAll(ctx)
	})
}

func (d *localDaemonREST) withStreaming(ctx context.Context, w http.ResponseWriter, fn func(ctx context.Context) error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	if err := d.executor.SetupLogManager(false); err != nil {
		grip.Warning(ctx, errors.Wrap(err, "setting up log manager"))
	}

	state := d.executor.GetDebugState()
	taskLogFile := d.getLogFile()
	sw := taskexec.NewStreamWriterExported(w, flusher, taskLogFile, state.CurrentStepIndex)
	d.executor.SetStreamWriter(sw)

	w.Header().Set("Content-Type", ndjsonContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if err := fn(ctx); err != nil {
		grip.Error(ctx, errors.Wrap(err, "executing streamed operation"))
	}

	d.executor.ClearStreamWriter()
	d.executor.CloseLogManager()
}

// getLogFile returns the output log file from the executor's log manager, or nil.
func (d *localDaemonREST) getLogFile() *taskexec.LogFileHandle {
	if d.executor == nil {
		return nil
	}
	lm := d.executor.GetLogManager()
	if lm == nil {
		return nil
	}
	return lm.LogHandle()
}

// handleSetVariable sets a custom variable.
func (d *localDaemonREST) handleSetVariable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.executor == nil {
		http.Error(w, "no configuration loaded", http.StatusBadRequest)
		return
	}

	d.executor.SetVariable(req.Key, req.Value)
	grip.Error(r.Context(), json.NewEncoder(w).Encode(map[string]bool{"success": true}))
}

// handleStepNext executes the next step with streaming output.
func (d *localDaemonREST) handleStepNext(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.executor == nil {
		http.Error(w, "no configuration loaded", http.StatusBadRequest)
		return
	}

	d.withStreaming(r.Context(), w, func(ctx context.Context) error {
		return d.executor.StepNext(ctx)
	})
}

// handleStatus returns the daemon status.
func (d *localDaemonREST) handleStatus(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	response := map[string]interface{}{
		"healthy": true,
	}

	if d.executor != nil {
		state := d.executor.GetDebugState()
		response["task_selected"] = state.SelectedTask != ""
		response["selected_task"] = state.SelectedTask
		response["current_step"] = state.CurrentStepIndex
		response["total_steps"] = len(state.CommandList)
	}

	grip.Error(r.Context(), json.NewEncoder(w).Encode(response))
}
