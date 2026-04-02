package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/evergreen-ci/evergreen/agent/taskexec"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	daemonDir      = ".evergreen-local"
	daemonPortFile = "daemon.port"
	daemonPIDFile  = "daemon.pid"
	daemonLogFile  = "daemon.log"

	daemonEnvVar = "_EVERGREEN_DAEMON_CHILD"

	stepFlagName  = "step"
	setupFlagName = "setup"
	tailFlagName  = "tail"
)

// getRootContext walks up the cli.Context chain to find the root context,
// which holds global flags like the config file path.
func getRootContext(c *cli.Context) *cli.Context {
	root := c
	for p := root.Parent(); p != nil && p != root; p = root.Parent() {
		root = p
	}
	return root
}

// getDaemonDir returns the full path to the daemon directory
func getDaemonDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, "getting user home directory")
	}
	return filepath.Join(homeDir, daemonDir), nil
}

// getDaemonURL retrieves the URL of a running daemon process.
// It expects a daemon to be running and reads the port from ~/.evergreen-local/daemon.port.
// The daemon writes this file when it starts to allow clients to discover its port.
// Returns an error if the daemon is not running or not responding.
func getDaemonURL() (string, error) {
	dir, err := getDaemonDir()
	if err != nil {
		return "", err
	}

	portFile := filepath.Join(dir, daemonPortFile)
	data, err := os.ReadFile(portFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("daemon not running (port file not found)")
		}
		return "", err
	}

	port, err := strconv.Atoi(string(data))
	if err != nil {
		return "", errors.Wrap(err, "invalid port in daemon.port file")
	}

	url := fmt.Sprintf("http://localhost:%d", port)
	resp, err := http.Get(url + "/health")
	if err != nil {
		return "", errors.New("daemon not responding (may have crashed)")
	}
	defer resp.Body.Close()

	return url, nil
}

// Debug returns the "debug" command for running Evergreen tasks locally.
func Debug() cli.Command {
	return cli.Command{
		Name:  "debug",
		Usage: "debug Evergreen tasks locally",
		Subcommands: []cli.Command{
			{
				Name:  "daemon",
				Usage: "Manage the debug daemon",
				Subcommands: []cli.Command{
					{
						Name:  "start",
						Usage: "Start the debug daemon",
						Flags: []cli.Flag{
							cli.IntFlag{
								Name:  "port, p",
								Usage: "Port to run the daemon on",
								Value: 9090,
							},
						},
						Action: startDebugDaemonCmd,
					},
					{
						Name:   "stop",
						Usage:  "Stop the debug daemon",
						Action: stopDebugDaemonCmd,
					},
					{
						Name:   "status",
						Usage:  "Check daemon status",
						Action: daemonStatusCmd,
					},
				},
			},
			{
				Name:      "load",
				Usage:     "Load a configuration file",
				ArgsUsage: "<config.yml>",
				Action:    loadConfigCmd,
			},
			{
				Name:      "select",
				Usage:     "Select a task for debugging",
				ArgsUsage: "<task_name>",
				Action:    selectTaskCmd,
			},
			{
				Name:   "next",
				Usage:  "Execute the next step",
				Action: stepNextCmd,
			},
			{
				Name:   "run-all",
				Usage:  "Run all remaining steps",
				Action: runAllCmd,
			},
			{
				Name:      "run-until",
				Usage:     "Run until a specific step",
				ArgsUsage: "<step_number>",
				Action:    runUntilCmd,
			},
			{
				Name:   "list-steps",
				Usage:  "List all steps in the current task",
				Action: listStepsCmd,
			},
			{
				Name:      "set-var",
				Usage:     "Set a custom variable",
				ArgsUsage: "<key>=<value>",
				Action:    setVariableCmd,
			},
			{
				Name:      "jump",
				Usage:     "Jump to a specific step without executing",
				ArgsUsage: "<step_number>",
				Action:    jumpToCmd,
			},
			{
				Name:  "logs",
				Usage: "View debug session logs",
				Flags: []cli.Flag{
					cli.StringFlag{
						Name:  stepFlagName,
						Usage: "Show logs from a specific step only (e.g. '3', '2.1', 'pre:1')",
					},
					cli.BoolFlag{
						Name:  setupFlagName,
						Usage: "Show setup phase logs instead of session logs",
					},
					cli.IntFlag{
						Name:  tailFlagName,
						Usage: "Show only the last N lines",
					},
				},
				Action: viewLogsCmd,
			},
		},
	}
}

// validateDebugSpawnHost validates that the current environment is authorized
// to run debug spawn host commands. It checks service flags and that the host
// was spawned by a task.
func validateDebugSpawnHost(ctx context.Context, conf *ClientSettings) error {
	if conf.SpawnHostID == "" {
		return errors.New("could not find spawn host ID in configuration; this command must be run from a spawn host")
	}

	restClient, err := conf.setupRestCommunicator(ctx, false)
	if err != nil {
		return errors.Wrap(err, "setting up REST communicator")
	}
	defer restClient.Close()

	flags, err := restClient.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "getting service flags for debug spawn host")
	}

	if flags.DebugSpawnHostDisabled {
		return errors.New("debug spawn hosts currently disabled")
	}

	currentHost, err := restClient.GetSpawnHost(ctx, conf.SpawnHostID)
	if err != nil {
		return errors.Wrapf(err, "getting current host '%s' for debug spawn host", conf.SpawnHostID)
	}

	taskID := utility.FromStringPtr(currentHost.ProvisionOptions.TaskID)
	if taskID == "" {
		return errors.New("only hosts spawned by tasks are allowed to use debugger")
	}

	if conf.ProjectID == "" {
		return errors.New("project ID not found in configuration; debug spawn host validation requires project information")
	}

	project, err := restClient.GetProject(ctx, conf.ProjectID)
	if err != nil {
		return errors.Wrapf(err, "getting project '%s' settings", conf.ProjectID)
	}
	if project == nil {
		return errors.Errorf("project '%s' not found", conf.ProjectID)
	}

	debugSpawnHostsDisabled := utility.FromBoolPtr(project.DebugSpawnHostsDisabled)
	if debugSpawnHostsDisabled {
		return errors.Errorf("debug spawn hosts are disabled for project '%s'", conf.ProjectID)
	}

	return nil
}

// startDebugDaemonCmd starts the debug daemon
func startDebugDaemonCmd(c *cli.Context) error {
	port := c.Int("port")

	if _, err := getDaemonURL(); err == nil {
		return errors.New("daemon is already running")
	}

	if os.Getenv(daemonEnvVar) == "1" {
		return runDaemonServer(c, port)
	}

	return forkDaemon(c, port)
}

func runDaemonServer(c *cli.Context, port int) error {
	confPath := getRootContext(c).String(ConfFlagName)
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrapf(err, "finding configuration at '%s'", confPath)
	}

	grip.Infof(context.Background(), "Starting daemon on port %d...", port)

	daemon := newLocalDaemonREST(port, conf)
	return daemon.Start()
}

// forkDaemon re-execs the current binary as a background process with the
// sentinel environment variable set. It redirects the child's stdout/stderr to
// a log file and waits for the daemon's /health endpoint to respond before
// returning.
func forkDaemon(c *cli.Context, port int) error {
	execPath, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "resolving executable path")
	}

	dir, err := getDaemonDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(err, "creating daemon directory")
	}

	logPath := filepath.Join(dir, daemonLogFile)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return errors.Wrap(err, "opening daemon log file")
	}

	// Build child command arguments, forwarding global flags.
	args := []string{"debug", "daemon", "start", "--port", fmt.Sprintf("%d", port)}
	if confPath := getRootContext(c).String(ConfFlagName); confPath != "" {
		args = append([]string{"--" + ConfFlagName, confPath}, args...)
	}

	cmd := exec.Command(execPath, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), daemonEnvVar+"=1")
	cmd.SysProcAttr = daemonSysProcAttr()

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return errors.Wrap(err, "starting daemon process")
	}
	logFile.Close()

	pid := cmd.Process.Pid

	if err := waitForDaemon(port); err != nil {
		// Attempt to kill the orphaned child process.
		_ = cmd.Process.Signal(syscall.SIGTERM)
		return errors.Wrapf(err, "daemon failed to start (check logs at %s)", logPath)
	}

	grip.Infof(context.Background(), "Daemon started (PID %d, port %d)", pid, port)
	grip.Infof(context.Background(), "Logs: %s", logPath)
	return nil
}

// waitForDaemon polls the daemon's /health endpoint until it responds
// successfully or the timeout is reached.
func waitForDaemon(port int) error {
	healthURL := fmt.Sprintf("http://localhost:%d/health", port)
	client := &http.Client{Timeout: time.Second}

	const maxAttempts = 30
	const pollInterval = 200 * time.Millisecond

	for i := 0; i < maxAttempts; i++ {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(pollInterval)
	}

	timeout := time.Duration(maxAttempts) * pollInterval
	return errors.Errorf("daemon did not become healthy within %s", timeout)
}

// stopDebugDaemonCmd stops the debug daemon
func stopDebugDaemonCmd(c *cli.Context) error {
	dir, err := getDaemonDir()
	if err != nil {
		return err
	}

	pidFile := filepath.Join(dir, daemonPIDFile)
	portFile := filepath.Join(dir, daemonPortFile)
	logFile := filepath.Join(dir, daemonLogFile)

	defer func() {
		for _, f := range []string{pidFile, portFile, logFile} {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				grip.Warning(context.Background(), errors.Wrapf(err, "removing %s", f))
			}
		}
	}()

	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			grip.Info(context.Background(), "Daemon is not running")
			return nil
		}
		return errors.Wrap(err, "reading PID file")
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return errors.Wrap(err, "invalid PID in daemon.pid file")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return errors.Wrap(err, "finding daemon process")
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead
		grip.Info(context.Background(), "Daemon process not found (may have already stopped)")
	} else {
		grip.Info(context.Background(), "Daemon stopped")
	}
	return nil
}

// daemonStatusResponse is the typed response from the /status endpoint.
type daemonStatusResponse struct {
	Healthy      bool   `json:"healthy"`
	TaskSelected bool   `json:"task_selected"`
	CurrentStep  int    `json:"current_step"`
	TotalSteps   int    `json:"total_steps"`
	SelectedTask string `json:"selected_task"`
}

// daemonStatusCmd checks the debug daemon status.
func daemonStatusCmd(c *cli.Context) error {
	url, err := getDaemonURL()
	if err != nil {
		grip.Info(context.Background(), "Daemon is not running")
		return nil
	}

	resp, err := http.Get(url + "/status")
	if err != nil {
		grip.Info(context.Background(), "Daemon is running but not responding to status check")
		return nil
	}
	defer resp.Body.Close()

	var status daemonStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return errors.Wrap(err, "decoding daemon status response")
	}

	grip.Info(context.Background(), "Daemon is running")

	if status.TaskSelected {
		grip.Infof(context.Background(), "Task: %s (step %d/%d)", status.SelectedTask, status.CurrentStep, status.TotalSteps)
	}

	return nil
}

// loadConfigCmd loads a configuration file. It handles OAuth authentication
// and spawn host validation before sending the config to the daemon.
func loadConfigCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("config file path required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	configPath, err := filepath.Abs(c.Args().Get(0))
	if err != nil {
		return errors.Wrap(err, "resolving config file path")
	}

	confPath := getRootContext(c).String(ConfFlagName)
	conf, err := NewClientSettings(confPath)
	if err != nil {
		return errors.Wrapf(err, "finding configuration at '%s'", confPath)
	}

	if err := validateDebugSpawnHost(ctx, conf); err != nil {
		return err
	}

	if err := conf.SetOAuthToken(ctx); err != nil {
		return errors.Wrap(err, "obtaining OAuth token")
	}

	url, err := getDaemonURL()
	if err != nil {
		return errors.Wrap(err, "connecting to daemon")
	}

	reqBody := map[string]string{
		"config_path": configPath,
		"oauth_token": conf.OAuth.AccessToken,
	}
	resp, err := postJSON(url+"/config/load", reqBody)
	if err != nil {
		return errors.Wrap(err, "loading configuration")
	}

	grip.Infof(context.Background(), "Loaded configuration: %s", configPath)
	grip.Infof(context.Background(), "Tasks: %v, Variants: %v", resp["task_count"], resp["variant_count"])

	return nil
}

// selectTaskCmd selects a task for debugging
func selectTaskCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("task name required")
	}

	taskName := c.Args().Get(0)
	// TODO: DEVPROD-26690 Support selecting variant task and apply variant-specific expansions.
	variantName := c.String("variant")

	// Clear previous session logs when selecting a new task.
	if err := taskexec.ClearSessionLogs(); err != nil {
		grip.Warning(context.Background(), errors.Wrap(err, "clearing previous session logs"))
	}

	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	reqBody := map[string]string{
		"task_name":    taskName,
		"variant_name": variantName,
	}

	resp, err := postJSON(url+"/task/select", reqBody)
	if err != nil {
		return err
	}

	grip.Infof(context.Background(), "Selected task: %s\n", taskName)
	if variantName != "" {
		grip.Infof(context.Background(), "Variant: %s\n", variantName)
	}
	grip.Infof(context.Background(), "Total steps: %v\n", resp["step_count"])

	return nil
}

// stepNextCmd executes the next step with streaming output.
func stepNextCmd(c *cli.Context) error {
	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	return postAndStreamStepResponse(url + "/step/next")
}

// runAllCmd runs all remaining steps with streaming output.
func runAllCmd(c *cli.Context) error {
	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	return postAndStreamStepResponse(url + "/step/run-all")
}

const noMoreStepsMessage = "No more steps to execute. You've reached the end of the task commands."

func postAndStreamStepResponse(url string) error {
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return errors.Wrap(err, "sending POST request")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		fmt.Fprintln(os.Stdout, noMoreStepsMessage)
		return nil
	}

	return handleStreamResponse(resp)
}

// runUntilCmd runs until a specific step with streaming output.
func runUntilCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("step number required")
	}

	stepNum := c.Args().Get(0)

	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	return postAndStreamResponse(fmt.Sprintf("%s/step/run-until/%s", url, stepNum), nil)
}

// jumpToCmd jumps to a specific step
func jumpToCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("step number required")
	}

	stepNum := c.Args().Get(0)

	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	resp, err := postJSON(fmt.Sprintf("%s/step/jump/%s", url, stepNum), nil)
	if err != nil {
		return err
	}

	grip.Infof(context.Background(), "Jumped to step %v\n", resp["current_step"])
	return nil
}

// setVariableCmd sets a custom variable.
func setVariableCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("variable assignment required (key=value)")
	}

	// Parse key=value
	assignment := c.Args().Get(0)
	parts := bytes.SplitN([]byte(assignment), []byte("="), 2)
	if len(parts) != 2 {
		return errors.New("invalid format, use key=value")
	}

	key := string(parts[0])
	value := string(parts[1])

	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	reqBody := map[string]string{
		"key":   key,
		"value": value,
	}

	_, err = postJSON(url+"/variable/set", reqBody)
	if err != nil {
		return err
	}

	fmt.Printf("Set variable: %s=%s\n", key, value)
	return nil
}

// listStepsCmd lists all steps
func listStepsCmd(c *cli.Context) error {
	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	resp, err := http.Get(url + "/task/list-steps")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyData, _ := io.ReadAll(resp.Body)
		return errors.Errorf("request failed with status %d: %s", resp.StatusCode, string(bodyData))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	steps := result["steps"].([]interface{})
	currentStep := int(result["current_step"].(float64))

	fmt.Println("Steps:")
	for _, stepRaw := range steps {
		step := stepRaw.(map[string]interface{})
		index := int(step["index"].(float64))
		marker := "  "
		if index == currentStep {
			marker = "→ "
		}

		status := ""
		if step["executed"].(bool) {
			if step["success"].(bool) {
				status = " ✓"
			} else {
				status = " ✗"
			}
		}

		fmt.Printf("%s%s: %s%s\n", marker, step["step_number"], step["display_name"], status)
	}

	return nil
}

// postAndStreamResponse sends a POST request and renders the NDJSON streaming response.
func postAndStreamResponse(url string, body interface{}) error {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return errors.Wrap(err, "marshaling request body")
		}
		reqBody = bytes.NewReader(jsonData)
	}

	resp, err := http.Post(url, "application/json", reqBody)
	if err != nil {
		return errors.Wrap(err, "sending POST request")
	}
	defer resp.Body.Close()

	return handleStreamResponse(resp)
}

func handleStreamResponse(resp *http.Response) error {
	if resp.StatusCode != http.StatusOK {
		bodyData, _ := io.ReadAll(resp.Body)
		return errors.Errorf("request failed with status %d: %s", resp.StatusCode, string(bodyData))
	}

	result, err := readAndRenderStream(resp.Body, os.Stdout)
	if err != nil {
		return errors.Wrap(err, "reading streaming response")
	}

	if !result.Success && result.Error != "" {
		return errors.New(result.Error)
	}

	return nil
}

// viewLogsCmd displays debug session logs from local log files.
func viewLogsCmd(c *cli.Context) error {
	isSetup := c.Bool(setupFlagName)
	stepFilter := c.String(stepFlagName)
	tail := c.Int(tailFlagName)

	lines, err := taskexec.ReadAllLogs(isSetup)
	if err != nil {
		return errors.Wrap(err, "reading logs")
	}

	if stepFilter != "" {
		lines = taskexec.FilterLogLinesByStep(lines, stepFilter)
	}

	if tail > 0 && len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}

	if len(lines) == 0 {
		grip.Info(context.Background(), "No logs found.")
		return nil
	}

	for _, line := range lines {
		fmt.Println(line)
	}

	return nil
}

func postJSON(url string, body interface{}) (map[string]interface{}, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, errors.Wrap(err, "marshaling request body")
		}
		reqBody = bytes.NewReader(jsonData)
	}

	resp, err := http.Post(url, "application/json", reqBody)
	if err != nil {
		return nil, errors.Wrap(err, "sending POST request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyData, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("request failed with status %d: %s", resp.StatusCode, string(bodyData))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.Wrap(err, "decoding response")
	}

	return result, nil
}
