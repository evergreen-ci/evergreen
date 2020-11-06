package cli

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/evergreen-ci/service"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Constants representing the Jasper service interface as a CLI command.
const (
	ServiceCommand               = "service"
	ServiceInstallCommand        = "install"
	ServiceUninstallCommand      = "uninstall"
	ServiceStartCommand          = "start"
	ServiceStopCommand           = "stop"
	ServiceRestartCommand        = "restart"
	ServiceRunCommand            = "run"
	ServiceStatusCommand         = "status"
	ServiceForceReinstallCommand = "force-reinstall"
)

// Constants representing the supported Jasper service types.
const (
	RPCService      = "rpc"
	RESTService     = "rest"
	CombinedService = "combined"
	WireService     = "wire"
)

// Constants representing service flags.
const (
	quietFlagName            = "quiet"
	userFlagName             = "user"
	passwordFlagName         = "password"
	interactiveFlagName      = "interactive"
	envFlagName              = "env"
	preconditionCmdsFlagName = "precondition"

	logNameFlagName  = "log_name"
	defaultLogName   = "jasper"
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

// serviceCmd encapsulates the functionality to set up Jasper services.
// Except for run, the subcommands will generally require elevated privileges to
// execute.
func serviceCmd() cli.Command {
	return cli.Command{
		Name:  ServiceCommand,
		Usage: "Tools for running Jasper services.",
		Subcommands: []cli.Command{
			serviceCommand(ServiceForceReinstallCommand, serviceForceReinstall),
			serviceCommand(ServiceInstallCommand, serviceInstall),
			serviceCommand(ServiceUninstallCommand, serviceUninstall),
			serviceCommand(ServiceStartCommand, serviceStart),
			serviceCommand(ServiceStopCommand, serviceStop),
			serviceCommand(ServiceRestartCommand, serviceRestart),
			serviceCommand(ServiceRunCommand, serviceRun),
			serviceCommand(ServiceStatusCommand, serviceStatus),
		},
	}
}

func serviceFlags() []cli.Flag {
	return []cli.Flag{
		cli.BoolFlag{
			Name:  quietFlagName,
			Usage: "Quiet mode - suppress errors when running the command.",
		},
		cli.StringFlag{
			Name:  userFlagName,
			Usage: "The user who the service will run as.",
		},
		cli.StringFlag{
			Name:   passwordFlagName,
			Usage:  "The password for the user running the service.",
			EnvVar: "JASPER_USER_PASSWORD",
		},
		cli.BoolFlag{
			Name:  interactiveFlagName,
			Usage: "Force the service to run in an interactive session.",
		},
		cli.StringSliceFlag{
			Name:  envFlagName,
			Usage: "The service environment variables (format: key=value).",
		},
		cli.StringSliceFlag{
			Name:  preconditionCmdsFlagName,
			Usage: "Execute command(s) that must be run and must succeed before the Jasper service can start.",
		},
		cli.StringFlag{
			Name:  logNameFlagName,
			Usage: "The name of the logger.",
			Value: defaultLogName,
		},
		cli.StringFlag{
			Name:  logLevelFlagName,
			Usage: "The threshold visible logging level.",
			Value: level.Error.String(),
		},
		cli.StringFlag{
			Name:   splunkURLFlagName,
			Usage:  "The URL of the splunk server for logging.",
			EnvVar: "GRIP_SPLUNK_SERVER_URL",
		},
		cli.StringFlag{
			Name:   splunkTokenFlagName,
			Usage:  "The token used for logging to splunk.",
			EnvVar: "GRIP_SPLUNK_CLIENT_TOKEN",
		},
		cli.StringFlag{
			Name:  splunkTokenFilePathFlagName,
			Usage: "The path to the file containing the splunk token.",
		},
		cli.StringFlag{
			Name:   splunkChannelFlagName,
			Usage:  "The splunk channel where logs should be sent.",
			EnvVar: "GRIP_SPLUNK_CHANNEL",
		},
		cli.IntFlag{
			Name:  limitNumFilesFlagName,
			Usage: "The maximum number of open file descriptors. Specify -1 for no limit.",
		},
		cli.IntFlag{
			Name:  limitNumProcsFlagName,
			Usage: "The maximum number of processes. Specify -1 for no limit.",
		},
		cli.IntFlag{
			Name:  limitLockedMemoryFlagName,
			Usage: "The maximum size that may be locked into memory (kB). Specify -1 for no limit.",
		},
		cli.IntFlag{
			Name:  limitVirtualMemoryFlagName,
			Usage: "The maximum available virtual memory (kB). Specify -1 for no limit.",
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
		if !priority.IsValid() {
			return errors.Errorf("%s is not a valid log level", l)
		}
		return nil
	}
}

// makeLogger creates a splunk logger. It may return nil if the splunk flags are
// not populated or the splunk logger is not registered.
func makeLogger(c *cli.Context) *options.LoggerConfig {
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
	if !priority.IsValid() {
		return nil
	}

	logger := &options.LoggerConfig{}
	producer := &options.SplunkLoggerOptions{
		Splunk: info,
		Base: options.BaseOptions{
			Format: options.LogFormatDefault,
			Level:  send.LevelInfo{Default: priority, Threshold: priority},
		},
	}
	if err := logger.Set(producer); err != nil {
		return nil
	}

	return logger
}

// buildServiceRunCommand builds the command arguments to run the Jasper service
// with the flags set in the cli.Context.
func buildServiceRunCommand(c *cli.Context, serviceType string) []string {
	args := unparseFlagSet(c, serviceType)
	var subCmd []string
	if filepath.Base(os.Args[0]) != JasperCommand {
		subCmd = append(subCmd, JasperCommand)
	}
	subCmd = append(subCmd, ServiceCommand, ServiceRunCommand, serviceType)
	return append(subCmd, args...)
}

// serviceOptions returns all options specific to particular service management
// systems.
func serviceOptions(c *cli.Context) service.KeyValue {
	opts := service.KeyValue{
		// launchd-specific options
		"RunAtLoad":     true,
		"SessionCreate": true,
		"ProcessType":   "Interactive",
		// Windows-specific options
		"Password": c.String(passwordFlagName),
	}

	// Linux/launchd-specific resource limit options
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
	case "linux-upstart", "unix-systemv", "darwin-launchd":
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
		Name:             fmt.Sprintf("%s_jasperd", serviceType),
		DisplayName:      fmt.Sprintf("Jasper %s service", serviceType),
		Description:      "Jasper is a service for process management",
		Executable:       "", // No executable refers to the current executable.
		Arguments:        args,
		Environment:      makeUserEnvironment(c.String(userFlagName), c.StringSlice(envFlagName)),
		UserName:         c.String(userFlagName),
		ForceInteractive: c.Bool(interactiveFlagName),
		Option:           serviceOptions(c),
	}
}

// makeUserEnvironment sets up the environment variables for the service. It
// attempts to reads the common user environment variables from /etc/passwd for
// upstart and sysv.
func makeUserEnvironment(user string, vars []string) map[string]string { //nolint: gocognit
	env := map[string]string{}
	for _, v := range vars {
		keyAndValue := strings.Split(v, "=")
		if len(keyAndValue) == 2 {
			env[keyAndValue[0]] = keyAndValue[1]
		}
	}

	if user == "" {
		return env
	}
	system := service.ChosenSystem()
	if system == nil || (system.String() != "linux-upstart" && system.String() != "unix-systemv") {
		return env
	}
	// Content and format of /etc/passwd is documented here:
	// https://linux.die.net/man/5/passwd
	file := "/etc/passwd"
	content, err := ioutil.ReadFile(file)
	if err != nil {
		grip.Debug(message.WrapErrorf(err, "could not read file '%s'", file))
		return env
	}

	const numEtcPasswdFields = 7
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, user+":") {
			fields := strings.Split(line, ":")
			if len(fields) == numEtcPasswdFields {
				if _, ok := env["USER"]; !ok {
					env["USER"] = user
				}
				if _, ok := env["LOGNAME"]; !ok {
					env["LOGNAME"] = user
				}
				if _, ok := env["HOME"]; !ok {
					env["HOME"] = fields[numEtcPasswdFields-2]
				}
				if _, ok := env["SHELL"]; !ok {
					env["SHELL"] = fields[numEtcPasswdFields-1]
				}
				return env
			}
		}
	}
	grip.Debug(message.WrapErrorf(err, "could not find user environment variables in file '%s'", file))
	return env
}

type serviceOperation func(daemon service.Interface, config *service.Config) error

// serviceCommand creates a cli.Command from a service operation supported by
// REST, RPC, and combined services.
func serviceCommand(cmd string, operation serviceOperation) cli.Command {
	return cli.Command{
		Name:  cmd,
		Usage: fmt.Sprintf("%s a daemon service.", cmd),
		Subcommands: []cli.Command{
			serviceCommandREST(cmd, operation),
			serviceCommandRPC(cmd, operation),
			serviceCommandCombined(cmd, operation),
			serviceCommandWire(cmd, operation),
		},
	}
}

// serviceForceReinstall stops the service if it is running, reinstalls the service
// with the new configuration, and starts the newly-configured service. It only
// returns an error if there is an error while installing or starting the new
// service.
func serviceForceReinstall(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		stopErr := message.WrapError(svc.Stop(), message.Fields{
			"msg":    "error stopping service",
			"cmd":    "force-reinstall",
			"config": fmt.Sprintf("%#v", *config),
		})
		uninstallErr := message.WrapError(svc.Uninstall(), message.Fields{
			"msg":    "error uninstalling service",
			"cmd":    "force-reinstall",
			"config": fmt.Sprintf("%#v", *config),
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

// serviceInstall registers the service with the given configuration in the
// service manager.
func serviceInstall(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		return svc.Install()
	}), "error installing service")
}

// serviceUninstall removes the service from the service manager.
func serviceUninstall(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		return svc.Uninstall()
	}), "error uninstalling service")
}

// serviceStart begins the service if it has not already started.
func serviceStart(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		return svc.Start()
	}), "error starting service")
}

// serviceStop ends the running service.
func serviceStop(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		return svc.Stop()
	}), "error stopping service")
}

// serviceRestart stops the existing service and starts it again.
func serviceRestart(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		return svc.Restart()
	}), "error restarting service")
}

// serviceRun runs the service in the foreground.
func serviceRun(daemon service.Interface, config *service.Config) error {
	return errors.Wrap(withService(daemon, config, func(svc service.Service) error {
		return svc.Run()
	}), "error running service")
}

// serviceStatus gets the current status of the running service.
func serviceStatus(daemon service.Interface, config *service.Config) error {
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
