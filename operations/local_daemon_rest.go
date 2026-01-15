package operations

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// localDaemonREST implements an API for the local debugger daemon
type localDaemonREST struct {
	mu   sync.RWMutex
	port int
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// TODO: DEVPROD-24304 load project from request config path, store it in daemon state.
	grip.Error(json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"task_count":    -1,
		"variant_count": -1,
	}))
}

// writeDaemonInfo writes PID and port files
func (d *localDaemonREST) writeDaemonInfo() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	daemonDir := filepath.Join(homeDir, ".evergreen-local")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		return err
	}

	pidFile := filepath.Join(daemonDir, "daemon.pid")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
		return err
	}

	portFile := filepath.Join(daemonDir, "daemon.port")
	return os.WriteFile(portFile, []byte(fmt.Sprintf("%d", d.port)), 0644)
}
