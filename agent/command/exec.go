package command

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/shlex"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

type subprocessExec struct {
	Binary    string            `mapstructure:"binary"`
	Args      []string          `mapstructure:"args"`
	Env       map[string]string `mapstructure:"env"`
	Command   string            `mapstructure:"command"`
	AddToPath []string          `mapstructure:"add_to_path"`

	// Add defined expansions to the environment of the process
	// that's launched.
	AddExpansionsToEnv bool `mapstructure:"add_expansions_to_env"`

	// IncludeExpansionsInEnv allows users to specify a number of
	// expansions that will be included in the environment, if
	// they are defined. It is not an error to specify expansions
	// that are not defined in include_expansions_in_env.
	IncludeExpansionsInEnv []string `mapstructure:"include_expansions_in_env"`

	// Background, if set to true, prevents shell code/output from
	// waiting for the script to complete and immediately returns
	// to the caller
	Background bool `mapstructure:"background"`

	// Silent, if set to true, prevents shell code/output from being
	// logged to the agent's task logs. This can be used to avoid
	// exposing sensitive expansion parameters and keys.
	Silent bool `mapstructure:"silent"`

	// SystemLog if set will write the shell command's output to the system logs, instead of the
	// task logs. This can be used to collect diagnostic data in the background of a running task.
	SystemLog bool `mapstructure:"system_log"`

	// WorkingDir is the working directory to start the shell in.
	WorkingDir string `mapstructure:"working_dir"`

	// IgnoreStandardOutput and IgnoreStandardError allow users to
	// elect to ignore either standard out and/or standard output.
	IgnoreStandardOutput bool `mapstructure:"ignore_standard_out"`
	IgnoreStandardError  bool `mapstructure:"ignore_standard_error"`

	// RedirectStandardErrorToOutput allows you to capture
	// standard error in the same stream as standard output. This
	// improves the synchronization of these streams.
	RedirectStandardErrorToOutput bool `mapstructure:"redirect_standard_error_to_output"`

	// ContinueOnError determines whether or not a failed return code
	// should cause the task to be marked as failed. Setting this to true
	// allows following commands to execute even if this shell command fails.
	ContinueOnError bool `mapstructure:"continue_on_err"`

	// KeepEmptyArgs will allow empty arguments in commands if set to true
	// note that non-blank whitespace arguments are never stripped
	KeepEmptyArgs bool `mapstructure:"keep_empty_args"`

	base
}

func subprocessExecFactory() Command   { return &subprocessExec{} }
func (c *subprocessExec) Name() string { return "subprocess.exec" }

func (c *subprocessExec) ParseParams(params map[string]any) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if c.Command != "" {
		if c.Binary != "" || len(c.Args) > 0 {
			return errors.New("must specify command as either binary and arguments, or a command string, but not both")
		}

		args, err := shlex.Split(c.Command)
		if err != nil {
			return errors.Wrapf(err, "parsing command using shell lexing rules")
		}
		if len(args) == 0 {
			return errors.Errorf("command could not be split using shell lexing rules")
		}

		c.Binary = args[0]
		if len(args) > 1 {
			c.Args = args[1:]
		}
	}

	if c.Silent {
		c.IgnoreStandardError = true
		c.IgnoreStandardOutput = true
	}

	if c.IgnoreStandardOutput && c.RedirectStandardErrorToOutput {
		return errors.New("cannot both ignore standard output and redirect standard error to it")
	}

	if c.Env == nil {
		c.Env = make(map[string]string)
	}

	return nil
}

func (c *subprocessExec) doExpansions(exp *util.Expansions) error {
	var err error
	catcher := grip.NewBasicCatcher()

	c.WorkingDir, err = exp.ExpandString(c.WorkingDir)
	catcher.Wrap(err, "expanding working directory")

	c.Binary, err = exp.ExpandString(c.Binary)
	catcher.Wrap(err, "expanding binary")

	for idx := range c.Args {
		c.Args[idx], err = exp.ExpandString(c.Args[idx])
		catcher.Wrap(err, "expanding args")
	}

	for k, v := range c.Env {
		c.Env[k], err = exp.ExpandString(v)
		catcher.Wrap(err, "expanding environment variables")
	}

	for idx := range c.AddToPath {
		c.AddToPath[idx], err = exp.ExpandString(c.AddToPath[idx])
		catcher.Wrap(err, "expanding path to add")
	}

	return errors.Wrap(catcher.Resolve(), "expanding strings")
}

type modifyEnvOptions struct {
	taskID                 string
	workingDir             string
	tmpDir                 string
	expansions             util.Expansions
	includeExpansionsInEnv []string
	addExpansionsToEnv     bool
	addToPath              []string
}

func defaultAndApplyExpansionsToEnv(env map[string]string, opts modifyEnvOptions) map[string]string {
	if env == nil {
		env = map[string]string{}
	}

	if len(opts.addToPath) > 0 {
		// Prepend paths to the runtime environment's PATH. More reasonable
		// behavior here would be to respect the PATH env var if it's explicitly
		// set for the command, but changing this could break existing
		// workflows, so we don't do that.
		path := make([]string, 0, len(opts.addToPath)+1)
		path = append(path, opts.addToPath...)
		path = append(path, os.Getenv("PATH"))
		env["PATH"] = strings.Join(path, string(filepath.ListSeparator))
	}

	expansions := opts.expansions.Map()
	if opts.addExpansionsToEnv {
		for k, v := range expansions {
			env[k] = v
		}
	}

	for _, expName := range opts.includeExpansionsInEnv {
		if val, ok := expansions[expName]; ok {
			env[expName] = val
		}
	}

	env[agentutil.MarkerTaskID] = opts.taskID
	env[agentutil.MarkerAgentPID] = strconv.Itoa(os.Getpid())

	addTempDirs(env, opts.tmpDir)

	if _, ok := env["GOCACHE"]; !ok {
		env["GOCACHE"] = filepath.Join(opts.workingDir, ".gocache")
	}
	if _, ok := env["CI"]; !ok {
		env["CI"] = "true"
	}

	return env
}

func addTempDirs(env map[string]string, dir string) {
	for _, key := range []string{"TMP", "TMPDIR", "TEMP"} {
		if _, ok := env[key]; ok {
			continue
		}
		env[key] = dir
	}
}

func (c *subprocessExec) getProc(ctx context.Context, execPath string, conf *internal.TaskConfig, logger client.LoggerProducer) *jasper.Command {
	cmd := c.JasperManager().CreateCommand(ctx).Add(append([]string{execPath}, c.Args...)).
		Background(c.Background).Environment(c.Env).Directory(c.WorkingDir).
		SuppressStandardError(c.IgnoreStandardError).SuppressStandardOutput(c.IgnoreStandardOutput).RedirectErrorToOutput(c.RedirectStandardErrorToOutput).
		ProcConstructor(func(lctx context.Context, opts *options.Create) (jasper.Process, error) {
			return runJasperProcess(lctx, c.JasperManager(), c.Background, opts, conf.Task.Id, logger)
		})

	if !c.IgnoreStandardOutput {
		if c.SystemLog {
			cmd.SetOutputSender(level.Info, logger.System().GetSender())
		} else {
			cmd.SetOutputSender(level.Info, logger.Task().GetSender())
		}
	}

	if !c.IgnoreStandardError {
		if c.SystemLog {
			cmd.SetErrorSender(level.Error, logger.System().GetSender())
		} else {
			cmd.SetErrorSender(level.Error, logger.Task().GetSender())
		}
	}

	if conf.Distro != nil {
		if execUser := conf.Distro.ExecUser; execUser != "" {
			cmd.SudoAs(execUser)
		}
	}

	return cmd
}

// runJasperProcess starts a Jasper process. This does not wait for the process
// to exit.
func runJasperProcess(ctx context.Context, jpm jasper.Manager, background bool, opts *options.Create, taskID string, logger client.LoggerProducer) (jasper.Process, error) {
	var cancel context.CancelFunc
	var ictx context.Context
	if background {
		ictx, cancel = context.WithCancel(context.Background())
	} else {
		ictx = ctx
	}

	// This momentarily sets the nice for this thread back to the default nice
	// to ensure the process that's about to be created and all of its children
	// processes use the default nice (child processes inherit the nice of the
	// parent process). This thread will have no special nice until it's reset
	// but that should be a brief window.
	// Passing 0 as the PID refers to the current thread.
	if niceErr := agentutil.SetNice(0, agentutil.DefaultNice); niceErr != nil {
		logger.Execution().Warningf("Unable to set agent's nice to %d before starting subprocess, subprocess may have non-default nice when it starts. Error: %s", agentutil.DefaultNice, niceErr.Error())
	}

	proc, err := jpm.CreateProcess(ictx, opts)
	if err != nil {
		if cancel != nil {
			cancel()
		}

		return proc, errors.WithStack(err)
	}

	// Once the child processes has started, reset the agent's nice back to the
	// lower nice value to ensure that this agent thread will have its original
	// CPU priority.
	if niceErr := agentutil.SetNice(0, agentutil.AgentNice); niceErr != nil {
		logger.Execution().Warningf("Unable to set agent's nice to %d before starting shell subprocess, shell may have non-default nice when it starts. Error: %s", agentutil.AgentNice, niceErr.Error())
	}

	if cancel != nil {
		grip.Warning(message.WrapError(proc.RegisterTrigger(ctx, func(info jasper.ProcessInfo) {
			cancel()
		}), "registering canceller for process"))
	}

	pid := proc.Info(ctx).PID

	agentutil.TrackProcess(taskID, pid, logger.System())

	if background {
		logger.Execution().Debugf("Running process in the background with PID %d.", pid)
	} else {
		logger.Execution().Infof("Started process with PID %d.", pid)
	}

	return proc, nil
}

// getExecutablePath returns the path to the command executable to run.
// If the executable is available in the default runtime environment's PATH or
// it is a file path (i.e. it's not supposed to be found in the PATH), then the
// executable binary will be returned as-is. Otherwise if it can't find the
// command in the default PATH locations, the command will fall back to checking
// the command's PATH environment variable for a matching executable location
// (if any).
func (c *subprocessExec) getExecutablePath(logger client.LoggerProducer) (absPath string, err error) {
	defaultPath, err := exec.LookPath(c.Binary)
	if defaultPath != "" {
		return c.Binary, err
	}

	cmdPath := c.Env["PATH"]
	// For non-Windows platforms, the filepath.Separator is always '/'. However,
	// for Windows, Go accepts both '\' and '/' as valid file path separators,
	// even though the native filepath.Separator for Windows is really '\'. This
	// detects both '\' and '/' as valid Windows file path separators.
	binaryIsFilePath := strings.Contains(c.Binary, string(filepath.Separator)) || runtime.GOOS == "windows" && strings.Contains(c.Binary, "/")

	if len(cmdPath) == 0 || binaryIsFilePath {
		return c.Binary, nil
	}

	logger.Execution().Debug("could not find executable binary in the default runtime environment PATH, falling back to trying the command's PATH")

	originalPath := os.Getenv("PATH")
	defer func() {
		// Try to reset the PATH back to its original state. If this fails, then
		// the agent may have a modified PATH, which will affect all future
		// agent operations that need to execute processes. However, given that
		// the potential ways this could fail to reset seem highly unlikely
		// (e.g. due to having insufficient memory to reset it) , it doesn't
		// seem worth handling in a better way.
		if resetErr := os.Setenv("PATH", originalPath); resetErr != nil {
			logger.Execution().Error(errors.Wrap(resetErr, "resetting agent's PATH env var back to its original state").Error())
		}
	}()

	if err := os.Setenv("PATH", cmdPath); err != nil {
		return c.Binary, errors.Wrap(err, "setting command's PATH to try fallback executable paths")
	}

	return exec.LookPath(c.Binary)
}

func (c *subprocessExec) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	var err error

	if err = c.doExpansions(&conf.Expansions); err != nil {
		return errors.Wrap(err, "expanding command parameters")
	}

	logger.Execution().WarningWhen(
		filepath.IsAbs(c.WorkingDir) && !strings.HasPrefix(c.WorkingDir, conf.WorkDir),
		message.Fields{
			"message":         "the working directory is an absolute path without the required prefix",
			"path":            c.WorkingDir,
			"required_prefix": conf.WorkDir,
		})
	c.WorkingDir, err = getWorkingDirectoryLegacy(conf, c.WorkingDir)
	if err != nil {
		return errors.Wrap(err, "getting working directory")
	}

	taskTmpDir, err := getWorkingDirectoryLegacy(conf, "tmp")
	if err != nil {
		logger.Execution().Notice(errors.Wrap(err, "getting temporary directory"))
	}

	c.Env = defaultAndApplyExpansionsToEnv(c.Env, modifyEnvOptions{
		taskID:                 conf.Task.Id,
		workingDir:             c.WorkingDir,
		tmpDir:                 taskTmpDir,
		expansions:             conf.Expansions,
		includeExpansionsInEnv: c.IncludeExpansionsInEnv,
		addExpansionsToEnv:     c.AddExpansionsToEnv,
		addToPath:              c.AddToPath,
	})

	if !c.KeepEmptyArgs {
		for i := len(c.Args) - 1; i >= 0; i-- {
			if c.Args[i] == "" {
				c.Args = append(c.Args[:i], c.Args[i+1:]...)
			}
		}
	}

	execPath, err := c.getExecutablePath(logger)
	if execPath == "" && err != nil {
		return errors.Wrap(err, "resolving executable path")
	}
	if err != nil {
		logger.Execution().Debug(message.WrapError(err, message.Fields{
			"message":           "found an executable path, but encountered errors while doing so",
			"working_directory": c.WorkingDir,
			"background":        c.Background,
			"binary":            c.Binary,
			"binary_path":       execPath,
		}))
	} else {
		logger.Execution().Debug(message.Fields{
			"working_directory": c.WorkingDir,
			"background":        c.Background,
			"binary":            c.Binary,
			"binary_path":       execPath,
		})
	}

	err = errors.WithStack(c.runCommand(ctx, c.getProc(ctx, execPath, conf, logger), logger))

	if ctxErr := ctx.Err(); ctxErr != nil {
		logger.System().Debugf("Canceled command '%s', dumping running processes.", c.Name())
		logger.System().Debug(message.CollectAllProcesses())
		logger.Execution().Notice(err)

		return errors.Wrapf(ctxErr, "canceled while running command '%s'", c.Name())
	}

	return err
}

func (c *subprocessExec) runCommand(ctx context.Context, cmd *jasper.Command, logger client.LoggerProducer) error {
	if c.Silent {
		logger.Execution().Info("Executing command in silent mode.")
	}

	err := cmd.Run(ctx)
	if !c.Background && err != nil {
		if exitCode, _ := cmd.Wait(ctx); exitCode != 0 {
			err = errors.Errorf("process encountered problem: exit code %d", exitCode)
		}
	}

	if c.ContinueOnError && err != nil {
		logger.Execution().Noticef("Script errored, but continue on error is set - continuing task execution. Error: %s.", err)
		return nil
	}

	return err
}
