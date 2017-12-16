package operations

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Agent() cli.Command {
	return cli.Command{
		Name:  "agent",
		Usage: "run an evergreen agent",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "host_id",
				Usage: "id of machine agent is running on",
			},
			cli.StringFlag{
				Name:  "host_secret",
				Usage: "secret for the current host",
			},
			cli.StringFlag{
				Name:  "api_server",
				Usage: "URL of the API server",
			},
			cli.StringFlag{
				Name:  "working_directory",
				Usage: "working directory for the agent",
			},
			cli.StringFlag{
				Name:  "log_prefix",
				Value: "evg.agent",
				Usage: "prefix for the agent's log filename",
			},
			cli.IntFlag{
				Name:  "status_port",
				Value: 2285,
				Usage: "port to run the status server",
			},
		},
		Before: func(c *cli.Context) error {
			grip.SetName("evergreen.agent")

			required := []string{
				c.String("api_server"),
				c.String("host_id"),
				c.String("host_secret"),
				c.String("working_directory"),
			}
			if util.StringSliceContains(required, "") {
				return errors.New("agent cannot start with missing required argument")
			}
			return nil
		},
		Action: func(c *cli.Context) error {
			opts := agent.Options{
				HostID:           c.String("host_id"),
				HostSecret:       c.String("host_secret"),
				StatusPort:       c.Int("status_port"),
				LogPrefix:        c.String("log_prefix"),
				WorkingDirectory: c.String("working_directory"),
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
