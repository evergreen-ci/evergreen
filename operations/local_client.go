package operations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func getDaemonURL() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	portFile := filepath.Join(homeDir, ".evergreen-local", "daemon.port")
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
	resp.Body.Close()

	return url, nil
}

func DaemonCommands() []cli.Command {
	return []cli.Command{
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
					Action: startDaemonCmd,
				},
				{
					Name:   "stop",
					Usage:  "Stop the debug daemon",
					Action: stopDaemonCmd,
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
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "variant, v",
					Usage: "Build variant to use",
				},
			},
			Action: selectTaskCmd,
		},
		{
			Name:   "list-steps",
			Usage:  "List all steps in the current task",
			Action: listStepsCmd,
		},
		{
			Name:   "next",
			Usage:  "Execute the next step",
			Action: stepNextCmd,
		},
		{
			Name:      "jump",
			Usage:     "Jump to a specific step without executing",
			ArgsUsage: "<step_index>",
			Action:    jumpToCmd,
		},
		{
			Name:      "run-until",
			Usage:     "Run until a specific step",
			ArgsUsage: "<step_index>",
			Action:    runUntilCmd,
		},
		{
			Name:   "run-all",
			Usage:  "Run all remaining steps",
			Action: runAllCmd,
		},
		{
			Name:      "set-var",
			Usage:     "Set a custom variable",
			ArgsUsage: "<key>=<value>",
			Action:    setVariableCmd,
		},
		{
			Name:   "state",
			Usage:  "Show current debug state",
			Action: getStateCmd,
		},
		{
			Name:   "reset",
			Usage:  "Reset the debug state",
			Action: resetCmd,
		},
	}
}

// startDaemonCmd starts the daemon
func startDaemonCmd(c *cli.Context) error {
	port := c.Int("port")

	// Check if daemon is already running
	if _, err := getDaemonURL(); err == nil {
		return errors.New("daemon is already running")
	}

	fmt.Printf("Starting daemon on port %d...\n", port)

	// Start daemon in current process (for testing)
	// In production, you might want to fork a new process
	daemon := NewLocalDaemonREST(port)
	return daemon.Start()
}

// stopDaemonCmd stops the daemon
func stopDaemonCmd(c *cli.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	pidFile := filepath.Join(homeDir, ".evergreen-local", "daemon.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Daemon is not running")
			return nil
		}
		return err
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return errors.Wrap(err, "invalid PID in daemon.pid file")
	}

	// Send SIGTERM to the daemon
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead
		fmt.Println("Daemon process not found (may have already stopped)")
	} else {
		fmt.Println("Daemon stopped")
	}

	// Clean up files
	os.Remove(pidFile)
	os.Remove(filepath.Join(homeDir, ".evergreen-local", "daemon.port"))

	return nil
}

// daemonStatusCmd checks daemon status
func daemonStatusCmd(c *cli.Context) error {
	url, err := getDaemonURL()
	if err != nil {
		fmt.Println("Daemon is not running")
		return nil
	}

	resp, err := http.Get(url + "/state")
	if err != nil {
		fmt.Println("Daemon is not responding")
		return nil
	}
	defer resp.Body.Close()

	var state map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return err
	}

	fmt.Println("Daemon is running")
	fmt.Printf("Loaded config: %s\n", state["loaded_config"])
	fmt.Printf("Selected task: %s\n", state["selected_task"])
	fmt.Printf("Current step: %v/%v\n", state["current_step"], state["total_steps"])

	return nil
}

// loadConfigCmd loads a configuration file
func loadConfigCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("config file path required")
	}

	configPath, err := filepath.Abs(c.Args().Get(0))
	if err != nil {
		return err
	}

	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	reqBody := map[string]string{"config_path": configPath}
	resp, err := postJSON(url+"/config/load", reqBody)
	if err != nil {
		return err
	}

	fmt.Printf("Loaded configuration: %s\n", configPath)
	fmt.Printf("Tasks: %v, Variants: %v\n", resp["task_count"], resp["variant_count"])

	return nil
}

// selectTaskCmd selects a task for debugging
func selectTaskCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("task name required")
	}

	taskName := c.Args().Get(0)
	variantName := c.String("variant")

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

	fmt.Printf("Selected task: %s\n", taskName)
	if variantName != "" {
		fmt.Printf("Variant: %s\n", variantName)
	}
	fmt.Printf("Total steps: %v\n", resp["step_count"])

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

		fmt.Printf("%s%d: %s%s\n", marker, index, step["display_name"], status)
	}

	return nil
}

// stepNextCmd executes the next step
func stepNextCmd(c *cli.Context) error {
	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	resp, err := postJSON(url+"/step/next", nil)
	if err != nil {
		return err
	}

	if resp["success"].(bool) {
		fmt.Printf("Step executed successfully (now at step %v)\n", resp["current_step"])
	} else {
		fmt.Printf("Step failed: %s (now at step %v)\n", resp["error"], resp["current_step"])
	}

	return nil
}

// jumpToCmd jumps to a specific step
func jumpToCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("step index required")
	}

	index, err := strconv.Atoi(c.Args().Get(0))
	if err != nil {
		return errors.New("invalid step index")
	}

	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	resp, err := postJSON(fmt.Sprintf("%s/step/jump/%d", url, index), nil)
	if err != nil {
		return err
	}

	fmt.Printf("Jumped to step %v\n", resp["current_step"])
	return nil
}

// runUntilCmd runs until a specific step
func runUntilCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("step index required")
	}

	index, err := strconv.Atoi(c.Args().Get(0))
	if err != nil {
		return errors.New("invalid step index")
	}

	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	fmt.Printf("Running until step %d...\n", index)
	resp, err := postJSON(fmt.Sprintf("%s/step/run-until/%d", url, index), nil)
	if err != nil {
		return err
	}

	fmt.Printf("Execution complete (now at step %v)\n", resp["current_step"])
	return nil
}

// runAllCmd runs all remaining steps
func runAllCmd(c *cli.Context) error {
	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	fmt.Println("Running all remaining steps...")
	resp, err := postJSON(url+"/step/run-all", nil)
	if err != nil {
		return err
	}

	fmt.Printf("Execution complete (at step %v)\n", resp["current_step"])
	return nil
}

// setVariableCmd sets a custom variable
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

// getStateCmd shows the current state
func getStateCmd(c *cli.Context) error {
	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	resp, err := http.Get(url + "/state")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var state map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return err
	}

	fmt.Println("Debug State:")
	fmt.Printf("  Config: %s\n", state["loaded_config"])
	fmt.Printf("  Task: %s\n", state["selected_task"])
	if state["selected_variant"] != "" {
		fmt.Printf("  Variant: %s\n", state["selected_variant"])
	}
	fmt.Printf("  Step: %v/%v\n", state["current_step"], state["total_steps"])
	fmt.Printf("  Working Dir: %s\n", state["working_dir"])

	if vars, ok := state["variables"].(map[string]interface{}); ok && len(vars) > 0 {
		fmt.Println("  Variables:")
		for k, v := range vars {
			fmt.Printf("    %s=%v\n", k, v)
		}
	}

	return nil
}

// resetCmd resets the debug state
func resetCmd(c *cli.Context) error {
	url, err := getDaemonURL()
	if err != nil {
		return err
	}

	_, err = postJSON(url+"/reset", nil)
	if err != nil {
		return err
	}

	fmt.Println("Debug state reset")
	return nil
}

// Helper function to post JSON
func postJSON(url string, body interface{}) (map[string]interface{}, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(jsonData)
	}

	resp, err := http.Post(url, "application/json", reqBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyData, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("request failed: %s", string(bodyData))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}
