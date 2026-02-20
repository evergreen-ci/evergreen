package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// APIParams includes information necessary to set up a smoke test app server
// and make authenticated requests to it.
type APIParams struct {
	// EVGHome is the local directory containing the Evergreen repo.
	EVGHome string
	// CLIPath is the path to the CLI executable.
	CLIPath string
	// AppServerURL is the URL where the smoke test app server is set up.
	AppServerURL string
	// AppConfigPath is the path to the file containing the app server admin
	// settings.
	AppConfigPath string
	// Username is the name of the user to make authenticated requests.
	Username string
	// APIKey is the API key associated with the user to make authenticated
	// requests.
	APIKey string
}

// GetAPIParamsFromEnv gets the parameters for the smoke test necessary to make
// API requests to the app server from the environment. It sets defaults where
// possible.
// Note that the default data depends on the setup test data for the smoke test.
func GetAPIParamsFromEnv(t *testing.T, evgHome string) APIParams {
	cliPath := os.Getenv("CLI_PATH")
	if cliPath == "" {
		cliPath = filepath.Join(evgHome, "clients", runtime.GOOS+"_"+runtime.GOARCH, "evergreen")
	}

	appServerURL := os.Getenv("APP_SERVER_URL")
	if appServerURL == "" {
		appServerURL = "http://localhost:9090"
	}

	appConfigPath := os.Getenv("APP_CONFIG_PATH")
	if appConfigPath == "" {
		appConfigPath = filepath.Join(evgHome, "smoke", "internal", "testdata", "admin_settings.yml")
	}

	username := os.Getenv("USERNAME")
	if username == "" {
		username = "admin"
	}

	apikey := os.Getenv("APIKEY")
	if apikey == "" {
		apikey = "abb623665fdbf368a1db980dde6ee0f0"
	}

	return APIParams{
		EVGHome:       evgHome,
		CLIPath:       cliPath,
		AppServerURL:  appServerURL,
		AppConfigPath: appConfigPath,
		Username:      username,
		APIKey:        apikey,
	}
}

// WaitForEvergreen waits for the Evergreen app server to be up and accepting
// requests.
func WaitForEvergreen(t *testing.T, appServerURL string, client *http.Client) {
	const attempts = 10
	for i := 0; i < attempts; i++ {
		grip.Infof("Checking if Evergreen is up. (%d/%d)", i, attempts)
		resp, err := client.Get(appServerURL)
		if err != nil {
			grip.Error(errors.Wrap(err, "connecting to Evergreen"))
			time.Sleep(time.Second)
			continue
		}
		resp.Body.Close()

		grip.Info("Evergreen is up.")

		return
	}

	require.FailNow(t, "ran out of attempts to wait for Evergreen", "Evergreen app server was not up after %d check attempts.", attempts)
}

// CheckTaskStatusAndLogs checks that all the expected tasks are finished,
// succeeded, and performed the expected operations based on the task log
// contents.
func CheckTaskStatusAndLogs(ctx context.Context, t *testing.T, params APIParams, client *http.Client, mode globals.Mode, tasks []string) {
	grip.Infof("Checking task status and task logs for tasks: %s", strings.Join(tasks, ", "))

	const maxTaskCheckAttempts = 40
	nextTaskToCheckIdx := 0
	attempt := 1

	// Poll the app server until the task is finished and check its task
	// logs for the expected results.
	// It's worth noting there that there is a substantial amount of
	// heavy-lifting being done here by the Evergreen app server and agent
	// under the covers. In the background, the app server must run the
	// scheduler to create a task queue to run the new tasks, the agent must
	// pick up those tasks from that task queue, and the agent must run
	// those tasks and report the result back to the app server. If any of
	// those operations go wrong (e.g. due to a modification to the app
	// server, agent, or smoke configuration files), these checks can fail.

	for _, taskID := range tasks[nextTaskToCheckIdx:] {
		grip.Infof("Checking %d remaining task(s). (%d/%d)", len(tasks)-nextTaskToCheckIdx, attempt, maxTaskCheckAttempts)

		task, err := getTaskInfo(ctx, params, client, taskID)
		require.NoError(t, err, "should be able to get task info")

		t.Run(task.DisplayName, func(t *testing.T) {
			for attempt < maxTaskCheckAttempts {
				time.Sleep(10 * time.Second)

				task, err := getTaskInfo(ctx, params, client, taskID)
				require.NoError(t, err, "should be able to get task info")

				if !evergreen.IsFinishedTaskStatus(task.Status) {
					grip.Infof("Found task '%s' is not yet finished and has status '%s' (expected '%s').", taskID, task.Status, evergreen.TaskSucceeded)
					attempt++
					continue
				}

				assert.Equal(t, evergreen.TaskSucceeded, task.Status, "task must succeed")
				getAndCheckTaskLog(ctx, t, params, client, mode, *task)

				grip.Infof("Successfully checked task '%s'", taskID)

				nextTaskToCheckIdx = nextTaskToCheckIdx + 1

				return
			}
		})
	}

	if nextTaskToCheckIdx >= len(tasks) {
		grip.Infof("Successfully checked %d %s task(s) and their task logs.", len(tasks), string(mode))
		return
	}

	require.FailNow(t, "ran out of attempts to check task statuses and task logs",
		"task status and task log checks were incomplete after %d attempts - "+
			"this might indicate an underlying issue with the app server, the agent, "+
			"or the smoke test's configuration setup", maxTaskCheckAttempts)
}

// smokeAPITask represents part of a task from the REST API for use in the smoke
// test.
type smokeAPITask struct {
	DisplayName string            `json:"display_name"`
	Status      string            `json:"status"`
	Logs        map[string]string `json:"logs"`
}

// getTaskInfo gets basic information about the current status and task logs for
// the given task ID from the REST API.
func getTaskInfo(ctx context.Context, params APIParams, client *http.Client, taskID string) (*smokeAPITask, error) {
	grip.Infof("Checking information for task '%s'.", taskID)

	body, err := MakeSmokeRequest(ctx, params, http.MethodGet, client, fmt.Sprintf("/rest/v2/tasks/%s", taskID))
	if err != nil {
		return nil, errors.Wrap(err, "getting task info")
	}

	task := smokeAPITask{}
	if err := json.Unmarshal(body, &task); err != nil {
		return nil, errors.Wrap(err, "unmarshalling JSON response body into task")
	}

	return &task, nil
}

// getAndCheckTaskLog gets the task logs from the task log URL and checks that
// it has the expected content, indicating that the task executed the commands
// properly.
func getAndCheckTaskLog(ctx context.Context, t *testing.T, params APIParams, client *http.Client, mode globals.Mode, task smokeAPITask) {
	// retry for *slightly* delayed logger closing
	const taskLogCheckAttempts = 3
	for i := 0; i < taskLogCheckAttempts; i++ {
		grip.Infof("Checking for task log from URL %s. (%d/%d)", task.Logs["task_log"], i+1, taskLogCheckAttempts)
		body, err := MakeSmokeRequest(ctx, params, http.MethodGet, client, task.Logs["task_log"]+"&text=true")
		if err != nil {
			grip.Error(errors.Wrap(err, "getting task log data"))
			continue
		}

		checkTaskLogContent(t, task.DisplayName, body, mode)

		grip.Infof("Successfully checked task logs for task '%s'", task.DisplayName)

		return
	}

	require.FailNow(t, "ran out of attempts to check task logs", "task log check failed after %d attempts", taskLogCheckAttempts)
}

// checkTaskLogContent compares the expected result of running the smoke test
// project YAML (project.yml) against the actual task log's text.
func checkTaskLogContent(t *testing.T, taskName string, body []byte, mode globals.Mode) {
	grip.Infof("Checking task logs for task named '%s'", taskName)

	page := string(body)

	// Validate that task contains task completed message
	require.Contains(t, page, "Task completed - SUCCESS", "task succeeded message not found in logs")

	// Note that these checks have a direct dependency on the task configuration
	// in the smoke test's project YAML (project.yml).
	const generatorTaskName = "host_smoke_test_generate_task"
	const generatedTaskName = "task_to_add_via_generator"
	const firstTaskGroupTaskName = "host_smoke_test_first_task_in_task_group"
	const lastTaskGroupTaskName = "host_smoke_test_fourth_task_in_task_group"

	const setupGroupLog = "smoke test is running the setup group"
	const teardownGroupLog = "smoke test is running the teardown group"

	if mode == globals.HostMode {
		switch taskName {
		case generatorTaskName:
			require.Contains(t, page, "Finished command 'generate.tasks'", "generator task '%s' should have logged generate.tasks command ran", taskName)
			return
		case generatedTaskName:
			require.Contains(t, page, "generated_task", "task '%s' should have logged generated task output", taskName)
			return
		case firstTaskGroupTaskName:
			const firstTaskGroupTaskLog = "smoke test is running the first task in the task group"
			require.Contains(t, page, firstTaskGroupTaskLog, "first task in the task group '%s' should log expected output", taskName)
			require.Contains(t, page, setupGroupLog, "first task in the task group '%s' should have the setup group log", taskName)
		case lastTaskGroupTaskName:
			const lastTaskGroupTaskLog = "smoke test is running the fourth task in the task group"
			require.Contains(t, page, lastTaskGroupTaskLog, "last task in the task group '%s' should log expected output", taskName)
			require.Contains(t, page, teardownGroupLog, "last task in the task group '%s' should have the teardown group log", taskName)
		default:
			// For all task group tasks that are neither the first or last,
			// check that they do not run the setup group or teardown group.
			require.NotContains(t, page, setupGroupLog, "task '%s' should not have the setup group log because it is not the first task in the task group", taskName)

			require.NotContains(t, page, teardownGroupLog, "task '%s' should not have the teardown group log because it is not the last task in the task group", taskName)
		}

		if strings.Contains(taskName, "task_group") {
			// For all tasks in the task group, check that they run the setup
			// task and teardown task.
			const setupTaskLog = "smoke test is running the setup task"
			require.Contains(t, page, setupTaskLog, "task '%s' should log setup task log since it runs for every task in the task group", taskName)

			const teardownTaskLog = "smoke test is running the teardown task"
			require.Contains(t, page, teardownTaskLog, "teardown task should run for every task in the task group")
		}

	}
}

// MakeSmokeRequest sends an authenticated smoke request to the smoke test app
// server.
func MakeSmokeRequest(ctx context.Context, params APIParams, method string, client *http.Client, url string) ([]byte, error) {
	grip.Infof("Getting endpoint '%s'", url)

	if !strings.HasPrefix(url, params.AppServerURL) {
		url = strings.Join([]string{strings.TrimSuffix(params.AppServerURL, "/"), strings.TrimPrefix(url, "/")}, "/")
	}

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "making request for URL '%s'", url)
	}
	req.Header.Add(evergreen.APIUserHeader, params.Username)
	req.Header.Add(evergreen.APIKeyHeader, params.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "getting endpoint '%s'", url)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}
	if resp.StatusCode >= 400 {
		return body, errors.Errorf("got HTTP response code %d with error %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// SmokeRunBinary runs a smoke test Evergreen binary in the background. The name
// indicates the process name so that it can be tracked in output logs. The
// given wd is used as the working directory for the command.
func SmokeRunBinary(ctx context.Context, name, wd, bin string, cmdParts ...string) (jasper.Process, error) {
	grip.Infof("Running command: %s", append([]string{bin}, cmdParts...))

	cmdSender := send.NewWriterSender(send.MakeNative())
	cmdSender.SetName(name)

	proc, err := jasper.NewProcess(ctx, &options.Create{
		Args:             append([]string{bin}, cmdParts...),
		Environment:      map[string]string{"EVGHOME": wd},
		WorkingDirectory: wd,
		Output: options.Output{
			Output: cmdSender,
			Error:  cmdSender,
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "creating Jasper process")
	}

	return proc, nil
}

// StartAppServer starts the smoke test app server.
func StartAppServer(ctx context.Context, t *testing.T, params APIParams) jasper.Process {
	grip.Info("Starting smoke test app server.")

	appServerCmd, err := SmokeRunBinary(ctx,
		"smoke-app-server",
		params.EVGHome,
		params.CLIPath,
		"service",
		"deploy",
		"start-evergreen",
		"--web",
		fmt.Sprintf("--conf=%s", params.AppConfigPath),
		fmt.Sprintf("--binary=%s", params.CLIPath),
	)
	require.NoError(t, err, "should have started Evergreen smoke test app server")

	grip.Info("Successfully started smoke test app server.")

	return appServerCmd
}

// StartAgent starts the smoke test agent with the given execution mode and
// ID.
func StartAgent(ctx context.Context, t *testing.T, params APIParams, mode globals.Mode, execModeID, execModeSecret string) jasper.Process {
	grip.Info("Starting smoke test agent.")

	agentCmd, err := SmokeRunBinary(ctx,
		"smoke-agent",
		params.EVGHome,
		params.CLIPath,
		"service",
		"deploy",
		"start-evergreen",
		"--agent",
		fmt.Sprintf("--mode=%s", mode),
		fmt.Sprintf("--exec_mode_id=%s", execModeID),
		fmt.Sprintf("--exec_mode_secret=%s", execModeSecret),
		fmt.Sprintf("--api_server=%s", params.AppServerURL),
		fmt.Sprintf("--binary=%s", params.CLIPath),
	)
	require.NoError(t, err, "should have started Evergreen agent")

	grip.Info("Successfully started smoke test agent.")

	return agentCmd
}
