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
	defer resp.Body.Close()

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
	}
}

// startDebugDaemonCmd starts the debug daemon
func startDebugDaemonCmd(c *cli.Context) error {
	port := c.Int("port")

	if _, err := getDaemonURL(); err == nil {
		return errors.New("daemon is already running")
	}

	fmt.Printf("Starting daemon on port %d...\n", port)

	daemon := newLocalDaemonREST(port)
	return daemon.Start()
}

// stopDebugDaemonCmd stops the debug daemon
func stopDebugDaemonCmd(c *cli.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	pidFile := filepath.Join(homeDir, ".evergreen-local", "daemon.pid")

	defer func() {
		os.Remove(pidFile)
		os.Remove(filepath.Join(homeDir, ".evergreen-local", "daemon.port"))
	}()

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
	return nil
}

// daemonStatusCmd checks the debug daemon status
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
