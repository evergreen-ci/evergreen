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

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	daemonDir      = ".evergreen-local"
	daemonPortFile = "daemon.port"
	daemonPIDFile  = "daemon.pid"
)

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

	grip.Infof("Starting daemon on port %d...", port)

	daemon := newLocalDaemonREST(port)
	return daemon.Start()
}

// stopDebugDaemonCmd stops the debug daemon
func stopDebugDaemonCmd(c *cli.Context) error {
	dir, err := getDaemonDir()
	if err != nil {
		return err
	}

	pidFile := filepath.Join(dir, daemonPIDFile)
	portFile := filepath.Join(dir, daemonPortFile)

	defer func() {
		if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
			grip.Warning(errors.Wrapf(err, "removing PID file %s", pidFile))
		}
		if err := os.Remove(portFile); err != nil && !os.IsNotExist(err) {
			grip.Warning(errors.Wrapf(err, "removing port file %s", portFile))
		}
	}()

	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			grip.Info("Daemon is not running")
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
		grip.Info("Daemon process not found (may have already stopped)")
	} else {
		grip.Info("Daemon stopped")
	}
	return nil
}

// daemonStatusCmd checks the debug daemon status
func daemonStatusCmd(c *cli.Context) error {
	if _, err := getDaemonURL(); err != nil {
		grip.Info("Daemon is not running")
		return nil
	}
	grip.Info("Daemon is running")
	return nil
}

// loadConfigCmd loads a configuration file
func loadConfigCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		return errors.New("config file path required")
	}

	configPath, err := filepath.Abs(c.Args().Get(0))
	if err != nil {
		return errors.Wrap(err, "resolving config file path")
	}

	url, err := getDaemonURL()
	if err != nil {
		return errors.Wrap(err, "connecting to daemon")
	}

	reqBody := map[string]string{"config_path": configPath}
	resp, err := postJSON(url+"/config/load", reqBody)
	if err != nil {
		return errors.Wrap(err, "loading configuration")
	}

	grip.Infof("Loaded configuration: %s", configPath)
	grip.Infof("Tasks: %v, Variants: %v", resp["task_count"], resp["variant_count"])

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
