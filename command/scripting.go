package command

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/shlex"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/greenbay/output"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/scripting"
	"github.com/pkg/errors"
)

type scriptingExec struct {
	Harness string            `mapstructure:"harness"`
	Script  string            `mapstructure:"script"`
	Command string            `mapstructure:"command"`
	Args    []string          `mapstructure:"args"`
	Path    []string          `mapstructure:"add_to_path"`
	Env     map[string]string `mapstructure:"env"`

	CacheDurationSeconds int    `mapstructure:"cache_duration_secs"`
	CleanupHarness       bool   `mapstructure:"cleanup_harness"`
	LockFile             string `mapstructure:"lock_file"`
	Packages             string `mapstructure:"packages"`
	// HarnessPath should be the
	HarnessPath string `mapstructure:"harness_path"`
	// Host is the path to the hosting interpreter, where
	// appropriate. This should be the path to the python
	// interpreter or go binary.
	Host string `mapstructure:"host"`

	// Add defined expansions to the environment of the process
	// that's launched.
	AddExpansionsToEnv bool `mapstructure:"add_expansions_to_env"`

	// IncludeExpansionsInEnv allows users to specify a number of
	// expansions that will be included in the environment, if
	// they are defined. It is not an error to specify expansions
	// that are not defined in include_expansions_in_env.
	IncludeExpansionsInEnv []string `mapstructure:"include_expansions_in_env"`

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

	base
}

func subprocessScriptingFactory() Command {
	return &scriptingExec{}
}

func (c *scriptingExec) Name() string { return "subprocess.scripting" }
func (c *scriptingExec) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrapf(err, "error decoding %s params", c.Name())
	}

	if c.Command != "" {
		if c.Script != "" || len(c.Args) > 0 {
			return errors.New("must specify command as either arguments or a command string but not both")
		}

		c.Args, err = shlex.Split(c.Command)
		if err != nil {
			return errors.Wrapf(err, "problem parsing %s command", c.Name())
		}
	}

	if c.Script != "" && len(c.Args) > 0 {
		return errors.New("must specify either a script or a command, but not both")
	}

	if c.CacheDurationSeconds < 1 {
		c.CacheDurationSeconds = 10
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

func (c *scriptingExec) doExpansions(exp *util.Expansions) error {
	var err error
	catcher := grip.NewBasicCatcher()

	c.Harness, err = exp.ExpandString(c.Harness)
	catcher.Add(err)

	c.Script, err = exp.ExpandString(c.Script)
	catcher.Add(err)

	c.WorkingDir, err = exp.ExpandString(c.WorkingDir)
	catcher.Add(err)

	c.LockFile, err = exp.ExpandString(c.LockFile)
	catcher.Add(err)

	c.Packages, err = exp.ExpandString(c.Packages)
	catcher.Add(err)

	c.HarnessPath, err = exp.ExpandString(c.HarnessPath)
	catcher.Add(err)

	for idx := range c.Packages {
		c.Packages[idx], err = exp.ExpandString(c.Packages[idx])
		catcher.Add(err)
	}

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

	expansions := exp.Map()
	if c.AddExpansionsToEnv {
		for k, v := range expansions {
			c.Env[k] = v
		}
	}

	for _, ei := range c.IncludeExpansionsInEnv {
		if val, ok := expansions[ei]; ok {
			c.Env[ei] = val
		}
	}

	return errors.Wrap(catcher.Resolve(), "problem expanding strings")
}

func (c *scriptingExec) getOutput(logger client.LoggerProducer) (*output.Options, func() error) {
	closers := []func() error{}

	output := &options.Output{
		SuppressError:     c.IgnoreStandardError,
		SuppressOutput:    c.IgnoreStandardOutput,
		SendErrorToOutput: c.RedirectStandardErrorToOutput,
	}

	if !c.IgnoreStandardOutput {
		var owc io.WriterCloser
		if c.SystemLog {
			owc = send.MakeWriterSender(level.Info, logger.System().GetSender())
		} else {
			owc = send.MakeWriterSender(level.Info, logger.Task().GetSender())
		}
		closers = append(closers, owc.Close)
		output.Output = owc
	}

	if !c.IgnoreStandardError {
		var owc io.WriterCloser
		if c.SystemLog {
			owc = send.MakeWriterSender(level.Error, logger.System().GetSender())
		} else {
			owc = send.MakeWriterSender(level.Error, logger.Task().GetSender())
		}
		closers = append(closers, owc.Close)
		output.Error = owc
	}

	return output, func() error {
		catcher := grip.NewBasicCatcher()
		for idx := range closers {
			catcher.Add(closers[idx]())
		}
		return catcher.Resolve()
	}
}

func (c *scriptingExec) getHarnessConfig(output *options.Output) (options.ScriptingHarness, error) {
	switch c.Harness {
	case "python3", "python":
		return &options.ScriptingPython{
			Output:                output,
			Environment:           c.Env,
			CachedDuration:        c.CacheDurationSeconds * time.Second,
			Packages:              c.Packages,
			VirtualEnvPath:        filepath.Join(c.WorkingDir, c.HarnessPath),
			HostPythonInterpreter: c.HostPath,
		}, nil
	case "python2":
		return &options.ScriptingPython{
			Output:                output,
			LegacyPython:          true,
			Environment:           c.Env,
			CachedDuration:        c.CacheDurationSeconds * time.Second,
			Packages:              c.Packages,
			VirtualEnvPath:        filepath.Join(c.WorkingDir, c.HarnessPath),
			HostPythonInterpreter: c.HostPath,
		}, nil
	case "roswell", "lisp":
		return &options.ScriptingRoswell{
			Output:         output,
			Environment:    c.Env,
			CachedDuration: c.CacheDurationSeconds * time.Second,
			Systems:        c.Packages,
			Path:           filepath.Join(c.WorkingDir, c.HarnessPath),
			Lisp:           c.HostPath,
		}, nil
	case "golang", "go":
		return &options.ScriptingGolang{
			Output:         output,
			Environment:    c.Env,
			Systems:        c.Packages,
			CachedDuration: c.CacheDurationSeconds * time.Second,
			Packages:       c.Packages,
			Gopath:         filepath.Join(c.WorkingDir, c.HarnessPath),
			Context:        c.WorkingDir,
			Goroot:         c.HostPath,
		}, nil
	default:
		return nil, errors.Errorf("there is no support for harness: '%s'", c.Harness)
	}

}

func (c *scriptingExec) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	var err error

	if err = c.doExpansions(conf.Expansions); err != nil {
		logger.Execution().Error("problem expanding command values")
		return errors.WithStack(err)
	}

	logger.Execution().WarningWhenf(filepath.IsAbs(c.WorkingDir) && !strings.HasPrefix(c.WorkingDir, conf.WorkDir),
		"the working directory is an absolute path [%s], which isn't supported except when prefixed by '%s'",
		c.WorkingDir, conf.WorkDir)

	c.WorkingDir, err = conf.GetWorkingDirectory(c.WorkingDir)
	if err != nil {
		logger.Execution().Warning(err.Error())
		return errors.WithStack(err)
	}

	taskTmpDir, err := conf.GetWorkingDirectory("tmp")
	if err != nil {
		logger.Execution().Notice(err.Error())
	}

	addTempDirs(c.Env, taskTmpDir)

	output, closer := c.getOutput(logger)
	opts, err := c.getHarnessConfig(output)
	if err != nil {
		return errors.WithStack(err)
	}

	harness, err := scripting.NewHarness(c.JasperManager(), opts)
	if err != nil {
		return errors.WithStack(err)
	}

	catcher := grip.NewBasicCatcher()
	if len(c.Args) > 0 {
		catcher.Add(harness.Run(ctx, c.Args))
	}
	if c.Script != "" {
		catcher.Add(harness.RunScript(ctx, c.Script))
	}
	catcher.Add(closer())
	if c.CleanupHarness {
		catcher.Add(harness.Cleanup(ctx))
	}

	if c.ContinueOnError {
		logger.Execution().Notice(message.WrapError(catcher.Resolve(), message.Fields{
			"task":     conf.Task.Id,
			"harness":  c.Harness,
			"silent":   c.Silent,
			"continue": c.ContinueOnError,
		}))
		return nil
	}

	return catcher.Resolve()
}
