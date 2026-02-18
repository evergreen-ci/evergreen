package operations

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var logIDSource chan int

func init() {
	logIDSource = make(chan int, 100)

	go func() {
		id := 0
		for {
			logIDSource <- id
			id++
		}
	}()
}

func getLogID() int {
	return <-logIDSource
}

// monitor represents the agent monitor, including arguments necessary for it to
// operate.
type monitor struct {
	// Monitor args
	credentialsPath string
	clientPath      string
	hostID          string
	cloudProvider   string
	distroID        string
	shellPath       string
	logOutput       globals.LogOutputType
	logPrefix       string
	jasperPort      int
	port            int

	// Args to be forwarded to the agent
	agentArgs []string

	comm         client.Communicator
	jasperClient remote.Manager
}

const (
	defaultMonitorPort        = defaultAgentStatusPort - 1
	defaultMaxRequestDelay    = 30 * time.Second
	defaultMaxRequestAttempts = 10

	monitorLoggerName = "evergreen.agent.monitor"
)

func agentMonitorDefaultRetryOptions() utility.RetryOptions {
	return utility.RetryOptions{
		MaxDelay:    defaultMaxRequestDelay,
		MaxAttempts: defaultMaxRequestAttempts,
	}
}

// agentMonitor starts the monitor that deploys the agent.
func agentMonitor() cli.Command {
	const (
		credentialsPathFlagName = "credentials"
		clientPathFlagName      = "client_path"
		distroIDFlagName        = "distro"
		shellPathFlagName       = "shell_path"
		logOutputFlagName       = "log_output"
		logPrefixFlagName       = "log_prefix"
		jasperPortFlagName      = "jasper_port"
		portFlagName            = "port"
	)

	const (
		minPort = 1 << 10
		maxPort = math.MaxUint16 - 1
	)

	return cli.Command{
		Name:  "monitor",
		Usage: "start the monitor on a host",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  credentialsPathFlagName,
				Usage: "the path to the credentials used to authenticate the monitor to Jasper",
			},
			cli.StringFlag{
				Name:  clientPathFlagName,
				Usage: "the name of the agent's evergreen binary",
			},
			cli.StringFlag{
				Name:  distroIDFlagName,
				Usage: "the ID of the distro the monitor is running on.",
			},
			cli.StringFlag{
				Name:  shellPathFlagName,
				Value: "bash",
				Usage: "the path to the shell for starting the agent",
			},
			cli.StringFlag{
				Name:  logOutputFlagName,
				Value: string(globals.LogOutputFile),
				Usage: "location for the agent monitor's log output (file, stdout)",
			},
			cli.StringFlag{
				Name:  logPrefixFlagName,
				Value: monitorLoggerName,
				Usage: "the prefix sender name for the agent monitor's log output. If logging to a file, the prefix of the file name for the agent monitor's log output",
			},
			cli.IntFlag{
				Name:  jasperPortFlagName,
				Value: evergreen.DefaultJasperPort,
				Usage: "the port that is running the Jasper RPC service",
			},
			cli.IntFlag{
				Name:  portFlagName,
				Value: defaultMonitorPort,
				Usage: "the port that used by the monitor",
			},
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(clientPathFlagName),
			requireStringFlag(distroIDFlagName),
			requireStringFlag(shellPathFlagName),
			requireIntValueBetween(jasperPortFlagName, minPort, maxPort),
			requireIntValueBetween(portFlagName, minPort, maxPort),
			func(*cli.Context) error {
				grip.SetName(monitorLoggerName)
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			apiServerURL := c.Parent().String(agentAPIServerURLFlagName)
			hostID := c.Parent().String(agentHostIDFlagName)
			hostSecret := c.Parent().String(agentHostSecretFlagName)
			if hostID == "" || hostSecret == "" {
				return errors.New("host ID and host secret must be set")
			}
			comm, err := client.NewCommunicator(apiServerURL)
			if err != nil {
				return errors.Wrap(err, "initializing communicator")
			}
			comm.SetHostID(hostID)
			comm.SetHostSecret(hostSecret)

			m := &monitor{
				comm:            comm,
				credentialsPath: c.String(credentialsPathFlagName),
				clientPath:      c.String(clientPathFlagName),
				hostID:          hostID,
				distroID:        c.String(distroIDFlagName),
				cloudProvider:   c.Parent().String(agentCloudProviderFlagName),
				shellPath:       c.String(shellPathFlagName),
				jasperPort:      c.Int(jasperPortFlagName),
				port:            c.Int(portFlagName),
				logOutput:       globals.LogOutputType(c.String(logOutputFlagName)),
				logPrefix:       c.String(logPrefixFlagName),
			}

			m.agentArgs, err = getAgentArgs(c, os.Args)
			if err != nil {
				return errors.Wrap(err, "getting agent args")
			}

			if err = setupLogging(m); err != nil {
				return errors.Wrap(err, "setting up logging")
			}

			// Reserve the given port to prevent other monitors from starting.
			if _, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", m.port)); err != nil {
				return errors.Wrapf(err, "listening on port %d", m.port)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go handleMonitorSignals(ctx, cancel)

			if err := m.setupJasperConnection(ctx, agentMonitorDefaultRetryOptions()); err != nil {
				return errors.Wrap(err, "connecting to RPC service")
			}
			defer func() {
				grip.Error(errors.Wrap(m.jasperClient.CloseConnection(), "closing RPC client connection"))
			}()

			m.run(ctx)

			return nil
		},
	}
}

// getAgentArgs find the slice of flags that should be forwarded to the agent.
func getAgentArgs(c *cli.Context, args []string) ([]string, error) {
	beginIndex, endIndex := -1, -1

	for index, arg := range args {
		if arg == "agent" {
			beginIndex = index
		}
		if arg == c.Command.Name {
			endIndex = index
		}
	}

	if beginIndex == -1 || endIndex == -1 {
		return nil, errors.Errorf("agent args are invalid: %s", strings.Join(args, " "))
	}

	return args[beginIndex+1 : endIndex], nil
}

// setupLogging sets up the monitor to log based on logPrefix. If the logging is
// set to log locally, it will log to standard output; otherwise it logs to a
// file.
func setupLogging(m *monitor) error {
	senderOutput := m.logOutput
	senderName := m.logPrefix
	var sender send.Sender

	switch senderOutput {
	case globals.LogOutputStdout:
		nativeSender, err := send.NewNativeLogger(senderName, send.LevelInfo{Default: level.Info, Threshold: level.Debug})
		if err != nil {
			return errors.Wrap(err, "creating native console logger")
		}
		sender = nativeSender
	default:
		logDir := filepath.Dir(senderName)
		if err := os.MkdirAll(logDir, 0777); err != nil {
			return errors.Wrapf(err, "creating log directory '%s'", logDir)
		}
		fileSender, err := send.NewFileLogger(
			senderName,
			fmt.Sprintf("%s-%d-%d.log", senderName, os.Getpid(), getLogID()),
			send.LevelInfo{Default: level.Info, Threshold: level.Debug},
		)
		if err != nil {
			return errors.Wrap(err, "creating file logger")
		}
		sender = fileSender
	}

	return errors.Wrap(grip.SetSender(sender), "setting logger")
}

// handleMonitorSignals gracefully shuts down the monitor process when it
// receives a terminate signal.
func handleMonitorSignals(ctx context.Context, serviceCancel context.CancelFunc) {
	defer recovery.LogStackTraceAndContinue("monitor signal handler")
	defer serviceCancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, os.Interrupt)

	select {
	case <-ctx.Done():
	case <-sigChan:
		grip.Info("monitor exiting after receiving signal")
	}
}

// removeMacOSClient deletes the Evergreen client file on MacOS hosts. This is
// necessary to fix a MacOS-specific issue where if the host has System
// Integrity Protection (SIP) enabled and it runs an Evergreen client that has a
// problem (e.g. the binary was not signed by Apple), running that client
// results in SIGKILL. The SIGKILL issue will persist even if a valid agent is
// downloaded to replace it. Removing the binary before downloading it is the
// only known workaround to ensure that MacOS can run the client.
func (m *monitor) removeMacOSClient() error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	if err := os.RemoveAll(m.clientPath); err != nil {
		return errors.Wrapf(err, "removing client '%s'", m.clientPath)
	}

	return nil
}

// fetchClient downloads the evergreen client.
func (m *monitor) fetchClient(ctx context.Context, urls []string, retry utility.RetryOptions) error {
	var downloaded bool
	catcher := grip.NewBasicCatcher()
	for _, url := range urls {
		info := options.Download{
			URL:         url,
			Path:        m.clientPath,
			ArchiveOpts: options.Archive{ShouldExtract: false},
		}

		var attemptNum int
		if err := utility.Retry(ctx, func() (bool, error) {
			if err := m.jasperClient.DownloadFile(ctx, info); err != nil {
				attemptNum++
				return true, errors.Wrapf(err, "attempt %d", attemptNum)
			}
			return false, nil
		}, retry); err != nil {
			catcher.Wrapf(err, "URL '%s'", url)
			continue
		}

		downloaded = true
		break
	}
	if !downloaded {
		return errors.Wrap(catcher.Resolve(), "downloading client")
	}

	return errors.Wrap(os.Chmod(m.clientPath, 0755), "changing permissions of downloaded client")
}

// setupJasperConnection attempts to connect to the Jasper RPC service running
// on this host and sets the RPC manager.
func (m *monitor) setupJasperConnection(ctx context.Context, retry utility.RetryOptions) error {
	addrStr := fmt.Sprintf("127.0.0.1:%d", m.jasperPort)
	serverAddr, err := net.ResolveTCPAddr("tcp", addrStr)
	if err != nil {
		return errors.Wrapf(err, "resolving Jasper server address '%s'", addrStr)
	}

	if err = utility.Retry(ctx, func() (bool, error) {
		m.jasperClient, err = remote.NewRPCClientWithFile(ctx, serverAddr, m.credentialsPath)
		if err != nil {
			return true, errors.Wrap(err, "connecting to Jasper RPC service")
		}
		return false, nil
	}, retry); err != nil {
		return errors.Wrapf(err, "connecting to Jasper RPC service '%s'", serverAddr)
	}

	return nil
}

// allowAgentNice ensures that the Evergreen client used by the agent is able to
// set its nice value, which is a way to control process-level CPU
// prioritization. Only applies to Linux.
func (m *monitor) allowAgentNice(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	return m.jasperClient.CreateCommand(ctx).Add([]string{
		"sudo",
		"setcap",
		// Set the cap_sys_nice setting. cap_sys_nice gives the agent's client
		// the ability to set negative nice values (i.e. higher CPU priority).
		// "p" means permitted (i.e. allow the program to set its nice).
		// "e" means effective (i.e. the permission is active immediately).
		"cap_sys_nice=+ep",
		m.clientPath}).Run(ctx)
}

// createAgentProcess attempts to start an agent subprocess.
func (m *monitor) createAgentProcess(ctx context.Context, retry utility.RetryOptions) (jasper.Process, error) {
	agentCmdArgs := append([]string{m.clientPath, "agent"}, m.agentArgs...)

	// Copy the monitor's environment to the agent.
	env := make(map[string]string)
	for _, envVar := range os.Environ() {
		keyVal := strings.Split(envVar, "=")
		env[keyVal[0]] = keyVal[1]
	}

	var proc jasper.Process
	var err error

	if err = utility.Retry(ctx, func() (bool, error) {
		cmd := m.jasperClient.CreateCommand(ctx).
			Add([]string{m.shellPath, "-l", "-c", strings.Join(agentCmdArgs, " ")}).
			Environment(env).
			Background(true)
		if err = cmd.Run(ctx); err != nil {
			return true, errors.Wrap(err, "starting agent process")
		}

		jasperIDs := cmd.GetProcIDs()

		// The command should spawn exactly 1 process.
		if len(jasperIDs) != 1 {
			catcher := grip.NewBasicCatcher()

			for _, jasperID := range jasperIDs {
				proc, err = m.jasperClient.Get(ctx, jasperID)
				if err != nil {
					catcher.Wrapf(err, "finding duplicate agent process '%s'", jasperID)
					continue
				}
				catcher.Add(proc.Signal(ctx, syscall.SIGKILL))
			}

			return true, errors.Wrapf(catcher.Resolve(), "monitor should spawn exactly 1 process (the agent), but found %d", len(jasperIDs))
		}

		proc, err = m.jasperClient.Get(ctx, jasperIDs[0])
		if err != nil {
			return true, errors.Wrap(err, "finding agent process")
		}

		return false, nil
	}, retry); err != nil {
		return nil, errors.Wrapf(err, "starting agent process with args: %s", agentCmdArgs)
	}
	return proc, nil
}

// waitUntilComplete waits until the given process has completed or the context
// times out. maxDelay determines the time between checks on the process.
func waitUntilComplete(ctx context.Context, proc jasper.Process, maxDelay time.Duration) (exitCode int, err error) {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return proc.Wait(ctx)
		case <-timer.C:
			if proc.Complete(ctx) {
				return proc.Wait(ctx)
			}
			timer.Reset(maxDelay)
		}
	}
}

// runAgent starts the agent with the necessary args and waits for it to
// terminate.
func (m *monitor) runAgent(ctx context.Context, retry utility.RetryOptions) error {
	proc, err := m.createAgentProcess(ctx, retry)
	if err != nil {
		return errors.Wrapf(err, "creating agent process")
	}

	exitCode, err := waitUntilComplete(ctx, proc, defaultMaxRequestDelay)

	return errors.Wrapf(err, "agent exited with code %d", exitCode)
}

var errAgentMonitorShouldExit = errors.New("host's agent monitor should exit")

// runMonitor runs the monitor loop. It fetches the agent, starts it, and
// repeats when the agent terminates.
func (m *monitor) run(ctx context.Context) {
	for {
		if err := utility.Retry(ctx, func() (bool, error) {
			// Check the host health before actually starting a new agent. If
			// the host is in a state where it cannot run tasks (e.g.
			// indefinitely for quarantined hosts, or permanently for terminated
			// hosts), the agent cannot run tasks anyways.
			healthCheck, err := m.getHostHealth(ctx)
			if err != nil {
				return true, errors.Wrap(err, "checking host health")
			}

			if healthCheck.shouldExit {
				grip.Info(message.Fields{
					"message":     "host status indicates it should exit, shutting down",
					"host_id":     m.hostID,
					"host_status": healthCheck.status,
					"distro_id":   m.distroID,
				})
				return false, errAgentMonitorShouldExit
			}

			if err := m.removeMacOSClient(); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":     "could not remove MacOS client",
					"distro":      m.distroID,
					"client_path": m.clientPath,
				}))
				return true, err
			}

			clientURLs, err := m.comm.GetClientURLs(ctx, m.distroID)
			if err != nil {
				return true, errors.Wrap(err, "retrieving client URLs")
			}

			// The evergreen agent runs using a separate binary from the monitor
			// to allow the agent to be updated.
			if err := m.fetchClient(ctx, clientURLs, agentMonitorDefaultRetryOptions()); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":     "could not fetch client",
					"distro":      m.distroID,
					"client_path": m.clientPath,
				}))
				return true, err
			}

			if _, err := os.Stat(m.clientPath); os.IsNotExist(err) {
				grip.Error(errors.Wrapf(err, "getting file stat for client '%s'", m.clientPath))
				return true, err
			}

			// This is only a warning because it's a best-effort attempt to give
			// the agent the ability to set its nice. Controlling nice gives the
			// agent a mechanism to tune CPU prioritization, but it's not a
			// guarantee and is not required for the agent to run.
			grip.Warning(errors.Wrap(m.allowAgentNice(ctx), "allowing agent to set nice"))

			grip.Info(message.Fields{
				"message":     "starting agent on host via Jasper",
				"client_path": m.clientPath,
				"distro":      m.distroID,
				"jasper_port": m.jasperPort,
			})

			if err := m.runAgent(ctx, agentMonitorDefaultRetryOptions()); err != nil {
				grip.Error(errors.Wrap(err, "running the agent"))
				return true, err
			}

			return false, nil
		}, agentMonitorDefaultRetryOptions()); err != nil {
			if ctx.Err() != nil {
				grip.Warning(errors.Wrap(err, "context cancelled while running monitor"))
				return
			}
			if errors.Is(err, errAgentMonitorShouldExit) {
				return
			}
			grip.Error(errors.Wrapf(err, "managing agent"))
		}
	}
}

type hostHealthCheck struct {
	shouldExit bool
	status     string
}

func (m *monitor) getHostHealth(ctx context.Context) (hostHealthCheck, error) {
	apiHost, err := m.comm.PostHostIsUp(ctx, host.HostMetadataOptions{})
	if err != nil {
		return hostHealthCheck{
			shouldExit: false,
		}, errors.Wrap(err, "getting host to check its status")
	}

	status := utility.FromStringPtr(apiHost.Status)

	if utility.FromStringPtr(apiHost.NeedsReprovision) != "" {
		// If the host needs to reprovision, the agent monitor has to stop to
		// allow the reprovisioning to happen.
		return hostHealthCheck{
			shouldExit: true,
			status:     status,
		}, nil
	}

	switch status {
	case evergreen.HostBuilding, evergreen.HostStarting, evergreen.HostRunning:
		return hostHealthCheck{
			shouldExit: false,
			status:     status,
		}, nil
	default:
		return hostHealthCheck{
			shouldExit: true,
			status:     status,
		}, nil
	}
}
