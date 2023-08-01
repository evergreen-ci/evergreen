package command

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
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
	Binary  string            `mapstructure:"binary"`
	Args    []string          `mapstructure:"args"`
	Env     map[string]string `mapstructure:"env"`
	Command string            `mapstructure:"command"`
	Path    []string          `mapstructure:"add_to_path"`

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

func (c *subprocessExec) ParseParams(params map[string]interface{}) error {
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

	// kim: TODO: update docs about precedence of PATH env var.
	// * If add_to_path is specified: add_to_path, agent PATH (explicit PATH is
	// overwritten).
	// * If explicit PATH is specified, only explicit PATH is available.
	// * Otherwise (PATH not specified), does not have PATH defined.
	for k, v := range c.Env {
		c.Env[k], err = exp.ExpandString(v)
		catcher.Wrap(err, "expanding environment variables")
	}

	for idx := range c.Path {
		c.Path[idx], err = exp.ExpandString(c.Path[idx])
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
		// Prepend paths to the agent process's PATH. More reasonable behavior
		// here would be to respect the PATH env var if it's explicitly set, but
		// changing this could break existing workflows, so we don't do that.
		path := append(opts.addToPath, os.Getenv("PATH"))
		env["PATH"] = strings.Join(path, string(filepath.ListSeparator))
	}

	expansions := opts.expansions.Map()
	if opts.addExpansionsToEnv {
		for k, v := range expansions {
			if k == evergreen.GlobalGitHubTokenExpansion || k == evergreen.GithubAppToken {
				//users should not be able to use the global github token expansion
				//as it can result in the breaching of Evergreen's GitHub API limit
				continue
			}
			env[k] = v
		}
	}

	for _, expName := range opts.includeExpansionsInEnv {
		if val, ok := expansions[expName]; ok && expName != evergreen.GlobalGitHubTokenExpansion && expName != evergreen.GithubAppToken {
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

func (c *subprocessExec) getProc(ctx context.Context, execPath, taskID string, logger client.LoggerProducer) *jasper.Command {
	cmd := c.JasperManager().CreateCommand(ctx).Add(append([]string{execPath}, c.Args...)).
		Background(c.Background).Environment(c.Env).Directory(c.WorkingDir).
		SuppressStandardError(c.IgnoreStandardError).SuppressStandardOutput(c.IgnoreStandardOutput).RedirectErrorToOutput(c.RedirectStandardErrorToOutput).
		ProcConstructor(func(lctx context.Context, opts *options.Create) (jasper.Process, error) {
			var cancel context.CancelFunc
			var ictx context.Context
			if c.Background {
				ictx, cancel = context.WithCancel(context.Background())
			} else {
				ictx = lctx
			}

			proc, err := c.JasperManager().CreateProcess(ictx, opts)
			if err != nil {
				if cancel != nil {
					cancel()
				}

				return proc, errors.WithStack(err)
			}

			if cancel != nil {
				grip.Warning(message.WrapError(proc.RegisterTrigger(lctx, func(info jasper.ProcessInfo) {
					cancel()
				}), "registering canceller for process"))
			}

			pid := proc.Info(ctx).PID

			agentutil.TrackProcess(taskID, pid, logger.System())

			if c.Background {
				logger.Execution().Debugf("Running process in the background with pid %d.", pid)
			} else {
				logger.Execution().Infof("Started process with pid %d.", pid)
			}

			return proc, nil
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

	return cmd
}

// getExecutablePath returns the absolute path to the command executable,
// falling back to looking for the command in the paths specified by the PATH
// environment variable if it is set. If the executable is available in the
// default path, then that path will be used. Otherwise if it can't find the
// command in the default path, the command will fall back to checking the
// command's PATH environment variable.
// kim: TODO: test in staging
// kim: TODO: update docs
func (c *subprocessExec) getExecutablePath(logger client.LoggerProducer) (absPath string, err error) {
	// kim; NOTE: explicitly not handling executing something in the current
	// working directory. For the agent, there shouldn't be anything in the
	// current working directory (supposedly)

	cmdPath := c.Env["PATH"]

	defaultPath, err := exec.LookPath(c.Binary)
	if defaultPath != "" {
		return defaultPath, nil
	}
	if len(cmdPath) == 0 || strings.Contains(c.Binary, string(filepath.Separator)) {
		return "", err
	}

	// Only look in the explicit command path if the default path didn't contain
	// a matching executable, the command path is set (either by add_to_path or
	// an explicit PATH environment variable), and the executable is not already
	// a file path (e.g. ./my-script.sh).

	originalPath := os.Getenv("PATH")
	defer func() {
		// Try to reset the PATH back to its original state. If this fails, then
		// the agent may have a modified PATH, which will affect all future
		// agent operations that need to execute processes. However, the
		// potential ways this could fail to reset (e.g. due to having
		// insufficient memory to set it) seem highly unlikely, so it doesn't
		// seem worth handling in a better way.
		if resetErr := os.Setenv("PATH", originalPath); resetErr != nil {
			logger.Execution().Error(errors.Wrap(resetErr, "resetting agent's PATH env var back to its original state").Error())
		}
	}()

	if err := os.Setenv("PATH", cmdPath); err != nil {
		return "", errors.Wrap(err, "setting PATH env var to try alternative executable paths")
	}

	altPath, err := exec.LookPath(c.Binary)
	if altPath != "" {
		return altPath, nil
	}

	return "", errors.Errorf("could not find executable in default execution PATH or in command's explicit PATH")
}

func (c *subprocessExec) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	var err error

	if err = c.doExpansions(conf.Expansions); err != nil {
		return errors.Wrap(err, "expanding command parameters")
	}

	logger.Execution().WarningWhen(
		filepath.IsAbs(c.WorkingDir) && !strings.HasPrefix(c.WorkingDir, conf.WorkDir),
		message.Fields{
			"message":         "the working directory is an absolute path without the required prefix",
			"path":            c.WorkingDir,
			"required_prefix": conf.WorkDir,
		})
	c.WorkingDir, err = conf.GetWorkingDirectory(c.WorkingDir)
	if err != nil {
		return errors.Wrap(err, "getting working directory")
	}

	taskTmpDir, err := conf.GetWorkingDirectory("tmp")
	if err != nil {
		logger.Execution().Notice(errors.Wrap(err, "getting temporary directory"))
	}

	c.Env = defaultAndApplyExpansionsToEnv(c.Env, modifyEnvOptions{
		taskID:                 conf.Task.Id,
		workingDir:             c.WorkingDir,
		tmpDir:                 taskTmpDir,
		expansions:             *conf.Expansions,
		includeExpansionsInEnv: c.IncludeExpansionsInEnv,
		addExpansionsToEnv:     c.AddExpansionsToEnv,
		addToPath:              c.Path,
	})

	if !c.KeepEmptyArgs {
		for i := len(c.Args) - 1; i >= 0; i-- {
			if c.Args[i] == "" {
				c.Args = append(c.Args[:i], c.Args[i+1:]...)
			}
		}
	}

	execPath, err := c.getExecutablePath(logger)
	if err != nil {
		return errors.Wrap(err, "resolving executable path")
	}

	logger.Execution().Debug(message.Fields{
		"working_directory": c.WorkingDir,
		"background":        c.Background,
		"binary":            execPath,
	})

	err = errors.WithStack(c.runCommand(ctx, conf.Task.Id, c.getProc(ctx, execPath, conf.Task.Id, logger), logger))

	if ctxErr := ctx.Err(); ctxErr != nil {
		logger.System().Debugf("Canceled command '%s', dumping running processes.", c.Name())
		logger.System().Debug(message.CollectAllProcesses())
		logger.Execution().Notice(err)

		return errors.Wrapf(ctxErr, "canceled while running command '%s'", c.Name())
	}

	return err
}

func (c *subprocessExec) runCommand(ctx context.Context, taskID string, cmd *jasper.Command, logger client.LoggerProducer) error {
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
