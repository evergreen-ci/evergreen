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

// LocalDaemonREST implements a REST API for the local debugger daemon
type LocalDaemonREST struct {
	executor   *taskexec.LocalExecutor
	mu         sync.RWMutex
	configPath string
	port       int
}

// NewLocalDaemonREST creates a new REST daemon
func NewLocalDaemonREST(port int) *LocalDaemonREST {
	return &LocalDaemonREST{
		port: port,
	}
}

// Start starts the REST daemon
func (d *LocalDaemonREST) Start() error {
	router := mux.NewRouter()

	// Define routes
	router.HandleFunc("/health", d.handleHealth).Methods("GET")
	router.HandleFunc("/config/load", d.handleLoadConfig).Methods("POST")
	router.HandleFunc("/task/select", d.handleSelectTask).Methods("POST")
	router.HandleFunc("/step/run-all", d.handleRunAll).Methods("POST")

	// Write PID and port files
	if err := d.writeDaemonInfo(); err != nil {
		grip.Warning(errors.Wrap(err, "writing daemon info"))
	}

	grip.Infof("Starting REST daemon on port %d", d.port)
	return http.ListenAndServe(fmt.Sprintf(":%d", d.port), router)
}

// handleHealth checks if the daemon is running
func (d *LocalDaemonREST) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]bool{"healthy": true})
}

// handleLoadConfig loads a configuration file
func (d *LocalDaemonREST) handleLoadConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfigPath string `json:"config_path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Create a new executor
	opts := taskexec.LocalExecutorOptions{
		WorkingDir: filepath.Dir(req.ConfigPath),
		LogLevel:   "info",
		Timeout:    7200,
	}

	executor, err := taskexec.NewLocalExecutor(opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Load the project
	project, err := executor.LoadProject(req.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set up working directory
	workDir := filepath.Dir(req.ConfigPath)
	if err := executor.SetupWorkingDirectory(workDir); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	d.executor = executor
	d.configPath = req.ConfigPath

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"task_count":    len(project.Tasks),
		"variant_count": len(project.BuildVariants),
	})
}

// handleSelectTask selects a task for debugging
func (d *LocalDaemonREST) handleSelectTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskName    string `json:"task_name"`
		VariantName string `json:"variant_name,omitempty"`
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

	if err := d.executor.PrepareTask(req.TaskName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	state := d.executor.GetDebugState()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"step_count": len(state.CommandList),
	})
}

// handleRunAll runs all remaining steps
func (d *LocalDaemonREST) handleRunAll(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.executor == nil {
		http.Error(w, "no configuration loaded", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	err := d.executor.RunAll(ctx)
	state := d.executor.GetDebugState()

	response := map[string]interface{}{
		"success":      err == nil,
		"current_step": state.CurrentStepIndex,
	}

	if err != nil {
		response["error"] = err.Error()
	}

	json.NewEncoder(w).Encode(response)
}

// writeDaemonInfo writes PID and port files
func (d *LocalDaemonREST) writeDaemonInfo() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	daemonDir := filepath.Join(homeDir, ".evergreen-local")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		return err
	}

	// Write PID file
	pidFile := filepath.Join(daemonDir, "daemon.pid")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
		return err
	}

	// Write port file
	portFile := filepath.Join(daemonDir, "daemon.port")
	return os.WriteFile(portFile, []byte(fmt.Sprintf("%d", d.port)), 0644)
}
