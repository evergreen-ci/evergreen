package operations

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/grip/sometimes"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/rpc"
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
	clientURL       string
	clientPath      string
	logPrefix       string
	jasperPort      int
	port            int

	// Args to be forwarded to the agent
	agentArgs []string

	// Set during runtime
	jasperClient jasper.RemoteClient
}

const (
	defaultMonitorPort        = defaultAgentStatusPort - 1
	defaultMaxRequestDelay    = 30 * time.Second
	defaultMaxRequestAttempts = 10

	monitorLoggerName = "evergreen.agent.monitor"
)

func defaultRetryArgs() util.RetryArgs {
	return util.RetryArgs{
		MaxDelay:    defaultMaxRequestDelay,
		MaxAttempts: defaultMaxRequestAttempts,
	}
}

// agentMonitor starts the monitor that deploys the agent.
func agentMonitor() cli.Command {
	const (
		credentialsPathFlagName = "credentials"
		clientURLFlagName       = "client_url"
		clientPathFlagName      = "client_path"
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
				Name:  clientURLFlagName,
				Usage: "the url to fetch the evergreen client from",
			},
			cli.StringFlag{
				Name:  clientPathFlagName,
				Usage: "the name of the agent's evergreen binary",
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
			requireStringFlag(clientURLFlagName),
			requireStringFlag(clientPathFlagName),
			requireIntValueBetween(jasperPortFlagName, minPort, maxPort),
			requireIntValueBetween(portFlagName, minPort, maxPort),
			func(*cli.Context) error {
				grip.SetName(monitorLoggerName)
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			m := &monitor{
				credentialsPath: c.String(credentialsPathFlagName),
				clientURL:       c.String(clientURLFlagName),
				clientPath:      c.String(clientPathFlagName),
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
			if _, err = net.Listen("tcp", fmt.Sprintf(":%d", m.port)); err != nil {
				return errors.Wrapf(err, "failed to listen on port %d", m.port)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go handleMonitorSignals(ctx, cancel)

			if err := m.setupJasperConnection(ctx, defaultRetryArgs()); err != nil {
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
		sender, err := send.NewFileLogger(
			senderName,
			fmt.Sprintf("%s-%d-%d.log", monitorLoggerName, os.Getpid(), getLogID()),
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
func (m *monitor) fetchClient(ctx context.Context, retry util.RetryArgs) error {
	info := jasper.DownloadInfo{
		URL:         m.clientURL,
		Path:        m.clientPath,
		ArchiveOpts: jasper.ArchiveOptions{ShouldExtract: false},
	}

	if err := util.RetryWithArgs(ctx, func() (bool, error) {
		if err := m.jasperClient.DownloadFile(ctx, info); err != nil {
			return true, errors.Wrap(err, "failed to download file")
		}
		return false, nil
	}, retry); err != nil {
		return err
	}

	return errors.Wrap(os.Chmod(m.clientPath, 0755), "failed to chmod client")
}

// setupJasperConnection attempts to connect to the Jasper RPC service running
// on this host and sets the RPC manager.
func (m *monitor) setupJasperConnection(ctx context.Context, retry util.RetryArgs) error {
	serverAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", m.jasperPort))
	if err != nil {
		return errors.Wrapf(err, "failed to resolve Jasper server address at '%s'", serverAddr)
	}

	if err = util.RetryWithArgs(ctx, func() (bool, error) {
		m.jasperClient, err = rpc.NewClientWithFile(ctx, serverAddr, m.credentialsPath)
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
func (m *monitor) createAgentProcess(ctx context.Context, retry util.RetryArgs) (jasper.Process, error) {
	agentCmdArgs := append([]string{m.clientPath, "agent"}, m.agentArgs...)

	// Copy the monitor's environment to the agent.
	env := make(map[string]string)
	for _, envVar := range os.Environ() {
		keyVal := strings.Split(envVar, "=")
		env[keyVal[0]] = keyVal[1]
	}

	var proc jasper.Process
	var err error

	if err = util.RetryWithArgs(ctx, func() (bool, error) {
		cmd := m.jasperClient.CreateCommand(ctx).
			Add(agentCmdArgs).
			SetOutputSender(level.Info, grip.GetSender()).
			SetErrorSender(level.Error, grip.GetSender()).
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

// waitUntilComplete waits until the given process to complete or the context
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
func (m *monitor) runAgent(ctx context.Context, retry util.RetryArgs) error {
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
		if err := util.RetryWithArgs(ctx, func() (bool, error) {
			// The evergreen agent runs using a separate binary from the monitor
			// to allow the agent to be updated.
			if err := m.fetchClient(ctx, defaultRetryArgs()); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":     "could not fetch client",
					"client_url":  m.clientURL,
					"client_path": m.clientPath,
				}))
				return true, err
			}

			if _, err := os.Stat(m.clientPath); os.IsNotExist(err) {
				grip.Error(errors.Wrapf(err, "could not stat client '%s'", m.clientPath))
				return true, err
			}

			grip.InfoWhen(sometimes.Fifth(), message.Fields{
				"message":     "starting agent on host via Jasper",
				"client_url":  m.clientURL,
				"client_path": m.clientPath,
				"jasper_port": m.jasperPort,
			})
			if err := m.runAgent(ctx, defaultRetryArgs()); err != nil {
				grip.Error(errors.Wrap(err, "error occurred while running the agent"))
				return true, err
			}
			return false, nil
		}, defaultRetryArgs()); err != nil {
			if ctx.Err() != nil {
				grip.Warning(errors.Wrap(err, "context cancelled while running monitor"))
				return
			}
			grip.Error(errors.Wrapf(err, "error while managing agent"))
		}
	}
}
