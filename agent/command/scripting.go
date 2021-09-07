package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/google/shlex"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/scripting"
	"github.com/pkg/errors"
)

type scriptingExec struct {
	// Harness declares the implementation of the scripting
	// harness to use to execute this code.
	Harness string `mapstructure:"harness"`

	////////////////////////////////
	//
	// Execution Options

	// Specify the command to run as a string that Evergreen will
	// split into an argument array. This, as with subprocess.exec,
	// is split using shell parsing rules.
	Command string `mapstructure:"command"`
	// Specify the command to run as a list of arguments.
	Args []string `mapstructure:"args"`

	// Specify the content of a script to execute in the
	// environment. This probably only makes sense for roswell,
	// but is here for completeness, and won't be documented.
	Script string `mapstructure:"script"`

	// TestDir specifies the directory containing the tests that should be run.
	// This should be a subdirectory of the working directory.
	TestDir string `mapstructure:"test_dir"`
	// TestOptions specifies additional options that determine how tests should
	// be executed.
	TestOptions *scriptingTestOptions `mapstructure:"test_options"`

	// Specify a list of directories to add to the PATH of the
	// environment.
	Path []string `mapstructure:"add_to_path"`
	// Specific environment variables to be added to the
	// environment of the scripting commands executed
	Env map[string]string `mapstructure:"env"`

	////////////////////////////////
	//
	// Harness Options

	// CacheDurationSeconds describes the total number of seconds
	// that an environment should be stored for.
	CacheDurationSeconds int `mapstructure:"cache_duration_secs"`
	// CleanupHarness forces the command to cleanup the harness
	// after the command returns. When this is false, the harness
	// will persist between commands. These harnesses are within
	// the working directory and so cleaned up between tasks or
	// task groups regardless.
	CleanupHarness bool `mapstructure:"cleanup_harness"`
	// LockFile describes the path to the dependency file
	// (e.g. requirements.txt if it exists) that lists your
	// dependencies. Not all environments support Lockfiles.
	LockFile string `mapstructure:"lock_file"`
	// Packages are a list of dependencies that will be installed
	// in your environment.
	Packages []string `mapstructure:"packages"`
	// HarnessPath should be the path to your local environment
	// (e.g. GOPATH or VirtualEnv.) Specify a subpath of the
	// working directory.
	HarnessPath string `mapstructure:"harness_path"`
	// HostPath is the path to the hosting interpreter or binary, where
	// appropriate. This should be the path to the interpreter for python or
	// GOROOT for golang.
	HostPath string `mapstructure:"host_path"`

	////////////////////////////////
	//
	// Execution Options

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

	// Report indicates whether or not the test results should be reported (if
	// supported).
	Report bool `mapstructure:"report"`

	// WorkingDir is the working directory to start the shell in.
	WorkingDir string `mapstructure:"working_dir"`

	// IgnoreStandardOutput and IgnoreStandardError allow users to
	// elect to ignore either standard error and/or standard output.
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

type scriptingTestOptions struct {
	Name        string   `bson:"name" json:"name" yaml:"name"`
	Args        []string `bson:"args" json:"args" yaml:"args"`
	Pattern     string   `bson:"pattern" json:"pattern" yaml:"pattern"`
	TimeoutSecs int      `bson:"timeout_secs" json:"timeout_secs" yaml:"timeout_secs"`
	Count       int      `bson:"count" json:"count" yaml:"count"`
}

func (opts *scriptingTestOptions) validate() error {
	if opts.TimeoutSecs < 0 {
		return errors.New("cannot specify negative timeout seconds")
	}
	if opts.Count < 0 {
		return errors.New("cannot specify negative run count")
	}
	return nil
}

func subprocessScriptingFactory() Command {
	return &scriptingExec{}
}

func validTestingHarnesses() []string {
	return []string{
		"go", "golang",
		"lisp", "roswell",
		"python", "python3", "python2",
	}
}

func (c *scriptingExec) Name() string { return "subprocess.scripting" }
func (c *scriptingExec) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrapf(err, "error decoding %s params", c.Name())
	}

	if !utility.StringSliceContains(validTestingHarnesses(), c.Harness) {
		return errors.Errorf("invalid testing harness '%s': valid options are %s", c.Harness, strings.Join(validTestingHarnesses(), ", "))
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

	if c.TestDir != "" && (c.Script != "" || len(c.Args) != 0) {
		return errors.New("cannot specify both test directory and a script or command to run")
	}
	if c.Script == "" && len(c.Args) == 0 && c.TestDir == "" {
		return errors.New("must specify either a script, a command, or a test directory")
	}
	if c.Script != "" && len(c.Args) > 0 {
		return errors.New("cannot specify both a script and a command")
	}
	if c.Script != "" && c.TestDir != "" {
		return errors.New("cannot specify both a script and a test directory")
	}
	if len(c.Args) > 0 && c.TestDir != "" {
		return errors.New("cannot specify both a command and a test directory")
	}
	if c.TestOptions != nil {
		if err := c.TestOptions.validate(); err != nil {
			return errors.Wrap(err, "invalid test options")
		}
	}

	if c.CacheDurationSeconds < 1 {
		c.CacheDurationSeconds = 900
	}

	if c.Silent {
		c.IgnoreStandardError = true
		c.IgnoreStandardOutput = true
	}

	if c.IgnoreStandardOutput && c.Report {
		return errors.New("cannot ignore standard output and also parse test results")
	}

	if c.Report && c.Harness != "go" && c.Harness != "golang" {
		return errors.Errorf("reporting test results is not supported for harness '%s'", c.Harness)
	}

	if (c.Harness == "go" || c.Harness == "golang") && c.HostPath == "" && c.Env["GOROOT"] == "" {
		return errors.Errorf("path to GOROOT is required for golang")
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

	c.HarnessPath, err = exp.ExpandString(c.HarnessPath)
	catcher.Add(err)

	c.HostPath, err = exp.ExpandString(c.HostPath)
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
			if k == evergreen.GlobalGitHubTokenExpansion {
				//users should not be able to use the global github token expansion
				//as it can result in the breaching of Evergreen's GitHub API limit
				continue
			}
			c.Env[k] = v
		}
	}

	for _, ei := range c.IncludeExpansionsInEnv {
		if val, ok := expansions[ei]; ok && ei != evergreen.GlobalGitHubTokenExpansion {
			c.Env[ei] = val
		}
	}

	return errors.Wrap(catcher.Resolve(), "problem expanding strings")
}

func (c *scriptingExec) getOutputWithWriter(w io.Writer, logger client.LoggerProducer) (options.Output, []grip.CheckFunction) {
	output := options.Output{
		SuppressError:     c.IgnoreStandardError,
		SuppressOutput:    c.IgnoreStandardOutput,
		SendErrorToOutput: c.RedirectStandardErrorToOutput,
	}

	ww := send.WrapWriter(w)

	var logSender send.Sender
	var closers []grip.CheckFunction
	if c.SystemLog {
		logSender = logger.System().GetSender()
	} else {
		logSender = logger.Task().GetSender()
	}

	multi := send.NewConfiguredMultiSender(logSender, ww)

	if !c.IgnoreStandardOutput {
		ws := send.MakeWriterSender(multi, level.Info)
		closers = append(closers, ws.Close)
		output.Output = ws
	}

	if !c.IgnoreStandardError {
		ws := send.MakeWriterSender(multi, level.Error)
		closers = append(closers, ws.Close)
		output.Error = ws
	}

	return output, closers
}

func (c *scriptingExec) getOutput(logger client.LoggerProducer) (options.Output, []grip.CheckFunction) {
	closers := []grip.CheckFunction{}

	output := options.Output{
		SuppressError:     c.IgnoreStandardError,
		SuppressOutput:    c.IgnoreStandardOutput,
		SendErrorToOutput: c.RedirectStandardErrorToOutput,
	}

	if !c.IgnoreStandardOutput {
		var owc io.WriteCloser
		if c.SystemLog {
			owc = send.MakeWriterSender(logger.System().GetSender(), level.Info)
		} else {
			owc = send.MakeWriterSender(logger.Task().GetSender(), level.Info)
		}
		closers = append(closers, owc.Close)
		output.Output = owc
	}

	if !c.IgnoreStandardError {
		var owc io.WriteCloser
		if c.SystemLog {
			owc = send.MakeWriterSender(logger.System().GetSender(), level.Error)
		} else {
			owc = send.MakeWriterSender(logger.Task().GetSender(), level.Error)
		}
		closers = append(closers, owc.Close)
		output.Error = owc
	}

	return output, closers
}

func (c *scriptingExec) getHarnessConfig(output options.Output) (options.ScriptingHarness, error) {
	switch c.Harness {
	case "python3", "python":
		return &options.ScriptingPython{
			Output:            output,
			Environment:       c.Env,
			CachedDuration:    time.Duration(c.CacheDurationSeconds) * time.Second,
			Packages:          c.Packages,
			VirtualEnvPath:    filepath.Join(c.WorkingDir, c.HarnessPath),
			InterpreterBinary: c.HostPath,
		}, nil
	case "python2":
		return &options.ScriptingPython{
			Output:            output,
			LegacyPython:      true,
			Environment:       c.Env,
			CachedDuration:    time.Duration(c.CacheDurationSeconds) * time.Second,
			Packages:          c.Packages,
			VirtualEnvPath:    filepath.Join(c.WorkingDir, c.HarnessPath),
			InterpreterBinary: c.HostPath,
		}, nil
	case "roswell", "lisp":
		return &options.ScriptingRoswell{
			Output:         output,
			Environment:    c.Env,
			CachedDuration: time.Duration(c.CacheDurationSeconds) * time.Second,
			Systems:        c.Packages,
			Path:           filepath.Join(c.WorkingDir, c.HarnessPath),
			Lisp:           c.HostPath,
		}, nil
	case "golang", "go":
		goroot := c.Env["GOROOT"]
		if goroot == "" {
			goroot = c.HostPath
		}
		gopath := c.Env["GOPATH"]
		if gopath == "" {
			gopath = filepath.Join(c.WorkingDir, c.HarnessPath)
		}
		return &options.ScriptingGolang{
			Output:         output,
			Environment:    c.Env,
			Packages:       c.Packages,
			CachedDuration: time.Duration(c.CacheDurationSeconds) * time.Second,
			Gopath:         gopath,
			Directory:      c.WorkingDir,
			Goroot:         goroot,
		}, nil
	default:
		return nil, errors.Errorf("there is no support for harness: '%s'", c.Harness)
	}

}

func (c *scriptingExec) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	var err error

	if err = c.doExpansions(conf.Expansions); err != nil {
		logger.Execution().Error("problem expanding command values")
		return errors.WithStack(err)
	}

	logger.Execution().WarningWhen(filepath.IsAbs(c.WorkingDir) && !strings.HasPrefix(c.WorkingDir, conf.WorkDir),
		fmt.Sprintf("the working directory is an absolute path [%s], which isn't supported except when prefixed by '%s'",
			c.WorkingDir, conf.WorkDir))

	if c.WorkingDir == "" {
		c.WorkingDir, err = conf.GetWorkingDirectory(c.WorkingDir)
		if err != nil {
			logger.Execution().Warning(err.Error())
			return errors.WithStack(err)
		}
	}

	taskTmpDir, err := conf.GetWorkingDirectory("tmp")
	if err != nil {
		logger.Execution().Notice(err.Error())
	}

	addTempDirs(c.Env, taskTmpDir)

	var output options.Output
	var closers []grip.CheckFunction
	report := &bytes.Buffer{}
	if c.Report {
		output, closers = c.getOutputWithWriter(report, logger)
	} else {
		output, closers = c.getOutput(logger)
	}
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
	if c.TestDir != "" {
		var opts scripting.TestOptions
		if c.TestOptions != nil {
			opts = scripting.TestOptions{
				Name:    c.TestOptions.Name,
				Args:    c.TestOptions.Args,
				Pattern: c.TestOptions.Pattern,
				Timeout: time.Duration(c.TestOptions.TimeoutSecs) * time.Second,
				Count:   c.TestOptions.Count,
			}
		}
		results, err := harness.Test(ctx, filepath.Join(c.WorkingDir, c.TestDir), opts)
		catcher.Add(err)
		for _, res := range results {
			logger.Task().Info(message.Fields{
				"name":     res.Name,
				"outcome":  res.Outcome,
				"duration": res.Duration,
			})
		}
	}

	if c.Report {
		if err := c.reportTestResults(ctx, comm, logger, conf, report); err != nil {
			return errors.Wrap(err, "reporting test results")
		}
	}

	catcher.CheckExtend(closers)
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

func (c *scriptingExec) reportTestResults(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, report io.Reader) error {
	switch c.Harness {
	case "go", "golang":
		// A suite name is required in the REST request or else it hangs, but it
		// seems like the particular suite name is unimportant.
		log, result, err := parseTestOutput(ctx, conf, report, "output.test")
		if err != nil {
			return errors.Wrap(err, "parsing test output")
		}

		logs := []model.TestLog{log}
		results := [][]task.TestResult{result}

		if err := sendTestLogsAndResults(ctx, comm, logger, conf, logs, results); err != nil {
			return errors.Wrap(err, "sending test logs and test results")
		}

		return nil
	default:
		return errors.Errorf("cannot report results for harness '%s'", c.Harness)
	}
}
