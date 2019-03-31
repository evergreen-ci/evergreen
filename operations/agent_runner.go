package operations

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// runnerArgs represents the args necessary for the runner to operate.
type runnerArgs struct {
	clientURL    string
	clientPath   string
	apiServerURL string
	agentOpts    agent.Options
}

// agentRunner starts the runner that deploys the agent.
func agentRunner() cli.Command {
	const (
		clientURLFlagName  = "client_url"
		clientPathFlagName = "client_path"
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
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(clientURLFlagName),
			requireStringFlag(clientPathFlagName),
			func(*cli.Context) error {
				grip.SetName("evergreen.agent.runner")
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			args := &runnerArgs{
				clientURL:    c.String(clientURLFlagName),
				clientPath:   c.String(clientPathFlagName),
				apiServerURL: c.Parent().String(apiServerURLFlagName),
				agentOpts: agent.Options{
					HostID:           c.Parent().String(hostIDFlagName),
					HostSecret:       c.Parent().String(hostSecretFlagName),
					LogkeeperURL:     c.Parent().String(logkeeperURLFlagName),
					WorkingDirectory: c.Parent().String(workingDirectoryFlagName),
				},
			}

			ctx, cancel := context.WithCancel(context.Background())

			go handleRunnerSignals(ctx, cancel)

			args.startRunner(ctx)

			return nil
		},
	}
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
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, args.clientURL, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to create client request from URL '%s'", args.clientURL)
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch client from URL '%s'", args.clientURL)
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

	agentCmd := exec.CommandContext(ctx, agentCmdArgs[0], agentCmdArgs[1:]...)
	return agentCmd.Run()
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
