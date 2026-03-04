package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/evergreen-ci/evergreen/agent/taskexec"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

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
	router.HandleFunc("/step/run-until/{index}", d.handleRunUntil).Methods("POST")
	router.HandleFunc("/step/jump/{index}", d.handleJumpTo).Methods("POST")
	router.HandleFunc("/variable/set", d.handleSetVariable).Methods("POST")

	if err := d.writeDaemonInfo(); err != nil {
		grip.Warning(errors.Wrap(err, "writing daemon info"))
	}

	grip.Infof("Starting REST daemon on port %d", d.port)
	return http.ListenAndServe(fmt.Sprintf(":%d", d.port), router)
}

// handleHealth checks if the daemon is running
func (d *localDaemonREST) handleHealth(w http.ResponseWriter, r *http.Request) {
	grip.Error(json.NewEncoder(w).Encode(map[string]bool{"healthy": true}))
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

	grip.Error(json.NewEncoder(w).Encode(map[string]interface{}{
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

	if err := d.executor.PrepareTask(req.TaskName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	state := d.executor.GetDebugState()
	grip.Error(json.NewEncoder(w).Encode(map[string]interface{}{
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
	index, err := strconv.Atoi(vars["index"])
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid step index %d", index), http.StatusBadRequest)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.executor == nil {
		http.Error(w, "no configuration loaded", http.StatusBadRequest)
		return
	}

	if err := d.executor.JumpTo(index); err != nil {
		http.Error(w, errors.Wrap(err, "jumping to index").Error(), http.StatusBadRequest)
		return
	}

	state := d.executor.GetDebugState()
	grip.Error(json.NewEncoder(w).Encode(map[string]interface{}{
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
		executed := false
		success := false

		// Check if this step has been executed
		for _, record := range state.ExecutionHistory {
			if record.StepIndex == i {
				executed = true
				success = record.Success
				break
			}
		}

		steps = append(steps, map[string]interface{}{
			"index":         i,
			"command_type":  cmd.Command.Command,
			"display_name":  cmd.DisplayName,
			"is_function":   cmd.IsFunction,
			"function_name": cmd.FunctionName,
			"executed":      executed,
			"success":       success,
		})
	}

	grip.Error(json.NewEncoder(w).Encode(map[string]interface{}{
		"steps":        steps,
		"current_step": state.CurrentStepIndex,
	}))
}

// handleRunUntil runs until a specific step
func (d *localDaemonREST) handleRunUntil(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	index, err := strconv.Atoi(vars["index"])
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid step index %d", index), http.StatusBadRequest)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.executor == nil {
		http.Error(w, "no configuration loaded", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	err = d.executor.RunUntil(ctx, index)
	state := d.executor.GetDebugState()

	response := map[string]interface{}{
		"success":      err == nil,
		"current_step": state.CurrentStepIndex,
	}

	if err != nil {
		response["error"] = err.Error()
	}

	grip.Error(json.NewEncoder(w).Encode(response))
}

// handleRunAll runs all remaining steps
func (d *localDaemonREST) handleRunAll(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.executor == nil {
		http.Error(w, "no configuration loaded", http.StatusBadRequest)
		return
	}

	err := d.executor.RunAll(r.Context())
	state := d.executor.GetDebugState()

	response := map[string]interface{}{
		"success":      err == nil,
		"current_step": state.CurrentStepIndex,
	}

	if err != nil {
		response["error"] = err.Error()
	}

	grip.Error(json.NewEncoder(w).Encode(response))
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
	grip.Error(json.NewEncoder(w).Encode(map[string]bool{"success": true}))
}

// handleStepNext executes the next step
func (d *localDaemonREST) handleStepNext(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.executor == nil {
		http.Error(w, "no configuration loaded", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	err := d.executor.StepNext(ctx)
	state := d.executor.GetDebugState()

	response := map[string]interface{}{
		"success":      err == nil,
		"current_step": state.CurrentStepIndex,
	}

	if err != nil {
		response["error"] = err.Error()
	}

	grip.Error(json.NewEncoder(w).Encode(response))
}
