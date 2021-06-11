package operations

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/evergreen-ci/evergreen"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
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
	cloudProvider   string
	distroID        string
	shellPath       string
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
				Name:  logPrefixFlagName,
				Value: monitorLoggerName,
				Usage: "the prefix for the monitor's log name",
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
			m := &monitor{
				comm:            client.NewCommunicator(c.Parent().String(agentAPIServerURLFlagName)),
				credentialsPath: c.String(credentialsPathFlagName),
				clientPath:      c.String(clientPathFlagName),
				distroID:        c.String(distroIDFlagName),
				cloudProvider:   c.Parent().String(agentCloudProviderFlagName),
				shellPath:       c.String(shellPathFlagName),
				jasperPort:      c.Int(jasperPortFlagName),
				port:            c.Int(portFlagName),
				logPrefix:       c.String(logPrefixFlagName),
			}

			var err error
			m.agentArgs, err = getAgentArgs(c, os.Args)
			if err != nil {
				return errors.Wrap(err, "error getting agent args")
			}

			if err = setupLogging(m); err != nil {
				return errors.Wrap(err, "error setting up logging")
			}

			// Reserve the given port to prevent other monitors from starting.
			if _, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", m.port)); err != nil {
				return errors.Wrapf(err, "failed to listen on port %d", m.port)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go handleMonitorSignals(ctx, cancel)

			if err := m.setupJasperConnection(ctx, agentMonitorDefaultRetryOptions()); err != nil {
				return errors.Wrap(err, "failed to connect to RPC service")
			}
			defer func() {
				grip.Error(errors.Wrap(m.jasperClient.CloseConnection(), "failed to close RPC client connection"))
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

// setupLogging sets up the monitor to log based on logPrefix. If the Splunk
// credentials are available, it logs to splunk. If the logging is set to log
// locally, it will log to standard output; otherwise it logs to a file.
func setupLogging(m *monitor) error {
	senderName := m.logPrefix
	senders := []send.Sender{}

	if splunkInfo := send.GetSplunkConnectionInfo(); splunkInfo.Populated() {
		sender, err := send.NewSplunkLogger(
			senderName,
			splunkInfo,
			send.LevelInfo{Default: level.Alert, Threshold: level.Alert},
		)
		if err != nil {
			return errors.Wrap(err, "problem creating Splunk logger")
		}
		senders = append(senders, sender)
	}

	if senderName == evergreen.LocalLoggingOverride || senderName == evergreen.StandardOutputLoggingOverride {
		sender, err := send.NewNativeLogger(senderName, send.LevelInfo{Default: level.Info, Threshold: level.Debug})
		if err != nil {
			return errors.Wrap(err, "problem creating native console logger")
		}
		senders = append(senders, sender)
	} else {
		logDir := filepath.Dir(senderName)
		if err := os.MkdirAll(logDir, 0777); err != nil {
			return errors.Wrapf(err, "problem creating log directory %s", logDir)
		}
		sender, err := send.NewFileLogger(
			senderName,
			fmt.Sprintf("%s-%d-%d.log", senderName, os.Getpid(), getLogID()),
			send.LevelInfo{Default: level.Info, Threshold: level.Debug},
		)
		if err != nil {
			return errors.Wrap(err, "problem creating file logger")
		}
		senders = append(senders, sender)
	}

	return errors.Wrap(grip.SetSender(send.NewConfiguredMultiSender(senders...)), "problem setting logger")
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

	return errors.Wrap(os.Chmod(m.clientPath, 0755), "failed to chmod client")
}

// setupJasperConnection attempts to connect to the Jasper RPC service running
// on this host and sets the RPC manager.
func (m *monitor) setupJasperConnection(ctx context.Context, retry utility.RetryOptions) error {
	addrStr := fmt.Sprintf("127.0.0.1:%d", m.jasperPort)
	serverAddr, err := net.ResolveTCPAddr("tcp", addrStr)
	if err != nil {
		return errors.Wrapf(err, "failed to resolve Jasper server address at '%s'", addrStr)
	}

	if err = utility.Retry(ctx, func() (bool, error) {
		m.jasperClient, err = remote.NewRPCClientWithFile(ctx, serverAddr, m.credentialsPath)
		if err != nil {
			return true, errors.Wrap(err, "could not connect to Jasper RPC service")
		}
		return false, nil
	}, retry); err != nil {
		return errors.Wrapf(err, "failed to connect to Jasper RPC service at '%s'", serverAddr)
	}

	return nil
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
			return true, errors.Wrap(err, "failed to create process")
		}

		jasperIDs := cmd.GetProcIDs()

		// The command should spawn exactly 1 process.
		if len(jasperIDs) != 1 {
			catcher := grip.NewBasicCatcher()

			for _, jasperID := range jasperIDs {
				proc, err = m.jasperClient.Get(ctx, jasperID)
				if err != nil {
					catcher.Add(errors.Wrap(err, "could not find erroneous process"))
					continue
				}
				catcher.Add(proc.Signal(ctx, syscall.SIGKILL))
			}

			return true, errors.Wrapf(catcher.Resolve(), "monitor should spawn exactly 1 process (the agent), but found %d", len(jasperIDs))
		}

		proc, err = m.jasperClient.Get(ctx, jasperIDs[0])
		if err != nil {
			return true, errors.Wrap(err, "could not find agent process")
		}

		return false, nil
	}, retry); err != nil {
		return nil, errors.Wrapf(err, "failed to start agent process '%v'", agentCmdArgs)
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
		return errors.Wrapf(err, "failed to create agent process")
	}

	exitCode, err := waitUntilComplete(ctx, proc, defaultMaxRequestDelay)

	return errors.Wrapf(err, "agent exited with code %d", exitCode)
}

// runMonitor runs the monitor loop. It fetches the agent, starts it, and
// repeats when the agent terminates.
func (m *monitor) run(ctx context.Context) {
	for {
		if err := utility.Retry(ctx, func() (bool, error) {
			if utility.StringSliceContains(evergreen.ProviderSpotEc2Type, m.cloudProvider) {
				if agentutil.SpotHostWillTerminateSoon() {
					return true, errors.New("spot host terminating soon, not starting a new agent")
				}
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
				grip.Error(errors.Wrapf(err, "could not stat client '%s'", m.clientPath))
				return true, err
			}

			grip.Info(message.Fields{
				"message":     "starting agent on host via Jasper",
				"client_path": m.clientPath,
				"distro":      m.distroID,
				"jasper_port": m.jasperPort,
			})
			if err := m.runAgent(ctx, agentMonitorDefaultRetryOptions()); err != nil {
				grip.Error(errors.Wrap(err, "error occurred while running the agent"))
				return true, err
			}

			return false, nil
		}, agentMonitorDefaultRetryOptions()); err != nil {
			if ctx.Err() != nil {
				grip.Warning(errors.Wrap(err, "context cancelled while running monitor"))
				return
			}
			grip.Error(errors.Wrapf(err, "error while managing agent"))
		}
	}
}
