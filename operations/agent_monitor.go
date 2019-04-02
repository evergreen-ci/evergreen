package operations

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// monitor represents the agent monitor, including arguments necessary for it
// to operate.
type monitor struct {
	certificatePath    string
	clientURL          string
	clientPath         string
	apiServerURL       string
	jasperPort         int
	port               int
	agentOpts          agent.Options
	maxRequestAttempts int
	jasperClient       rpc.Client
}

const (
	defaultMaxJasperRequestAttempts = 5
	defaultMonitorPort              = defaultAgentStatusPort - 1
	defaultJasperPort               = defaultMonitorPort - 1
)

// agentMonitor starts the monitor that deploys the agent.
func agentMonitor() cli.Command {
	const (
		certificatePathFlagName = "certificate"
		clientURLFlagName       = "client_url"
		clientPathFlagName      = "client_path"
		jasperPortFlagName      = "jasper_port"
		portFlagName            = "port"
	)

	return cli.Command{
		Name:  "monitor",
		Usage: "start the monitor on a host",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  certificatePathFlagName,
				Usage: "the path to the certificate used to authenticate the monitor to Jasper",
			},
			cli.StringFlag{
				Name:  clientURLFlagName,
				Usage: "the url to fetch the evergreen client from",
			},
			cli.StringFlag{
				Name:  clientPathFlagName,
				Usage: "the name of the agent's evergreen binary",
			},
			cli.IntFlag{
				Name:  jasperPortFlagName,
				Value: defaultAgentStatusPort - 2,
				Usage: "the port that is running the Jasper RPC service",
			},
			cli.IntFlag{
				Name:  portFlagName,
				Value: defaultAgentStatusPort - 1,
				Usage: "the port that used by the monitor",
			},
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(clientURLFlagName),
			requireStringFlag(clientPathFlagName),
			requireIntValueBetween(jasperPortFlagName, 0, 1<<16),
			requireIntValueBetween(portFlagName, 0, 1<<16),
			func(*cli.Context) error {
				grip.SetName("evergreen.agent.monitor")
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			m := &monitor{
				apiServerURL:    c.Parent().String(apiServerURLFlagName),
				certificatePath: c.String(certificatePathFlagName),
				clientURL:       c.String(clientURLFlagName),
				clientPath:      c.String(clientPathFlagName),
				jasperPort:      c.Int(jasperPortFlagName),
				port:            c.Int(portFlagName),
				agentOpts: agent.Options{
					HostID:           c.Parent().String(hostIDFlagName),
					HostSecret:       c.Parent().String(hostSecretFlagName),
					LogkeeperURL:     c.Parent().String(logkeeperURLFlagName),
					WorkingDirectory: c.Parent().String(workingDirectoryFlagName),
				},
				maxRequestAttempts: defaultMaxJasperRequestAttempts,
			}
			m.agentOpts.LogPrefix = filepath.Join(m.agentOpts.WorkingDirectory, "agent")

			if err := setupLogging(); err != nil {
				return errors.Wrap(err, "error setting up logging")
			}

			_, err := net.Listen("tcp", fmt.Sprintf(":%d", m.port))
			if err != nil {
				if err == syscall.EADDRINUSE {
					return nil
				}
				return errors.Wrapf(err, "failed to listen on port %d", m.port)
			}

			conn, err := m.setupJasperConnection(context.Background(), time.Minute)
			if err != nil {
				return errors.Wrapf(err, "failed to connect to RPC service after %d attempts", m.maxRequestAttempts)
			}

			ctx, cancel := context.WithCancel(context.Background())
			go handleMonitorSignals(ctx, cancel, conn)

			m.startMonitor(ctx)

			return nil
		},
	}
}

// setupLogging sets up the monitor to log to Splunk.
func setupLogging() error {
	if splunkInfo := send.GetSplunkConnectionInfo(); splunkInfo.Populated() {
		sender, err := send.NewSplunkLogger("evergreen.agent.monitor", splunkInfo, send.LevelInfo{Default: level.Alert, Threshold: level.Alert})
		if err != nil {
			return errors.Wrap(err, "problem creating the splunk logger")
		}
		if err := grip.SetSender(sender); err != nil {
			return errors.Wrap(err, "problem setting sender")
		}
	}
	return errors.New("splunk info is not populated")
}

// handleMonitorSignals gracefully shuts down the monitor process when it
// receives a terminate signal.
func handleMonitorSignals(ctx context.Context, serviceCanceler context.CancelFunc, conn *grpc.ClientConn) {
	defer recovery.LogStackTraceAndExit("monitor signal handler")
	defer serviceCanceler()
	defer func() {
		if conn != nil {
			grip.Error(errors.Wrap(conn.Close(), "failed to close gRPC connection"))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-sigChan:
		grip.Info("monitor exiting after receiving signal")
	}

	os.Exit(2)
}

// fetchClient downloads the evergreen client.
func (m *monitor) fetchClient(ctx context.Context, maxDelay time.Duration) error {
	info := jasper.DownloadInfo{
		URL:         m.clientURL,
		Path:        m.clientPath,
		ArchiveOpts: jasper.ArchiveOptions{ShouldExtract: false},
	}

	if err := util.Retry(
		ctx,
		func() (bool, error) {
			if err := m.jasperClient.DownloadFile(ctx, info); err != nil {
				return true, errors.Wrap(err, "failed to download file")
			}
			return false, nil
		},
		m.maxRequestAttempts,
		0,
		maxDelay,
	); err != nil {
		return err
	}

	return errors.Wrap(os.Chmod(m.clientPath, 0755), "failed to chmod client")
}

// setupJasperConnection attempts to connect to the Jasper RPC service running
// on this host and sets the RPC manager.
func (m *monitor) setupJasperConnection(ctx context.Context, maxDelay time.Duration) (*grpc.ClientConn, error) {
	serverAddress := fmt.Sprintf("localhost:%d", m.jasperPort)
	var conn *grpc.ClientConn
	var err error

	if err := util.Retry(
		ctx,
		func() (bool, error) {
			if m.certificatePath != "" {
				var creds credentials.TransportCredentials
				creds, err = credentials.NewClientTLSFromFile(m.certificatePath, "")
				if err != nil {
					return false, errors.Wrapf(err, "could not open client ceritificate file")
				}
				conn, err = grpc.DialContext(ctx, serverAddress, grpc.WithTransportCredentials(creds), grpc.WithBlock())
			} else {
				conn, err = grpc.DialContext(ctx, serverAddress, grpc.WithInsecure(), grpc.WithBlock())
			}

			if err != nil {
				return true, errors.Wrap(err, "could not connect to Jasper RPC service")
			}

			return false, nil
		},
		m.maxRequestAttempts,
		0,
		maxDelay,
	); err != nil {
		return nil, errors.Wrapf(err, "failed to connect to Jasper RPC service at '%s'", serverAddress)
	}
	m.jasperClient = rpc.NewClient(conn)

	return conn, nil
}

// createAgentProcess attempts to start an agent subprocess.
func (m *monitor) createAgentProcess(ctx context.Context, maxDelay time.Duration) (jasper.Process, error) {
	agentCmdArgs := []string{
		m.clientPath,
		"agent",
		fmt.Sprintf("--api_server='%s'", m.apiServerURL),
		fmt.Sprintf("--host_id='%s'", m.agentOpts.HostID),
		fmt.Sprintf("--host_secret='%s'", m.agentOpts.HostSecret),
		fmt.Sprintf("--log_prefix='%s'", filepath.Join(m.agentOpts.WorkingDirectory, "agent")),
		fmt.Sprintf("--working_directory='%s'", m.agentOpts.WorkingDirectory),
		fmt.Sprintf("--logkeeper_url='%s'", m.agentOpts.LogkeeperURL),
		"--cleanup",
	}
	var proc jasper.Process
	var err error
	opts := &jasper.CreateOptions{Args: agentCmdArgs}

	if err := util.Retry(
		ctx,
		func() (bool, error) {
			proc, err = m.jasperClient.CreateProcess(ctx, opts)
			if err != nil {
				return true, errors.Wrap(err, "failed to create process")
			}
			return false, nil
		},
		m.maxRequestAttempts,
		0,
		time.Minute,
	); err != nil {
		return nil, errors.Wrapf(err, "failed to start agent process with monitor '%v'", agentCmdArgs)
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

// runAgent runs the agent with the necessary m.
func (m *monitor) runAgent(ctx context.Context) error {
	maxDelay := time.Minute

	proc, err := m.createAgentProcess(ctx, maxDelay)
	if err != nil {
		return errors.Wrapf(err, "failed to create agent process after %d attempts", m.maxRequestAttempts)
	}

	exitCode, err := waitUntilComplete(ctx, proc, maxDelay)

	return errors.Wrapf(err, "agent exited with code %d", exitCode)
}

// startMonitor starts the monitor loop. It fetches the agent, starts it, and
// repeats when the agent terminates.
func (m *monitor) startMonitor(ctx context.Context) {
	for {
		// The evergreen agent runs using a separate binary from the monitor to
		// allow the agent to be updated.
		if err := m.fetchClient(ctx, time.Minute); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":     "could not fetch client",
				"client_url":  m.clientURL,
				"client_path": m.clientPath,
			}))
			continue
		}

		if _, err := os.Stat(m.clientPath); os.IsNotExist(err) {
			grip.Error(errors.Wrapf(err, "could not stat client '%s'", m.clientPath))
			continue
		}

		if err := m.runAgent(ctx); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":      "error occurred while running the agent",
				"max_attempts": m.maxRequestAttempts,
			}))
		}
	}
}
