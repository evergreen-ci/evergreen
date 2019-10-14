package cli

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/evergreen-ci/service"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Constants representing the Jasper service interface as a CLI command.
const (
	ServiceCommand        = "service"
	InstallCommand        = "install"
	UninstallCommand      = "uninstall"
	StartCommand          = "start"
	StopCommand           = "stop"
	RestartCommand        = "restart"
	RunCommand            = "run"
	StatusCommand         = "status"
	ForceReinstallCommand = "force-reinstall"
)

// Constants representing the supported Jasper service types.
const (
	RPCService      = "rpc"
	RESTService     = "rest"
	CombinedService = "combined"
)

// Constants representing service flags.
const (
	quietFlagName    = "quiet"
	userFlagName     = "user"
	passwordFlagName = "password"

	logNameFlagName = "log_name"
	defaultLogName  = "jasper"

	logLevelFlagName = "log_level"

	splunkURLFlagName           = "splunk_url"
	splunkTokenFlagName         = "splunk_token"
	splunkTokenFilePathFlagName = "splunk_token_path"
	splunkChannelFlagName       = "splunk_channel"

	// Flags related to resource limits.
	limitNumFilesFlagName      = "limit_num_files"
	limitNumProcsFlagName      = "limit_num_procs"
	limitLockedMemoryFlagName  = "limit_locked_memory"
	limitVirtualMemoryFlagName = "limit_virtual_memory"
)

// Service encapsulates the functionality to set up Jasper services.
// Except for run, the subcommands will generally require elevated privileges to
// execute.
func Service() cli.Command {
	return cli.Command{
		Name:  ServiceCommand,
		Usage: "tools for running Jasper services",
		Subcommands: []cli.Command{
			serviceCommand(ForceReinstallCommand, forceReinstall),
			serviceCommand(InstallCommand, install),
			serviceCommand(UninstallCommand, uninstall),
			serviceCommand(StartCommand, start),
			serviceCommand(StopCommand, stop),
			serviceCommand(RestartCommand, restart),
			serviceCommand(RunCommand, run),
			serviceCommand(StatusCommand, status),
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

func serviceFlags() []cli.Flag {
	return []cli.Flag{
		cli.BoolFlag{
			Name:  quietFlagName,
			Usage: "quiet mode - suppress errors when running the command",
		},
		cli.StringFlag{
			Name:  userFlagName,
			Usage: "the user who running the service",
		},
		cli.StringFlag{
			Name:   passwordFlagName,
			Usage:  "the password for the user running the service",
			EnvVar: "JASPER_USER_PASSWORD",
		},
		cli.StringFlag{
			Name:  logNameFlagName,
			Usage: "the name of the logger",
			Value: defaultLogName,
		},
		cli.StringFlag{
			Name:  logLevelFlagName,
			Usage: "the threshold visible logging level",
			Value: level.Error.String(),
		},
		cli.StringFlag{
			Name:   splunkURLFlagName,
			Usage:  "the URL of the splunk server",
			EnvVar: "GRIP_SPLUNK_SERVER_URL",
		},
		cli.StringFlag{
			Name:   splunkTokenFlagName,
			Usage:  "the splunk token",
			EnvVar: "GRIP_SPLUNK_CLIENT_TOKEN",
		},
		cli.StringFlag{
			Name:  splunkTokenFilePathFlagName,
			Usage: "the path to the file containing the splunk token",
		},
		cli.StringFlag{
			Name:   splunkChannelFlagName,
			Usage:  "the splunk channel",
			EnvVar: "GRIP_SPLUNK_CHANNEL",
		},
		cli.IntFlag{
			Name:  limitNumFilesFlagName,
			Usage: "the maximum number of open file descriptors. Specify -1 for no limit",
		},
		cli.IntFlag{
			Name:  limitNumProcsFlagName,
			Usage: "the maximum number of processes. Specify -1 for no limit",
		},
		cli.IntFlag{
			Name:  limitLockedMemoryFlagName,
			Usage: "the maximum size that may be locked into memory (kB). Specify -1 for no limit",
		},
		cli.IntFlag{
			Name:  limitVirtualMemoryFlagName,
			Usage: "the maximum available virtual memory (kB). Specify -1 for no limit",
		},
	}
}

func validateLimits(flagNames ...string) func(*cli.Context) error {
	return func(c *cli.Context) error {
		catcher := grip.NewBasicCatcher()
		for _, flagName := range flagNames {
			l := c.Int(flagName)
			if l < -1 {
				catcher.Errorf("%s is not a valid limit value for %s", l, flagName)
			}
		}
		return catcher.Resolve()
	}
}

func validateLogLevel(flagName string) func(*cli.Context) error {
	return func(c *cli.Context) error {
		l := c.String(logLevelFlagName)
		priority := level.FromString(l)
		if !level.IsValidPriority(priority) {
			return errors.Errorf("%s is not a valid log level", l)
		}
		return nil
	}
}

// makeLogger creates a splunk logger. It may return nil if the splunk flags are
// not populated.
func makeLogger(c *cli.Context) *options.Logger {
	info := send.SplunkConnectionInfo{
		ServerURL: c.String(splunkURLFlagName),
		Token:     c.String(splunkTokenFlagName),
		Channel:   c.String(splunkChannelFlagName),
	}
	if info.Token == "" {
		if tokenFilePath := c.String(splunkTokenFilePathFlagName); tokenFilePath != "" {
			token, err := ioutil.ReadFile(tokenFilePath)
			if err != nil {
				grip.Error(errors.Wrapf(err, "could not read splunk token file from path '%s'", tokenFilePath))
				return nil
			}
			info.Token = string(token)
		}
	}
	if !info.Populated() {
		return nil
	}

	l := c.String(logLevelFlagName)
	priority := level.FromString(l)
	if !level.IsValidPriority(priority) {
		return nil
	}

	return &options.Logger{
		Type: options.LogSplunk,
		Options: options.Log{
			Format:        options.LogFormatDefault,
			Level:         send.LevelInfo{Default: priority, Threshold: priority},
			SplunkOptions: info,
		},
	}
}

// buildRunCommand builds the command arguments to run the Jasper service with
// the flags set in the cli.Context.
func buildRunCommand(c *cli.Context, serviceType string) []string {
	args := unparseFlagSet(c)
	subCmd := []string{JasperCommand, ServiceCommand, RunCommand, serviceType}
	return append(subCmd, args...)
}

// serviceOptions returns all options specific to particular service management
// systems.
func serviceOptions(c *cli.Context) service.KeyValue {
	opts := service.KeyValue{
		// launchd-specific options
		"RunAtLoad": true,
		// Windows-specific options
		"Password": c.String(passwordFlagName),
	}

	// Linux-specific resource limit options
	if limit := resourceLimit(c.Int(limitNumFilesFlagName)); limit != "" {
		opts["LimitNumFiles"] = limit
	}
	if limit := resourceLimit(c.Int(limitNumProcsFlagName)); limit != "" {
		opts["LimitNumProcs"] = limit
	}
	if limit := resourceLimit(c.Int(limitLockedMemoryFlagName)); limit != "" {
		opts["LimitLockedMemory"] = limit
	}
	if limit := resourceLimit(c.Int(limitVirtualMemoryFlagName)); limit != "" {
		opts["LimitVirtualMemory"] = limit
	}

	return opts
}

func resourceLimit(limit int) string {
	system := service.ChosenSystem()
	if system == nil {
		return ""
	}
	if limit < -1 || limit == 0 {
		return ""
	}
	switch system.String() {
	case "linux-systemd":
		if limit == -1 {
			return "infinity"
		}
	case "linux-upstart", "unix-systemv":
		if limit == -1 {
			return "unlimited"
		}
	default:
		return ""
	}

	return strconv.Itoa(limit)
}

// serviceConfig returns the daemon service configuration.
func serviceConfig(serviceType string, c *cli.Context, args []string) *service.Config {
	return &service.Config{
		Name:        fmt.Sprintf("%s_jasperd", serviceType),
		DisplayName: fmt.Sprintf("Jasper %s service", serviceType),
		Description: "Jasper is a service for process management",
		Executable:  "", // No executable refers to the current executable.
		Arguments:   args,
		Option:      serviceOptions(c),
		UserName:    c.String(userFlagName),
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
		stopErr := message.WrapError(svc.Stop(), message.Fields{
			"msg":    "error stopping service",
			"cmd":    "force-reinstall",
			"config": *config,
		})
		uninstallErr := message.WrapError(svc.Uninstall(), message.Fields{
			"msg":    "error uninstalling service",
			"cmd":    "force-reinstall",
			"config": *config,
		})

		catcher := grip.NewBasicCatcher()
		catcher.Wrap(svc.Install(), "error installing service")
		catcher.Wrap(svc.Start(), "error starting service")
		if catcher.HasErrors() {
			grip.Debug(stopErr)
			grip.Debug(uninstallErr)
		}
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
			return errors.Wrapf(writeOutput(os.Stdout, &ServiceStatusResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error getting status from service"))}), "error writing to standard output")
		}
		return errors.Wrapf(writeOutput(os.Stdout, &ServiceStatusResponse{Status: statusToString(status), OutcomeResponse: *makeOutcomeResponse(nil)}), "error writing status to standard output")
	}), "error getting service status")
}

// ServiceStatus represents the state of the service.
type ServiceStatus string

// Constants representing the status of the service.
const (
	ServiceRunning ServiceStatus = "running"
	ServiceStopped ServiceStatus = "stopped"
	ServiceInvalid ServiceStatus = "invalid"
	ServiceUnknown ServiceStatus = "unknown"
)

// statusToString converts a service.Status code into a string ServiceStatus.
func statusToString(status service.Status) ServiceStatus {
	switch status {
	case service.StatusUnknown:
		return ServiceUnknown
	case service.StatusRunning:
		return ServiceRunning
	case service.StatusStopped:
		return ServiceStopped
	default:
		return ServiceInvalid
	}
}
