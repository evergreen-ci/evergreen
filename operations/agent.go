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

func Agent() cli.Command {
	const (
		logPrefixFlagName  = "log_prefix"
		statusPortFlagName = "status_port"
		cleanupFlagName    = "cleanup"
		s3BaseFlagName     = "s3_base_url"
	)

	return cli.Command{
		Name:  "agent",
		Usage: "run an evergreen agent",
		Flags: append(addCommonAgentAndRunnerFlags(),
			cli.StringFlag{
				Name:  logPrefixFlagName,
				Value: "evg.agent",
				Usage: "prefix for the agent's log filename",
			},
			cli.StringFlag{
				Name:  s3BaseFlagName,
				Usage: "base URL for S3 uploads (defaults to 'https://s3.amazonaws.com'",
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
		),
		Before: mergeBeforeFuncs(
			append(requireAgentFlags(), func(c *cli.Context) error {
				grip.SetName("evergreen.agent")
				return nil
			})...,
		),
		Action: func(c *cli.Context) error {
			s3Base := c.String(s3BaseFlagName)
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
					Permission:  "public-read",
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
