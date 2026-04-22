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

	"github.com/evergreen-ci/evergreen/agent/taskexec"
	"github.com/evergreen-ci/evergreen/rest/client"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/gorilla/mux"
	"github.com/mitchellh/go-homedir"
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
	homedir.Reset()
	defer func() {
		os.Setenv(homeEnvVar, origHome)
		homedir.Reset()
	}()

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

	t.Run("MissingOAuthTokenShouldReturnUnauthorized", func(t *testing.T) {
		reqBody := map[string]string{"config_path": configPath}
		jsonBody, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", "/config/load", bytes.NewReader(jsonBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		recorder := httptest.NewRecorder()
		daemon.handleLoadConfig(recorder, req)

		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})

	reqBody := map[string]string{"config_path": configPath, "oauth_token": "mock_oauth_token"}
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
	homedir.Reset()
	defer func() {
		os.Setenv(homeEnvVar, origHome)
		homedir.Reset()
	}()

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
	d := newLocalDaemonREST(9090, &ClientSettings{})
	router := mux.NewRouter()
	router.HandleFunc("/health", d.handleHealth).Methods("GET")
	router.HandleFunc("/config/load", d.handleLoadConfig).Methods("POST")
	router.HandleFunc("/task/select", d.handleSelectTask).Methods("POST")
	router.HandleFunc("/step/next", d.handleStepNext).Methods("POST")
	router.HandleFunc("/step/run-all", d.handleRunAll).Methods("POST")
	router.HandleFunc("/step/run-until/{index}", d.handleRunUntil).Methods("POST")
	router.HandleFunc("/step/jump/{index}", d.handleJumpTo).Methods("POST")

	req, err := http.NewRequest("GET", "/health", nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestJumpToCmd(t *testing.T) {
	t.Run("no step index provided", func(t *testing.T) {
		app := cli.NewApp()
		set := flag.NewFlagSet("test", 0)
		c := cli.NewContext(app, set, nil)
		err := jumpToCmd(c)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "step number required")
	})

	t.Run("invalid step index", func(t *testing.T) {
		app := cli.NewApp()
		set := flag.NewFlagSet("test", 0)
		require.NoError(t, set.Parse([]string{"notanumber"}))
		c := cli.NewContext(app, set, nil)
		err := jumpToCmd(c)
		assert.Error(t, err)
	})

	t.Run("successful jump", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health":
				w.WriteHeader(http.StatusOK)
			case "/step/jump/3":
				require.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
					"success":      true,
					"current_step": 3,
				}))
			}
		}))
		defer server.Close()

		tempDir, err := os.MkdirTemp("", "jump_to_test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		homeEnvVar := getHomeEnvVar()
		origHome := os.Getenv(homeEnvVar)
		err = os.Setenv(homeEnvVar, tempDir)
		require.NoError(t, err)
		homedir.Reset()
		defer func() {
			os.Setenv(homeEnvVar, origHome)
			homedir.Reset()
		}()

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
		set := flag.NewFlagSet("test", 0)
		require.NoError(t, set.Parse([]string{"3"}))
		c := cli.NewContext(app, set, nil)

		err = jumpToCmd(c)
		assert.NoError(t, err)
	})
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
		homedir.Reset()
		defer func() {
			os.Setenv(homeEnvVar, origHome)
			homedir.Reset()
		}()

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

func TestWaitForDaemon(t *testing.T) {
	t.Run("HealthyDaemonShouldSucceed", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		var port int
		_, err := fmt.Sscanf(server.URL, "http://127.0.0.1:%d", &port)
		require.NoError(t, err)

		err = waitForDaemon(port)
		assert.NoError(t, err)
	})

	t.Run("UnhealthyDaemonShouldError", func(t *testing.T) {
		err := waitForDaemon(0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "did not become healthy")
	})
}

func TestReadAndRenderStream(t *testing.T) {
	t.Run("ParsesNDJSONStream", func(t *testing.T) {
		success := true
		durationMs := int64(1200)
		nextStep := 4
		lines := []taskexec.StreamLine{
			{Channel: taskexec.ExecChannel, Step: 3, Message: "Running 'shell.exec'"},
			{Channel: taskexec.TaskChannel, Step: 3, Message: "+ go test -v ./..."},
			{Channel: taskexec.TaskChannel, Step: 3, Message: "PASS"},
			{Channel: taskexec.DoneChannel, Step: 3, Success: &success, DurationMs: &durationMs, NextStep: &nextStep},
		}

		var input bytes.Buffer
		for _, line := range lines {
			data, err := json.Marshal(line)
			require.NoError(t, err)
			input.Write(data)
			input.WriteByte('\n')
		}

		var output bytes.Buffer
		result, err := readAndRenderStream(&input, &output)
		require.NoError(t, err)

		assert.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Empty(t, result.Error)

		rendered := output.String()
		assert.Contains(t, rendered, "Running 'shell.exec'")
		assert.Contains(t, rendered, "+ go test -v ./...")
		assert.Contains(t, rendered, "PASS")
	})

	t.Run("HandlesFailure", func(t *testing.T) {
		success := false
		durationMs := int64(500)
		nextStep := 2
		lines := []taskexec.StreamLine{
			{Channel: taskexec.TaskChannel, Step: 2, Message: "error output"},
			{Channel: taskexec.DoneChannel, Step: 2, Success: &success, DurationMs: &durationMs, NextStep: &nextStep, Error: "exit code 1"},
		}

		var input bytes.Buffer
		for _, line := range lines {
			data, _ := json.Marshal(line)
			input.Write(data)
			input.WriteByte('\n')
		}

		var output bytes.Buffer
		result, err := readAndRenderStream(&input, &output)
		require.NoError(t, err)

		assert.NotNil(t, result)
		assert.False(t, result.Success)
		assert.Equal(t, "exit code 1", result.Error)
	})
}

func TestValidateDebugLocal(t *testing.T) {
	ctx := t.Context()

	validConf := &ClientSettings{
		APIServerHost: "http://localhost",
		User:          "testuser",
	}

	t.Run("EmptyTaskIDShouldError", func(t *testing.T) {
		mockClient = &client.Mock{}
		defer func() { mockClient = nil }()

		err := validateDebugLocal(ctx, validConf, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "task-id flag is required")
	})

	t.Run("ServiceFlagsDisabledShouldError", func(t *testing.T) {
		mockClient = &client.Mock{
			MockServiceFlags: &restmodel.APIServiceFlags{
				DebugSpawnHostDisabled: true,
			},
		}
		defer func() { mockClient = nil }()

		err := validateDebugLocal(ctx, validConf, "task123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "debug spawn hosts currently disabled")
	})
}
