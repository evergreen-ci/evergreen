package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type execCmdSuite struct {
	cancel func()
	conf   *model.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer
	jasper jasper.Manager
	ctx    context.Context

	suite.Suite
}

func TestExecCmdSuite(t *testing.T) {
	suite.Run(t, new(execCmdSuite))
}

func (s *execCmdSuite) SetupSuite() {
	var err error
	s.jasper, err = jasper.NewSynchronizedManager(false)
	s.Require().NoError(err)
}

func (s *execCmdSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	var err error

	s.comm = client.NewMock("http://localhost.com")
	s.conf = &model.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
	s.logger, err = s.comm.GetLoggerProducer(s.ctx, client.TaskData{ID: s.conf.Task.Id, Secret: s.conf.Task.Secret}, nil)
	s.NoError(err)
}

func (s *execCmdSuite) TearDownTest() {
	s.cancel()
}

func (s *execCmdSuite) TestNoopExpansion() {
	cmd := &subprocessExec{
		WorkingDir: "foo",
		Binary:     "bar",
		Args:       []string{"a", "b"},
	}

	s.NoError(cmd.doExpansions(s.conf.Expansions))
	s.Equal("foo", cmd.WorkingDir)
	s.Equal("bar", cmd.Binary)
	s.Equal("a", cmd.Args[0])
	s.Equal("b", cmd.Args[1])
}

func (s *execCmdSuite) TestExpansionOfArgs() {

	cmd := &subprocessExec{
		Args: []string{
			"${foo|a}", "${foo|b}",
		},
	}

	s.NotEqual("a", cmd.Args[0])
	s.NotEqual("b", cmd.Args[1])

	s.NoError(cmd.doExpansions(s.conf.Expansions))
	s.Len(cmd.Args, 2)
	s.Equal("a", cmd.Args[0])
	s.Equal("b", cmd.Args[1])

}

func (s *execCmdSuite) TestExpansionOfEnvVarValues() {
	cmd := &subprocessExec{
		Env: map[string]string{
			"${foo|a}": "${foo|a}",
		},
	}

	s.NoError(cmd.doExpansions(s.conf.Expansions))
	v, ok := cmd.Env["${foo|a}"]
	s.True(ok)
	s.Equal("a", v)
}

func (s *execCmdSuite) TestWeirdAndBadExpansions() {
	cmd := &subprocessExec{
		WorkingDir: "fo${o",
		Binary:     "ba${sfdf${bar}f}}r",
		Args:       []string{"${foo|a}", "${bar|b}"},
	}

	s.Error(cmd.doExpansions(s.conf.Expansions))
	s.Equal("fo${o", cmd.WorkingDir)
	s.Equal("baf}}r", cmd.Binary)
	s.Equal("a", cmd.Args[0])
	s.Equal("b", cmd.Args[1])

}

func (s *execCmdSuite) TestParseParamsInitializesEnvMap() {
	cmd := &subprocessExec{}
	s.Nil(cmd.Env)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.NotNil(cmd.Env)
}

func (s *execCmdSuite) TestErrorToIgnoreAndRedirectToStdOut() {
	cmd := &subprocessExec{}

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	cmd.IgnoreStandardOutput = true
	cmd.RedirectStandardErrorToOutput = true
	s.Error(cmd.ParseParams(map[string]interface{}{}))

	cmd = &subprocessExec{}
	cmd.Silent = true
	cmd.RedirectStandardErrorToOutput = true
	s.Error(cmd.ParseParams(map[string]interface{}{}))
}

func (s *execCmdSuite) TestCommandParsing() {
	cmd := &subprocessExec{
		Command: "/bin/bash -c 'foo bar'",
	}
	s.Zero(cmd.Binary)
	s.Zero(cmd.Args)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.Len(cmd.Args, 2)
	s.NotZero(cmd.Binary)
	s.Equal("/bin/bash", cmd.Binary)
	s.Equal("-c", cmd.Args[0])
	s.Equal("foo bar", cmd.Args[1])
}

func (s *execCmdSuite) TestParseErrorIfTypeMismatch() {
	cmd := &subprocessExec{}
	s.Error(cmd.ParseParams(map[string]interface{}{"args": 1, "silent": "false"}))
	s.False(cmd.Background)
}

func (s *execCmdSuite) TestInvalidToSpecifyCommandInMultipleWays() {
	cmd := &subprocessExec{
		Command: "/bin/bash -c 'echo foo'",
		Binary:  "bash",
		Args: []string{
			"-c",
			"echo foo",
		},
	}
	s.Error(cmd.ParseParams(map[string]interface{}{}))
}

func (s *execCmdSuite) TestGetProcEnvSetting() {
	cmd := &subprocessExec{
		Binary:    "bash",
		SystemLog: true,
	}
	cmd.SetJasperManager(s.jasper)

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec := cmd.getProc(s.ctx, "foo", s.logger)
	s.Len(cmd.Env, 3)
	s.NotNil(exec)
}

func (s *execCmdSuite) TestRunCommand() {
	cmd := &subprocessExec{
		Binary: "bash",
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec := cmd.getProc(s.ctx, "foo", s.logger)
	s.NoError(cmd.runCommand(s.ctx, "foo", exec, s.logger))
}

func (s *execCmdSuite) TestRunCommandPropgatesError() {
	cmd := &subprocessExec{
		Command: "bash -c 'exit 1'",
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec := cmd.getProc(s.ctx, "foo", s.logger)
	s.Error(cmd.runCommand(s.ctx, "foo", exec, s.logger))
}

func (s *execCmdSuite) TestRunCommandContinueOnErrorNoError() {
	cmd := &subprocessExec{
		Command:         "bash -c 'exit 1'",
		ContinueOnError: true,
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec := cmd.getProc(s.ctx, "foo", s.logger)
	s.NoError(cmd.runCommand(s.ctx, "foo", exec, s.logger))
}

func (s *execCmdSuite) TestRunCommandBackgroundAlwaysNil() {
	cmd := &subprocessExec{
		Command:    "bash -c 'exit 1'",
		Background: true,
		Silent:     true,
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec := cmd.getProc(s.ctx, "foo", s.logger)
	s.NoError(cmd.runCommand(s.ctx, "foo", exec, s.logger))
}

func (s *execCmdSuite) TestCommandFailsWithoutWorkingDirectorySet() {
	// this is a situation that won't happen in production code,
	// but should happen logicaly, but means if you don't specify
	// a directory and there's not one configured on the distro,
	// then you're in trouble.
	cmd := &subprocessExec{
		Command: "bash -c 'echo hello world!'",
	}

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.Error(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *execCmdSuite) TestCommandIntegrationSimple() {
	cmd := &subprocessExec{
		Command:    "bash -c 'echo hello world!'",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	cmd.SetJasperManager(s.jasper)

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.NoError(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *execCmdSuite) TestCommandIntegrationFailureExpansion() {
	cmd := &subprocessExec{
		Command:    "bash -c 'echo hello wor${ld!'",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	err := cmd.Execute(s.ctx, s.comm, s.logger, s.conf)
	if s.Error(err) {
		s.Contains(err.Error(), "problem expanding")
	}
}

func (s *execCmdSuite) TestCommandIntegrationFailureCase() {
	cmd := &subprocessExec{
		// just set up enough so that we don't fail parse params
		Env:        map[string]string{},
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.Error(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *execCmdSuite) TestExecuteErrorsIfCommandAborts() {
	cmd := &subprocessExec{
		Command:    "bash -c 'echo hello world!'",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	cmd.SetJasperManager(s.jasper)

	s.cancel()

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	err := cmd.Execute(s.ctx, s.comm, s.logger, s.conf)
	if s.Error(err) {
		s.Contains(err.Error(), "aborted")
	}
}

func (s *execCmdSuite) TestKeepEmptyArgs() {
	// by default empty args should be stripped
	cmd := &subprocessExec{
		Command:    "echo ${foo|} bar",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.NoError(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Len(cmd.Args, 1)

	// empty args should not be stripped if set
	cmd = &subprocessExec{
		Command:       "echo ${foo|} bar",
		WorkingDir:    testutil.GetDirectoryOfFile(),
		KeepEmptyArgs: true,
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.NoError(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Len(cmd.Args, 2)
}

func (s *execCmdSuite) TestPathSetting() {
	cmd := &subprocessExec{
		// just set up enough so that we don't fail parse params
		Env:        map[string]string{},
		WorkingDir: testutil.GetDirectoryOfFile(),
		Path:       []string{"foo", "bar"},
	}
	exp := util.NewExpansions(map[string]string{})
	s.Len(cmd.Env, 0)
	s.NoError(cmd.doExpansions(exp))
	s.Len(cmd.Env, 1)

	path, ok := cmd.Env["PATH"]
	s.True(ok)
	s.Len(filepath.SplitList(path), len(filepath.SplitList(os.Getenv("PATH")))+2)
}

func (s *execCmdSuite) TestNoPathSetting() {
	cmd := &subprocessExec{
		// just set up enough so that we don't fail parse params
		Env:        map[string]string{},
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	exp := util.NewExpansions(map[string]string{})
	s.Len(cmd.Env, 0)
	s.NoError(cmd.doExpansions(exp))
	s.Len(cmd.Env, 0)

	path, ok := cmd.Env["PATH"]
	s.False(ok)
	s.Zero(path)
}

func (s *execCmdSuite) TestExpansionsEnvOptionDisabled() {
	cmd := &subprocessExec{
		Env:        map[string]string{},
		WorkingDir: testutil.GetDirectoryOfFile(),
	}

	s.NoError(cmd.doExpansions(util.NewExpansions(map[string]string{})))
	s.Len(cmd.Env, 0)
	cmd.Env["one"] = "one"
	s.NoError(cmd.doExpansions(util.NewExpansions(map[string]string{"two": "two"})))
	s.Len(cmd.Env, 1)
	s.NotEqual("two", cmd.Env["two"])
	s.Equal("one", cmd.Env["one"])
}

func (s *execCmdSuite) TestExpansionsEnvOptionEnabled() {
	cmd := &subprocessExec{
		Env:                map[string]string{},
		WorkingDir:         testutil.GetDirectoryOfFile(),
		AddExpansionsToEnv: true,
	}

	s.NoError(cmd.doExpansions(util.NewExpansions(map[string]string{})))
	s.Len(cmd.Env, 0)
	cmd.Env["one"] = "one"
	s.Equal("one", cmd.Env["one"])
	s.NoError(cmd.doExpansions(util.NewExpansions(map[string]string{"two": "two", "one": "1"})))
	s.Len(cmd.Env, 2)
	s.Equal("two", cmd.Env["two"])
	s.Equal("1", cmd.Env["one"])
}

func TestAddTemp(t *testing.T) {
	for name, test := range map[string]func(*testing.T, map[string]string){
		"Empty": func(t *testing.T, env map[string]string) {
			addTempDirs(env, "")
			assert.Len(t, env, 3)
		},
		"WithAnExistingOther": func(t *testing.T, env map[string]string) {
			env["foo"] = "one"
			addTempDirs(env, "")
			assert.Len(t, env, 4)
		},
		"WithExistingTmp": func(t *testing.T, env map[string]string) {
			env["TMP"] = "foo"
			addTempDirs(env, "bar")
			assert.Len(t, env, 3)
			assert.Equal(t, "foo", env["TMP"])
			assert.Equal(t, "bar", env["TMPDIR"])
		},
		"CorrectKeys": func(t *testing.T, env map[string]string) {
			addTempDirs(env, "bar")
			assert.Len(t, env, 3)
			assert.Equal(t, "bar", env["TMP"])
			assert.Equal(t, "bar", env["TEMP"])
			assert.Equal(t, "bar", env["TMPDIR"])
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := make(map[string]string)
			require.Len(t, env, 0)
			test(t, env)
		})
	}
}
