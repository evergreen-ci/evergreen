package operations

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Agent() cli.Command {
	const (
		hostIDFlagName           = "host_id"
		hostSecretFlagName       = "host_secret"
		apiServerFlagName        = "api_server"
		workingDirectoryFlagName = "working_directory"
		logPrefixFlagName        = "log_prefix"
		statusPortFlagName       = "status_port"
		cleanupFlagName          = "cleanup"
	)

	return cli.Command{
		Name:  "agent",
		Usage: "run an evergreen agent",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  hostIDFlagName,
				Usage: "id of machine agent is running on",
			},
			cli.StringFlag{
				Name:  hostSecretFlagName,
				Usage: "secret for the current host",
			},
			cli.StringFlag{
				Name:  apiServerFlagName,
				Usage: "URL of the API server",
			},
			cli.StringFlag{
				Name:  workingDirectoryFlagName,
				Usage: "working directory for the agent",
			},
			cli.StringFlag{
				Name:  logPrefixFlagName,
				Value: "evg.agent",
				Usage: "prefix for the agent's log filename",
			},
			cli.IntFlag{
				Name:  statusPortFlagName,
				Value: 2285,
				Usage: "port to run the status server",
			},
			cli.BoolFlag{
				Name:  cleanupFlagName,
				Usage: "clean up working directory and processes (do not set for smoke tests)",
			},
		},
		Before: mergeBeforeFuncs(
			func(c *cli.Context) error {
				grip.SetName("evergreen.agent")
			},
			requireStringFlag(apiServerFlagName),
			requireStringFlag(hostIDFlagName),
			requireStringFlag(hostSecretFlagName),
			requireStringFlag(workingDirectoryFlagName),
		),
		Action: func(c *cli.Context) error {
			opts := agent.Options{
				HostID:           c.String(hostIDFlagName),
				HostSecret:       c.String(hostSecretFlagName),
				StatusPort:       c.Int(statusPortFlagName),
				LogPrefix:        c.String(logPrefixFlagName),
				WorkingDirectory: c.String(workingDirectoryFlagName),
				Cleanup:          c.Bool(cleanupFlagName),
			}

			if err := os.MkdirAll(opts.WorkingDirectory, 0777); err != nil {
				return errors.Wrapf(err, "problem creating working directory '%s'", opts.WorkingDirectory)
			}

			grip.Info(message.Fields{
				"message":  "starting agent",
				"commands": command.RegisteredCommandNames(),
				"dir":      opts.WorkingDirectory,
				"host":     opts.HostID,
			})

			comm := client.NewCommunicator(c.String("api_server"))
			defer comm.Close()

			agt := agent.New(opts, comm)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sender, err := agent.GetSender(ctx, opts.LogPrefix, "init")
			if err != nil {
				return errors.Wrap(err, "problem configuring logger")
			}

			if err = grip.SetSender(sender); err != nil {
				return errors.Wrap(err, "problem setting up logger")
			}

			err = agt.Start(ctx)
			grip.Emergency(err)

			return err
		},
	}

}
