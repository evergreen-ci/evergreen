package operations

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const defaultAgentStatusPort = 2285

func Agent() cli.Command {
	const (
		hostIDFlagName           = "host_id"
		hostSecretFlagName       = "host_secret"
		apiServerURLFlagName     = "api_server"
		workingDirectoryFlagName = "working_directory"
		logkeeperURLFlagName     = "logkeeper_url"
		logPrefixFlagName        = "log_prefix"
		s3BaseURLFlagName        = "s3_base_url"
		statusPortFlagName       = "status_port"
		cleanupFlagName          = "cleanup"
	)

	return cli.Command{
		Name:  "agent",
		Usage: "run an evergreen agent",
		Subcommands: []cli.Command{
			agentMonitor(),
		},
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
				Name:  apiServerURLFlagName,
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
			cli.StringFlag{
				Name:  logkeeperURLFlagName,
				Usage: "URL of the logkeeper service to be used by tasks",
			},
			cli.StringFlag{
				Name:  s3BaseURLFlagName,
				Usage: "base URL for S3 uploads (defaults to 'https://s3.amazonaws.com'",
			},
			cli.IntFlag{
				Name:  statusPortFlagName,
				Value: defaultAgentStatusPort,
				Usage: "port to run the status server",
			},
			cli.BoolFlag{
				Name:  cleanupFlagName,
				Usage: "clean up working directory and processes (do not set for smoke tests)",
			},
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(apiServerURLFlagName),
			requireStringFlag(hostIDFlagName),
			requireStringFlag(hostSecretFlagName),
			requireStringFlag(workingDirectoryFlagName),
			func(c *cli.Context) error {
				grip.SetName("evergreen.agent")
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			s3Base := c.String(s3BaseURLFlagName)
			if s3Base == "" {
				s3Base = "https://s3.amazonaws.com"
			}
			opts := agent.Options{
				HostID:           c.String(hostIDFlagName),
				HostSecret:       c.String(hostSecretFlagName),
				StatusPort:       c.Int(statusPortFlagName),
				LogPrefix:        c.String(logPrefixFlagName),
				WorkingDirectory: c.String(workingDirectoryFlagName),
				Cleanup:          c.Bool(cleanupFlagName),
				LogkeeperURL:     c.String(logkeeperURLFlagName),
				S3BaseURL:        s3Base,
				S3Opts: pail.S3Options{
					Credentials: pail.CreateAWSCredentials(os.Getenv("S3_KEY"), os.Getenv("S3_SECRET"), ""),
					Region:      endpoints.UsEast1RegionID,
					Name:        os.Getenv("S3_BUCKET"),
					Permissions: pail.S3PermissionsPublicRead,
					ContentType: "text/plain",
				},
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

			comm := client.NewCommunicator(c.String(apiServerURLFlagName))
			defer comm.Close()

			agt, err := agent.New(opts, comm)
			if err != nil {
				return errors.Wrap(err, "problem constructing agent")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go hardShutdownForSignals(ctx, cancel)

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

func hardShutdownForSignals(ctx context.Context, serviceCanceler context.CancelFunc) {
	defer recovery.LogStackTraceAndExit("agent signal handler")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-sigChan:
		grip.Info("service exiting after receiving signal")
	}
	serviceCanceler()
	os.Exit(2)
}
