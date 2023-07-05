package operations

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const defaultAgentStatusPort = 2285

const (
	agentAPIServerURLFlagName  = "api_server"
	agentCloudProviderFlagName = "provider"
)

func Agent() cli.Command {
	const (
		hostIDFlagName           = "host_id"
		hostSecretFlagName       = "host_secret"
		workingDirectoryFlagName = "working_directory"
		logOutputFlagName        = "log_output"
		logPrefixFlagName        = "log_prefix"
		statusPortFlagName       = "status_port"
		cleanupFlagName          = "cleanup"
		modeFlagName             = "mode"
		podIDFlagName            = "pod_id"
		podSecretFlagName        = "pod_secret"
		versionFlagName          = "version"
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
				Usage: "the ID of the host the agent is running on (applies only to host mode)",
			},
			cli.StringFlag{
				Name:  hostSecretFlagName,
				Usage: "secret for the current host (applies only to host mode)",
			},
			cli.StringFlag{
				Name:   podIDFlagName,
				Usage:  "the ID of the pod that the agent is running on (applies only to pod mode)",
				EnvVar: "POD_ID",
			},
			cli.StringFlag{
				Name:   podSecretFlagName,
				Usage:  "the secret for the pod that the agent is running on (applies only to pod mode)",
				EnvVar: "POD_SECRET",
			},
			cli.StringFlag{
				Name:  agentAPIServerURLFlagName,
				Usage: "URL of the API server",
			},
			cli.StringFlag{
				Name:  workingDirectoryFlagName,
				Usage: "working directory for the agent",
			},
			cli.StringFlag{
				Name:  logOutputFlagName,
				Value: string(agent.LogOutputFile),
				Usage: "location for the agent's log output (file, stdout)",
			},
			cli.StringFlag{
				Name:  logPrefixFlagName,
				Value: "evg.agent",
				Usage: "prefix for the agent's log output",
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
			cli.StringFlag{
				Name:  agentCloudProviderFlagName,
				Usage: "the cloud provider that manages this host",
			},
			cli.StringFlag{
				Name:  modeFlagName,
				Usage: "the mode that the agent should run in (host, pod)",
				Value: "host",
			},
			cli.BoolFlag{
				Name:  joinFlagNames(versionFlagName, "v"),
				Usage: "print the agent revision of the current binary and exit",
			},
		},
		Before: mergeBeforeFuncs(
			func(c *cli.Context) error {
				if c.Bool(versionFlagName) {
					return nil
				}

				catcher := grip.NewBasicCatcher()
				catcher.Add(requireStringFlag(agentAPIServerURLFlagName)(c))
				catcher.Add(requireStringFlag(workingDirectoryFlagName)(c))
				mode := c.String(modeFlagName)
				switch mode {
				case string(agent.HostMode):
					catcher.Add(requireStringFlag(hostIDFlagName)(c))
					catcher.Add(requireStringFlag(hostSecretFlagName)(c))
				case string(agent.PodMode):
					catcher.Add(requireStringFlag(podIDFlagName)(c))
					catcher.Add(requireStringFlag(podSecretFlagName)(c))
				default:
					return errors.Errorf("invalid mode '%s'", mode)
				}
				return catcher.Resolve()
			},
			func(c *cli.Context) error {
				grip.SetName("evergreen.agent")
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			if c.Bool(versionFlagName) {
				fmt.Println(evergreen.AgentVersion)
				return nil
			}

			opts := agent.Options{
				HostID:           c.String(hostIDFlagName),
				HostSecret:       c.String(hostSecretFlagName),
				PodID:            c.String(podIDFlagName),
				PodSecret:        c.String(podSecretFlagName),
				Mode:             agent.Mode(c.String(modeFlagName)),
				StatusPort:       c.Int(statusPortFlagName),
				LogPrefix:        c.String(logPrefixFlagName),
				LogOutput:        agent.LogOutputType(c.String(logOutputFlagName)),
				WorkingDirectory: c.String(workingDirectoryFlagName),
				Cleanup:          c.Bool(cleanupFlagName),
				CloudProvider:    c.String(agentCloudProviderFlagName),
			}

			if err := os.MkdirAll(opts.WorkingDirectory, 0777); err != nil {
				return errors.Wrapf(err, "creating working directory '%s'", opts.WorkingDirectory)
			}

			grip.Info(message.Fields{
				"message":  "starting agent",
				"commands": command.RegisteredCommandNames(),
				"dir":      opts.WorkingDirectory,
				"host_id":  opts.HostID,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			agt, err := agent.New(ctx, opts, c.String(agentAPIServerURLFlagName))
			if err != nil {
				return errors.Wrap(err, "constructing agent")
			}

			go hardShutdownForSignals(ctx, cancel, agt.Close)

			defer agt.Close(ctx)

			sender, err := agt.GetSender(ctx, opts.LogOutput, opts.LogPrefix)
			if err != nil {
				return errors.Wrap(err, "configuring logger")
			}

			if err = grip.SetSender(sender); err != nil {
				return errors.Wrap(err, "setting up global logger")
			}
			agt.SetDefaultLogger(sender)

			err = agt.Start(ctx)
			grip.Emergency(err)

			return err
		},
	}
}

func hardShutdownForSignals(ctx context.Context, serviceCanceler context.CancelFunc, closeAgent func(ctx context.Context)) {
	defer recovery.LogStackTraceAndExit("agent signal handler")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-sigChan:
		grip.Info("service exiting after receiving signal")
	}

	// Close may not succeed if the context is cancelled, but this is a
	// best-effort attempt to clean up before imminent shutdown anyways.
	closeAgent(ctx)

	serviceCanceler()
	os.Exit(2)
}
