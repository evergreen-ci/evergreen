package operations

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func getHomeEnvVar() string {
	if runtime.GOOS == "windows" {
		return "USERPROFILE"
	}
	return "HOME"
}

func TestGetDaemonDir(t *testing.T) {
	dir, err := getDaemonDir()
	require.NoError(t, err)

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	expected := filepath.Join(homeDir, daemonDir)
	assert.Equal(t, expected, dir)
}

func TestGetDaemonURL(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "daemon_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	homeEnvVar := getHomeEnvVar()
	origHome := os.Getenv(homeEnvVar)
	err = os.Setenv(homeEnvVar, tempDir)
	require.NoError(t, err)
	defer os.Setenv(homeEnvVar, origHome)

	daemonDir := filepath.Join(tempDir, ".evergreen-local")
	err = os.MkdirAll(daemonDir, 0755)
	require.NoError(t, err)

	t.Run("daemon not running", func(t *testing.T) {
		url, err := getDaemonURL()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "daemon not running")
		assert.Empty(t, url)
	})

	t.Run("invalid port file", func(t *testing.T) {
		portFile := filepath.Join(daemonDir, daemonPortFile)
		err := os.WriteFile(portFile, []byte("invalid"), 0644)
		require.NoError(t, err)

		url, err := getDaemonURL()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid port")
		assert.Empty(t, url)
	})

	t.Run("daemon not responding", func(t *testing.T) {
		portFile := filepath.Join(daemonDir, daemonPortFile)
		err := os.WriteFile(portFile, []byte("65535"), 0644)
		require.NoError(t, err)

		url, err := getDaemonURL()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "daemon not responding")
		assert.Empty(t, url)
	})

	t.Run("daemon running", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer server.Close()

		var port int
		_, err := fmt.Sscanf(server.URL, "http://127.0.0.1:%d", &port)
		require.NoError(t, err)

		portFile := filepath.Join(daemonDir, daemonPortFile)
		err = os.WriteFile(portFile, []byte(fmt.Sprintf("%d", port)), 0644)
		require.NoError(t, err)

		url, err := getDaemonURL()
		assert.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("http://localhost:%d", port), url)
	})
}

func TestPostJSON(t *testing.T) {
	t.Run("successful request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"success": true, "value": 42}`)
		}))
		defer server.Close()

		body := map[string]string{"key": "value"}
		result, err := postJSON(server.URL, body)
		require.NoError(t, err)
		assert.True(t, result["success"].(bool))
		assert.Equal(t, float64(42), result["value"])
	})

	t.Run("nil body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"success": true}`)
		}))
		defer server.Close()

		result, err := postJSON(server.URL, nil)
		require.NoError(t, err)
		assert.True(t, result["success"].(bool))
	})
}

func TestHandleHealth(t *testing.T) {
	daemon := newLocalDaemonREST(9090, &ClientSettings{})

	req, err := http.NewRequest("GET", "/health", nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	daemon.handleHealth(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)

	var response map[string]bool
	err = json.NewDecoder(recorder.Body).Decode(&response)
	require.NoError(t, err)
	assert.True(t, response["healthy"])
}

func TestHandleLoadConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_config")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "test.yml")
	clientConfigPath := filepath.Join(tempDir, ".evergreen-local.yml")
	clientConfigContent := `
task_id: ""
`
	configContent := `
tasks:
  - name: test_task
    commands:
      - command: shell.exec
        params:
          script: echo "test"
  - name: another_task
    commands:
      - command: shell.exec
        params:
          script: echo "another"

buildvariants:
  - name: test_variant
    tasks:
      - name: test_task
  - name: another_variant
    tasks:
      - name: another_task
  - name: third_variant
    tasks:
      - name: test_task
`
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)
	err = os.WriteFile(clientConfigPath, []byte(clientConfigContent), 0644)
	require.NoError(t, err)

	daemon := newLocalDaemonREST(9090, &ClientSettings{OAuth: OAuth{AccessToken: "mock_oauth_token"}, APIServerHost: "http://localhost.com"})

	reqBody := map[string]string{"config_path": configPath}
	jsonBody, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", "/config/load", bytes.NewReader(jsonBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	daemon.handleLoadConfig(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)

	var response map[string]interface{}
	err = json.NewDecoder(recorder.Body).Decode(&response)
	require.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.Equal(t, float64(2), response["task_count"])
	assert.Equal(t, float64(3), response["variant_count"])
}

func TestWriteDaemonInfo(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "daemon_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	homeEnvVar := getHomeEnvVar()
	origHome := os.Getenv(homeEnvVar)
	err = os.Setenv(homeEnvVar, tempDir)
	require.NoError(t, err)
	defer os.Setenv(homeEnvVar, origHome)

	daemon := newLocalDaemonREST(9090, &ClientSettings{})
	err = daemon.writeDaemonInfo()
	require.NoError(t, err)

	daemonDir := filepath.Join(tempDir, ".evergreen-local")
	assert.DirExists(t, daemonDir)

	pidFile := filepath.Join(daemonDir, "daemon.pid")
	assert.FileExists(t, pidFile)

	portFile := filepath.Join(daemonDir, "daemon.port")
	assert.FileExists(t, portFile)

	portContent, err := os.ReadFile(portFile)
	require.NoError(t, err)
	assert.Equal(t, "9090", string(portContent))
}

func TestRouterSetup(t *testing.T) {
	daemon := newLocalDaemonREST(9090, &ClientSettings{})
	router := mux.NewRouter()
	router.HandleFunc("/health", daemon.handleHealth).Methods("GET")
	router.HandleFunc("/config/load", daemon.handleLoadConfig).Methods("POST")

	req, err := http.NewRequest("GET", "/health", nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestSelectTaskCmd(t *testing.T) {
	t.Run("no task name provided", func(t *testing.T) {
		app := cli.NewApp()
		set := flag.NewFlagSet("test", 0)
		c := cli.NewContext(app, set, nil)
		err := selectTaskCmd(c)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task name required")
	})

	t.Run("successful task selection", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health":
				w.WriteHeader(http.StatusOK)
			case "/task/select":
				var reqBody map[string]string
				require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
				assert.Equal(t, "test_task", reqBody["task_name"])
				require.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
					"success":    true,
					"step_count": 5,
				}))
			}
		}))
		defer server.Close()

		tempDir, err := os.MkdirTemp("", "select_task_test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		homeEnvVar := getHomeEnvVar()
		origHome := os.Getenv(homeEnvVar)
		err = os.Setenv(homeEnvVar, tempDir)
		require.NoError(t, err)
		defer os.Setenv(homeEnvVar, origHome)

		daemonDir := filepath.Join(tempDir, ".evergreen-local")
		err = os.MkdirAll(daemonDir, 0755)
		require.NoError(t, err)

		var port int
		_, err = fmt.Sscanf(server.URL, "http://127.0.0.1:%d", &port)
		require.NoError(t, err)

		portFile := filepath.Join(daemonDir, "daemon.port")
		err = os.WriteFile(portFile, []byte(fmt.Sprintf("%d", port)), 0644)
		require.NoError(t, err)

		app := cli.NewApp()
		oldStdout := os.Stdout

		set := flag.NewFlagSet("test", 0)
		require.NoError(t, set.Parse([]string{"test_task"}))
		c := cli.NewContext(app, set, nil)

		err = selectTaskCmd(c)
		os.Stdout = oldStdout
		assert.NoError(t, err)
	})
}
