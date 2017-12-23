package command

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
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
		WorkingDir:  "foo",
		CommandName: "bar",
		Args:        []string{"a", "b"},
	}

	s.NoError(cmd.doExpansions(s.conf.Expansions))
	s.Equal("foo", cmd.WorkingDir)
	s.Equal("bar", cmd.CommandName)
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
		WorkingDir:  "fo${o",
		CommandName: "ba${sfdf${bar}f}}r",
		Args:        []string{"${foo|a}", "${bar|b}"},
	}

	s.Error(cmd.doExpansions(s.conf.Expansions))
	s.Equal("fo${o", cmd.WorkingDir)
	s.Equal("baf}}r", cmd.CommandName)
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
	s.Zero(cmd.CommandName)
	s.Zero(cmd.Args)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.Len(cmd.Args, 2)
	s.NotZero(cmd.CommandName)
	s.Equal("/bin/bash", cmd.CommandName)
	s.Equal("-c", cmd.Args[0])
	s.Equal("foo bar", cmd.Args[1])
}
