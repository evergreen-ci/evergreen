package command

import (
	"context"
	"os"
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
		return errors.Wrapf(err, "error decoding %s params", c.Name())
	}

	if c.Command != "" {
		if c.Binary != "" || len(c.Args) > 0 {
			return errors.New("must specify command as either arguments or a command string but not both")
		}

		args, err := shlex.Split(c.Command)
		if err != nil {
			return errors.Wrapf(err, "problem parsing %s command", c.Name())
		}
		if len(args) == 0 {
			return errors.Errorf("no arguments for command %s", c.Name())
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
		return errors.New("cannot ignore standard out, and redirect standard error to it")
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
	catcher.Add(err)

	c.Binary, err = exp.ExpandString(c.Binary)
	catcher.Add(err)

	for idx := range c.Args {
		c.Args[idx], err = exp.ExpandString(c.Args[idx])
		catcher.Add(err)
	}

	for k, v := range c.Env {
		c.Env[k], err = exp.ExpandString(v)
		catcher.Add(err)
	}

	if len(c.Path) > 0 {
		path := make([]string, len(c.Path), len(c.Path)+1)
		for idx := range c.Path {
			path[idx], err = exp.ExpandString(c.Path[idx])
			catcher.Add(err)
		}
		path = append(path, os.Getenv("PATH"))

		c.Env["PATH"] = strings.Join(path, string(filepath.ListSeparator))
	}

	return errors.Wrap(catcher.Resolve(), "problem expanding strings")
}

type modifyEnvOptions struct {
	taskID                 string
	workingDir             string
	tmpDir                 string
	expansions             util.Expansions
	includeExpansionsInEnv []string
	addExpansionsToEnv     bool
}

func defaultAndApplyExpansionsToEnv(env map[string]string, opts modifyEnvOptions) map[string]string {
	if env == nil {
		env = map[string]string{}
	}

	expansions := opts.expansions.Map()
	if opts.addExpansionsToEnv {
		for k, v := range expansions {
			if k == evergreen.GlobalGitHubTokenExpansion {
				//users should not be able to use the global github token expansion
				//as it can result in the breaching of Evergreen's GitHub API limit
				continue
			}
			env[k] = v
		}
	}

	for _, expName := range opts.includeExpansionsInEnv {
		if val, ok := expansions[expName]; ok && expName != evergreen.GlobalGitHubTokenExpansion {
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

func (c *subprocessExec) getProc(ctx context.Context, taskID string, logger client.LoggerProducer) *jasper.Command {
	cmd := c.JasperManager().CreateCommand(ctx).Add(append([]string{c.Binary}, c.Args...)).
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
				}), "problem registering cancellation for process"))
			}

			pid := proc.Info(ctx).PID

			agentutil.TrackProcess(taskID, pid, logger.System())

			if c.Background {
				logger.Execution().Debugf("running command in the background [pid=%d]", pid)
			} else {
				logger.Execution().Infof("started process with pid '%d'", pid)
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

func addTempDirs(env map[string]string, dir string) {
	for _, key := range []string{"TMP", "TMPDIR", "TEMP"} {
		if _, ok := env[key]; ok {
			continue
		}
		env[key] = dir
	}
}

func (c *subprocessExec) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	var err error

	if err = c.doExpansions(conf.Expansions); err != nil {
		logger.Execution().Error("problem expanding command values")
		return errors.WithStack(err)
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
		logger.Execution().Warning(err.Error())
		return errors.WithStack(err)
	}

	taskTmpDir, err := conf.GetWorkingDirectory("tmp")
	if err != nil {
		logger.Execution().Notice(err.Error())
	}

	var exp util.Expansions
	if conf.Expansions != nil {
		exp = *conf.Expansions
	}
	c.Env = defaultAndApplyExpansionsToEnv(c.Env, modifyEnvOptions{
		taskID:                 conf.Task.Id,
		workingDir:             c.WorkingDir,
		tmpDir:                 taskTmpDir,
		expansions:             exp,
		includeExpansionsInEnv: c.IncludeExpansionsInEnv,
		addExpansionsToEnv:     c.AddExpansionsToEnv,
	})

	if !c.KeepEmptyArgs {
		for i := len(c.Args) - 1; i >= 0; i-- {
			if c.Args[i] == "" {
				c.Args = append(c.Args[:i], c.Args[i+1:]...)
			}
		}
	}

	logger.Execution().Debug(message.Fields{
		"working_directory": c.WorkingDir,
		"background":        c.Background,
		"binary":            c.Binary,
	})

	err = errors.WithStack(c.runCommand(ctx, conf.Task.Id, c.getProc(ctx, conf.Task.Id, logger), logger))

	if ctx.Err() != nil {
		logger.System().Debug("dumping running processes")
		logger.System().Debug(message.CollectAllProcesses())
		logger.Execution().Notice(err)

		return errors.Errorf("%s aborted", c.Name())
	}

	return err
}

func (c *subprocessExec) runCommand(ctx context.Context, taskID string, cmd *jasper.Command, logger client.LoggerProducer) error {
	if c.Silent {
		logger.Execution().Info("executing command in silent mode")
	}

	err := cmd.Run(ctx)

	if c.ContinueOnError {
		logger.Execution().Notice(message.WrapError(err, message.Fields{
			"task":       taskID,
			"binary":     c.Binary,
			"background": c.Background,
			"silent":     c.Silent,
			"continue":   c.ContinueOnError,
		}))
		return nil
	}

	return err
}
