package command

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/suite"
)

type execCmdSuite struct {
	cancel func()
	conf   *model.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer
	ctx    context.Context

	suite.Suite
}

func TestExecCmdSuite(t *testing.T) {
	suite.Run(t, new(execCmdSuite))
}

func (s *execCmdSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.comm = client.NewMock("http://localhost.com")
	s.conf = &model.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
	s.logger = s.comm.GetLoggerProducer(s.ctx, client.TaskData{ID: s.conf.Task.Id, Secret: s.conf.Task.Secret})
}

func (s *execCmdSuite) TearDownTest() {
	s.cancel()
}

func (s *execCmdSuite) TestNoopExpansion() {
	cmd := &simpleExec{
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

	cmd := &simpleExec{
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
	cmd := &simpleExec{
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
	cmd := &simpleExec{
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
	cmd := &simpleExec{}
	s.Nil(cmd.Env)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.NotNil(cmd.Env)
}

func (s *execCmdSuite) TestErrorToIgnoreAndRedirectToStdOut() {
	cmd := &simpleExec{}

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	cmd.IgnoreStandardOutput = true
	cmd.RedirectStandardErrorToOutput = true
	s.Error(cmd.ParseParams(map[string]interface{}{}))

	cmd = &simpleExec{}
	cmd.Silent = true
	cmd.RedirectStandardErrorToOutput = true
	s.Error(cmd.ParseParams(map[string]interface{}{}))
}

func (s *execCmdSuite) TestCommandParsing() {
	cmd := &simpleExec{
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
	cmd := &simpleExec{}
	s.Error(cmd.ParseParams(map[string]interface{}{"args": 1, "silent": "false"}))
	s.False(cmd.Background)
}

func (s *execCmdSuite) TestInvalidToSpecifyCommandInMultipleWays() {
	cmd := &simpleExec{
		Command: "/bin/bash -c 'echo foo'",
		Binary:  "bash",
		Args: []string{
			"-c",
			"echo foo",
		},
	}
	s.Error(cmd.ParseParams(map[string]interface{}{}))
}

func (s *execCmdSuite) TestGetProcErrorsIfCommandIsNotSet() {
	cmd := &simpleExec{}
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec, closer, err := cmd.getProc("foo", s.logger)
	s.Len(cmd.Env, 2)
	s.Error(err)
	s.Nil(exec)
	s.Nil(closer)
}

func (s *execCmdSuite) TestGetProcEnvSetting() {
	cmd := &simpleExec{
		Binary:    "bash",
		SystemLog: true,
	}
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec, closer, err := cmd.getProc("foo", s.logger)
	s.NoError(err)
	s.NotNil(exec)
	s.NotNil(closer)
	s.NotPanics(func() { closer() })
	s.Len(cmd.Env, 2)
}

func (s *execCmdSuite) TestRunCommand() {
	cmd := &simpleExec{
		Binary: "bash",
	}
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec, closer, err := cmd.getProc("foo", s.logger)
	s.NoError(err)
	s.NoError(cmd.runCommand(s.ctx, "foo", exec, s.logger))
	s.NotPanics(func() { closer() })
}

func (s *execCmdSuite) TestRunCommandPropgatesError() {
	cmd := &simpleExec{
		Command: "bash -c 'exit 1'",
	}
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec, closer, err := cmd.getProc("foo", s.logger)
	s.NoError(err)
	s.Error(cmd.runCommand(s.ctx, "foo", exec, s.logger))
	s.NotPanics(func() { closer() })
}

func (s *execCmdSuite) TestRunCommandContinueOnErrorNoError() {
	cmd := &simpleExec{
		Command:         "bash -c 'exit 1'",
		ContinueOnError: true,
	}
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec, closer, err := cmd.getProc("foo", s.logger)
	s.NoError(err)
	s.NoError(cmd.runCommand(s.ctx, "foo", exec, s.logger))
	s.NotPanics(func() { closer() })
}

func (s *execCmdSuite) TestRunCommandBackgroundAlwaysNil() {
	cmd := &simpleExec{
		Command:    "bash -c 'exit 1'",
		Background: true,
		Silent:     true,
	}
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec, closer, err := cmd.getProc("foo", s.logger)
	s.NoError(err)
	s.NoError(cmd.runCommand(s.ctx, "foo", exec, s.logger))
	s.NotPanics(func() { closer() })
}

func (s *execCmdSuite) TestCommandFailsWithoutWorkingDirectorySet() {
	// this is a situation that won't happen in production code,
	// but should happen logicaly, but means if you don't specify
	// a directory and there's not one configured on the distro,
	// then you're in trouble.
	cmd := &simpleExec{
		Command: "bash -c 'echo hello world!'",
	}

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.Error(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *execCmdSuite) TestCommandIntegrationSimple() {
	cmd := &simpleExec{
		Command:    "bash -c 'echo hello world!'",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.NoError(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *execCmdSuite) TestCommandIntegrationFailureExpansion() {
	cmd := &simpleExec{
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
	cmd := &simpleExec{
		// just set up enough so that we don't fail parse params
		Env:        map[string]string{},
		WorkingDir: testutil.GetDirectoryOfFile(),
	}

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.Error(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *execCmdSuite) TestExecuteErrorsIfCommandAborts() {
	cmd := &simpleExec{
		Command:    "bash -c 'echo hello world!'",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}

	s.cancel()

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	err := cmd.Execute(s.ctx, s.comm, s.logger, s.conf)
	if s.Error(err) {
		s.Contains(err.Error(), "aborted")
	}
}
