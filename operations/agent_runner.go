package operations

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/PuerkitoBio/rehttp"
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
)

// monitorArgs represents the args necessary for the monitor to operate.
type monitorArgs struct {
	clientURL    string
	clientPath   string
	apiServerURL string
	jasperPort   int
	port         int
	agentOpts    agent.Options
}

// agentMonitor starts the monitor that deploys the agent.
func agentMonitor() cli.Command {
	const (
		clientURLFlagName  = "client_url"
		clientPathFlagName = "client_path"
		jasperPortFlagName = "jasper_port"
		portFlagName       = "port"
	)

	return cli.Command{
		Name:  "monitor",
		Usage: "start the monitor on a host",
		Flags: []cli.Flag{
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
				Usage: "the port that is running the Jasper RPC service",
			},
			cli.IntFlag{
				Name:  portFlagName,
				Usage: "the port that used by the monitor",
			},
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(clientURLFlagName),
			requireStringFlag(clientPathFlagName),
			requireIntFlag(jasperPortFlagName),
			requireIntFlag(portFlagName),
			func(*cli.Context) error {
				grip.SetName("evergreen.agent.monitor")
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			args := &monitorArgs{
				apiServerURL: c.Parent().String(apiServerURLFlagName),
				clientURL:    c.String(clientURLFlagName),
				clientPath:   c.String(clientPathFlagName),
				jasperPort:   c.Int(jasperPortFlagName),
				port:         c.Int(portFlagName),
				agentOpts: agent.Options{
					HostID:           c.Parent().String(hostIDFlagName),
					HostSecret:       c.Parent().String(hostSecretFlagName),
					LogkeeperURL:     c.Parent().String(logkeeperURLFlagName),
					WorkingDirectory: c.Parent().String(workingDirectoryFlagName),
				},
			}
			args.agentOpts.LogPrefix = filepath.Join(args.agentOpts.WorkingDirectory, "agent")

			ctx, cancel := context.WithCancel(context.Background())

			go handleMonitorSignals(ctx, cancel)

			if err := setupLogging(); err != nil {
				return errors.Wrap(err, "error setting up logging")
			}

			// kim: TODO: start listening like a server on the given port.

			_, err := net.Listen("tcp", fmt.Sprintf(":%d", args.port))
			if err != nil {
				return nil
			}

			args.startMonitor(ctx)

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

// handleMonitorSignals gracefully shuts down the monitor process when it receives
// a terminate signal.
func handleMonitorSignals(ctx context.Context, serviceCanceler context.CancelFunc) {
	defer recovery.LogStackTraceAndExit("monitor signal handler")
	defer serviceCanceler()
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
func (args *monitorArgs) fetchClient(ctx context.Context) error {
	req, err := http.NewRequest(http.MethodGet, args.clientURL, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to create client request from URL '%s'", args.clientURL)
	}

	client := &http.Client{}
	maxAttempts := 10
	client.Transport = rehttp.NewTransport(
		client.Transport,
		func(attempt rehttp.Attempt) bool {
			return attempt.Index < maxAttempts && (attempt.Error != nil || attempt.Response.StatusCode != http.StatusOK)
		},
		util.RehttpDelay(0, maxAttempts),
	)
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch client from URL '%s' after %d attempts", args.clientURL, maxAttempts)
	}

	if err := os.MkdirAll(filepath.Dir(args.clientPath), 0777); err != nil {
		return errors.Wrapf(err, "error making directories for client binary file '%s'", args.clientPath)
	}
	if err := util.WriteToFile(resp.Body, args.clientPath); err != nil {
		return errors.Wrapf(err, "error writing client binary to file '%s'", args.clientPath)
	}

	return errors.Wrap(os.Chmod(args.clientPath, 0755), "failed to chmod client")
}

// connectToJasper attempts to connect to the Jasper RPC service running on this
// host.
func (args *monitorArgs) connectToJasper(ctx context.Context, maxAttempts int, maxDelay time.Duration) (*grpc.ClientConn, error) {
	serverAddress := fmt.Sprintf("localhost:%d", args.jasperPort)
	var conn *grpc.ClientConn
	var err error

	if err := util.Retry(
		ctx,
		func() (bool, error) {
			conn, err = grpc.DialContext(ctx, serverAddress, grpc.WithInsecure(), grpc.WithBlock())
			if err != nil {
				return true, errors.Wrapf(err, "could not dial Jasper RPC service at '%s'", serverAddress)
			}
			return false, nil
		},
		maxAttempts,
		0,
		maxDelay,
	); err != nil {
		return nil, errors.Wrapf(err, "failed to connect to Jasper RPC service at '%s'", serverAddress)
	}

	return conn, nil
}

// createAgentProcess attempts to request that Jasper start an agent.
func (args *monitorArgs) createAgentProcess(ctx context.Context, manager jasper.Manager, maxAttempts int, maxDelay time.Duration) (jasper.Process, error) {
	agentCmdArgs := []string{
		args.clientPath,
		"agent",
		fmt.Sprintf("--api_server='%s'", args.apiServerURL),
		fmt.Sprintf("--host_id='%s'", args.agentOpts.HostID),
		fmt.Sprintf("--host_secret='%s'", args.agentOpts.HostSecret),
		fmt.Sprintf("--log_prefix='%s'", filepath.Join(args.agentOpts.WorkingDirectory, "agent")),
		fmt.Sprintf("--working_directory='%s'", args.agentOpts.WorkingDirectory),
		fmt.Sprintf("--logkeeper_url='%s'", args.agentOpts.LogkeeperURL),
		"--cleanup",
	}
	opts := &jasper.CreateOptions{
		Args: agentCmdArgs,
	}

	var proc jasper.Process
	var err error

	if err := util.Retry(
		ctx,
		func() (bool, error) {
			proc, err = manager.CreateProcess(ctx, opts)
			if err != nil {
				return true, errors.Wrap(err, "failed to create process")
			}
			return false, nil
		},
		maxAttempts,
		0,
		time.Minute,
	); err != nil {
		return nil, errors.Wrapf(err, "failed to start agent process with args '%v'", agentCmdArgs)
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

// runAgent runs the agent with the necessary args.
func (args *monitorArgs) runAgent(ctx context.Context) error {
	maxAttempts := 5
	maxDelay := time.Minute

	conn, err := args.connectToJasper(ctx, maxAttempts, maxDelay)
	if err != nil {
		return errors.Wrapf(err, "failed to connect to RPC service after %d attempts", maxAttempts)
	}
	defer func() {
		grip.Error(errors.Wrap(conn.Close(), "error closing gRPC connection"))
	}()
	manager := rpc.NewRPCManager(conn)

	proc, err := args.createAgentProcess(ctx, manager, maxAttempts, maxDelay)
	if err != nil {
		return errors.Wrapf(err, "faileed to create agent process after %d attempts", maxAttempts)
	}

	exitCode, err := waitUntilComplete(ctx, proc, maxDelay)

	return errors.Wrapf(err, "agent exited with code %d", exitCode)
}

// startMonitor starts the monitor loop. It fetches the agent, starts it, and
// repeats when the agent terminates.
func (args *monitorArgs) startMonitor(ctx context.Context) {
	for {
		// The evergreen agent runs using a separate binary from the monitor to
		// allow the agent to be updated.
		if err := args.fetchClient(ctx); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":     "could not fetch client",
				"client_url":  args.clientURL,
				"client_path": args.clientPath,
			}))
			continue
		}

		if _, err := os.Stat(args.clientPath); os.IsNotExist(err) {
			grip.Error(errors.Wrapf(err, "could not stat client '%s'", args.clientPath))
			continue
		}

		if err := args.runAgent(ctx); err != nil {
			grip.Error(errors.Wrap(err, "error occurred while running the agent"))
		}
	}
}
