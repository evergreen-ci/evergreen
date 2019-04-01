package operations

import (
	"context"
	"fmt"
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

// runnerArgs represents the args necessary for the runner to operate.
type runnerArgs struct {
	clientURL    string
	clientPath   string
	apiServerURL string
	jasperPort   int
	agentOpts    agent.Options
}

// agentRunner starts the runner that deploys the agent.
func agentRunner() cli.Command {
	const (
		clientURLFlagName  = "client_url"
		clientPathFlagName = "client_path"
		jasperPortFlagName = "jasper_port"
	)

	return cli.Command{
		Name:  "runner",
		Usage: "start the runner on a host",
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
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(clientURLFlagName),
			requireStringFlag(clientPathFlagName),
			requireStringFlag(jasperPortFlagName),
			func(*cli.Context) error {
				grip.SetName("evergreen.agent.runner")
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			args := &runnerArgs{
				apiServerURL: c.Parent().String(apiServerURLFlagName),
				clientURL:    c.String(clientURLFlagName),
				clientPath:   c.String(clientPathFlagName),
				jasperPort:   c.Int(jasperPortFlagName),
				agentOpts: agent.Options{
					HostID:           c.Parent().String(hostIDFlagName),
					HostSecret:       c.Parent().String(hostSecretFlagName),
					LogkeeperURL:     c.Parent().String(logkeeperURLFlagName),
					WorkingDirectory: c.Parent().String(workingDirectoryFlagName),
				},
			}
			args.agentOpts.LogPrefix = filepath.Join(args.agentOpts.WorkingDirectory, "agent")

			ctx, cancel := context.WithCancel(context.Background())

			go handleRunnerSignals(ctx, cancel)

			if err := setupLogging(); err != nil {
				return errors.Wrap(err, "error setting up logging")
			}

			args.startRunner(ctx)

			return nil
		},
	}
}

// setupLogging sets up the runner to log to Splunk.
func setupLogging() error {
	if splunkInfo := send.GetSplunkConnectionInfo(); splunkInfo.Populated() {
		sender, err := send.NewSplunkLogger("evergreen.agent.runner", splunkInfo, send.LevelInfo{Default: level.Alert, Threshold: level.Alert})
		if err != nil {
			return errors.Wrap(err, "problem creating the splunk logger")
		}
		if err := grip.SetSender(sender); err != nil {
			return errors.Wrap(err, "problem setting sender")
		}
	}
	return errors.New("splunk info is not populated")
}

func handleRunnerSignals(ctx context.Context, serviceCanceler context.CancelFunc) {
	defer recovery.LogStackTraceAndExit("runner signal handler")
	defer serviceCanceler()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-sigChan:
		grip.Info("runner exiting after receiving signal")
	}
	os.Exit(2)
}

// fetchClient downloads the evergreen client.
func (args *runnerArgs) fetchClient(ctx context.Context) error {
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

// runAgent runs the agent with the necessary args.
func (args *runnerArgs) runAgent(ctx context.Context) error {

	maxAttempts := 5
	serverAddress := fmt.Sprintf("localhost:%d", args.jasperPort)
	var conn *grpc.ClientConn
	var err error

	if err := util.Retry(
		ctx,
		func() (bool, error) {
			// kim: QUESTION: need SSL/TLS connection with RPC server if it's running locally?

			conn, err = grpc.DialContext(ctx, serverAddress, grpc.WithInsecure(), grpc.WithBlock())
			if err != nil {
				return true, errors.Wrapf(err, "could not dial Jasper RPC service at '%s'", serverAddress)
			}
			return false, nil
		},
		maxAttempts,
		0,
		time.Minute,
	); err != nil {
		return errors.Wrapf(err, "failed to connect to Jasper RPC service after %d attempts", maxAttempts)
	}

	defer conn.Close()
	manager := rpc.NewRPCManager(conn)

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
		return errors.Wrapf(err, "failed to start agent process after %d attempts", maxAttempts)
	}

	agentFinished := make(chan struct{})
	go func() {
		timer := time.NewTimer(0)
		defer timer.Stop()
		defer close(agentFinished)
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				<-timer.C
				if proc.Complete(ctx) {
					return
				}
				timer.Reset(time.Second)
			}
		}
	}()

	<-agentFinished
	exitCode, err := proc.Wait(ctx)

	return errors.Wrapf(err, "agent exited with code %d", exitCode)
}

// startRunner fetches the agent, starts it, and repeats when the agent
// terminates.
func (args *runnerArgs) startRunner(ctx context.Context) {
	for {
		// The evergreen agent runs using a separate binary from the runner to
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
