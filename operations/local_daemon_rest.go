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
	"gopkg.in/yaml.v3"
)

const configPath = ".evergreen-local.yml"

type clientConfig struct {
	ServerURL string `yaml:"server_url"`
	TaskID    string `yaml:"task_id"`
	APIUser   string `yaml:"api_user"`
	APIKey    string `yaml:"api_key"`
}

// localDaemonREST implements an API for the local debugger daemon
type localDaemonREST struct {
	executor   *taskexec.LocalExecutor
	mu         sync.RWMutex
	configPath string
	port       int
}

// newLocalDaemonREST creates a new REST daemon
func newLocalDaemonREST(port int) *localDaemonREST {
	return &localDaemonREST{
		port: port,
	}
}

// Start starts the REST debug daemon
func (d *localDaemonREST) Start() error {
	router := mux.NewRouter()
	router.HandleFunc("/health", d.handleHealth).Methods("GET")
	router.HandleFunc("/config/load", d.handleLoadConfig).Methods("POST")
	router.HandleFunc("/task/select", d.handleSelectTask).Methods("POST")
	router.HandleFunc("/step/next", d.handleStepNext).Methods("POST")

	if err := d.writeDaemonInfo(); err != nil {
		grip.Warning(errors.Wrap(err, "writing daemon info"))
	}

	grip.Infof("Starting REST daemon on port %d", d.port)
	return http.ListenAndServe(fmt.Sprintf(":%d", d.port), router)
}

func (d *localDaemonREST) loadDebugClientConfig() (*clientConfig, error) {
	grip.Debugf("Checking for config file: %s", configPath)
	if _, err := os.Stat(configPath); err != nil {
		return nil, errors.Wrapf(err, "config file %s does not exist", configPath)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.Wrapf(err, "reading config file %s", configPath)
	}
	var config clientConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errors.Wrapf(err, "parsing config file %s", configPath)
	}

	grip.Infof("Loaded cleint configuration from %s (server: %s, task: %s, user: %s)",
		configPath, config.ServerURL, config.TaskID, config.APIUser)
	return &config, nil
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	workDir := filepath.Dir(req.ConfigPath)

	backendConfig, err := d.loadDebugClientConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	opts := taskexec.LocalExecutorOptions{
		WorkingDir: workDir,
		ServerURL:  backendConfig.ServerURL,
		TaskID:     backendConfig.TaskID,
		APIUser:    backendConfig.APIUser,
		APIKey:     backendConfig.APIKey,
	}

	if opts.APIUser == "" || opts.APIKey == "" {
		http.Error(w, "API user and key are required", http.StatusUnauthorized)
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
