package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kardianos/service"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Service encapsulates the functionality to set up Jasper services. Except for
// run, the subcommands will generally require elevated privileges to execute.
func Service() cli.Command {
	return cli.Command{
		Name:  "service",
		Usage: "tools for running Jasper services",
		Flags: []cli.Flag{},
		Subcommands: []cli.Command{
			serviceCommand("force-reinstall", forceReinstall),
			serviceCommand("install", install),
			serviceCommand("uninstall", uninstall),
			serviceCommand("start", start),
			serviceCommand("stop", stop),
			serviceCommand("restart", restart),
			serviceCommand("run", run),
			serviceCommand("status", status),
		},
	}
}

// handleDaemonSignals shuts down the daemon by cancelling the context, either
// when the context is done, it receives a terminate signal, or when it
// receives a signal to exit the daemon.
func handleDaemonSignals(ctx context.Context, cancel context.CancelFunc, exit chan struct{}) {
	defer recovery.LogStackTraceAndContinue("graceful shutdown")
	defer cancel()
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGTERM, os.Interrupt)

	select {
	case <-sig:
		grip.Debug("received signal")
	case <-ctx.Done():
		grip.Debug("context canceled")
	case <-exit:
		grip.Debug("received daemon exit signal")
	}
}

// buildRunCommand builds the command arguments to run the Jasper service with
// the flags set in the cli.Context.
func buildRunCommand(c *cli.Context, serviceType string) []string {
	args := unparseFlagSet(c)
	subCmd := []string{"jasper", "service", "run", serviceType}
	return append(subCmd, args...)
}

// serviceOptions returns all options specific to particular service management
// systems.
func serviceOptions() service.KeyValue {
	return service.KeyValue{
		// launchd-specific options
		"RunAtLoad": true,
	}
}

// serviceConfig returns the daemon service configuration.
func serviceConfig(serviceType string, args []string) *service.Config {
	return &service.Config{
		Name:        fmt.Sprintf("%s_jasperd", serviceType),
		DisplayName: fmt.Sprintf("Jasper %s service", serviceType),
		Description: "Jasper is a service for process management",
		Executable:  "", // No executable refers to the current executable.
		Arguments:   args,
		Option:      serviceOptions(),
	}
}

type serviceOperation func(daemon service.Interface, config *service.Config) error

// serviceCommand creates a cli.Command from a service operation supported by
// REST, RPC, and combined services.
func serviceCommand(cmd string, operation serviceOperation) cli.Command {
	return cli.Command{
		Name:  cmd,
		Usage: fmt.Sprintf("%s a daemon service", cmd),
		Subcommands: []cli.Command{
			serviceCommandREST(cmd, operation),
			serviceCommandRPC(cmd, operation),
			serviceCommandCombined(cmd, operation),
		},
	}
}

// forceReinstall stops the service if it is running, reinstalls the service
// with the new configuration, and starts the newly-configured service. It only
// returns an error if there is an error while installing or starting the new
// service.
func forceReinstall(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		grip.Debug(message.WrapError(svc.Stop(), message.Fields{
			"msg":    "error stopping service",
			"cmd":    "force-reinstall",
			"config": *config,
		}))
		grip.Debug(message.WrapError(svc.Uninstall(), message.Fields{
			"msg":    "error uninstalling service",
			"cmd":    "force-reinstall",
			"config": *config,
		}))

		catcher := grip.NewBasicCatcher()
		catcher.Wrap(svc.Install(), "error installing service")
		catcher.Wrap(svc.Start(), "error starting service")
		return catcher.Resolve()
	}), "error force reinstalling service")
}

// install registers the service with the given configuration in the service
// manager.
func install(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		return svc.Install()
	}), "error installing service")
}

// uninstall removes the service from the service manager.
func uninstall(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		return svc.Uninstall()
	}), "error uninstalling service")
}

// start begins the service if it has not already started.
func start(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		return svc.Start()
	}), "error starting service")
}

// stop ends the running service.
func stop(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		return svc.Stop()
	}), "error stopping service")
}

// restart stops the existing service and starts it again.
func restart(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		return svc.Restart()
	}), "error restarting service")
}

// run runs the service in the foreground.
func run(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		return svc.Run()
	}), "error running service")
}

// status gets the current status of the running service.
func status(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		status, err := svc.Status()
		if err != nil {
			return err
		}
		return errors.Wrap(writeOutput(os.Stdout, status), "error writing status")
	}), "error getting service status")
}
