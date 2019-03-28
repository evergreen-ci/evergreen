package operations

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/evergreen-ci/evergreen/agent"
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

// hostRunner starts the runner that deploys the agent.
func hostRunner() cli.Command {
	const (
		clientURLFlagName  = "client_url"
		clientPathFlagName = "client_path"
	)

	return cli.Command{
		Name:  "runner",
		Usage: "start the runner on a host",
		Flags: append(addCommonAgentAndRunnerFlags(),
			cli.StringFlag{
				Name:  clientURLFlagName,
				Usage: "the url to fetch the evergreen client from",
			},
			cli.StringFlag{
				Name:  clientPathFlagName,
				Usage: "the name of the agent's evergreen binary",
			},
		),
		Before: mergeBeforeFuncs(
			append(requireAgentFlags(),
				requireStringFlag(clientURLFlagName),
				requireStringFlag(clientPathFlagName),
				func(_ *cli.Context) error {
					grip.SetName("evergreen.runner")
					return nil
				},
			)...,
		),
		Action: func(c *cli.Context) error {
			args := &runnerArgs{
				clientURL:    c.String(clientURLFlagName),
				clientPath:   c.String(clientPathFlagName),
				apiServerURL: c.String(apiServerURLFlagName),
				agentOpts: agent.Options{
					HostID:           c.String(hostIDFlagName),
					HostSecret:       c.String(hostSecretFlagName),
					LogkeeperURL:     c.String(logkeeperURLFlagName),
					WorkingDirectory: c.String(workingDirectoryFlagName),
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

// curlClient curls the evergreen client.
func (args *runnerArgs) fetchClient(ctx context.Context) error {
	curlClientArgs := []string{
		"curl",
		"--create-dirs",
		"-L",
		args.clientURL,
		"-o",
		args.clientPath,
	}

	curlCmd := exec.CommandContext(ctx, curlClientArgs[0], curlClientArgs[1:]...)
	if err := curlCmd.Run(); err != nil {
		return errors.Wrap(err, "failed to curl client")
	}
	grip.Info(curlClientArgs)

	chmodArgs := []string{
		"chmod",
		"+x",
		args.clientPath,
	}
	chmodCmd := exec.CommandContext(ctx, chmodArgs[0], chmodArgs[1:]...)

	return errors.Wrap(chmodCmd.Run(), "failed to chmod client")
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
