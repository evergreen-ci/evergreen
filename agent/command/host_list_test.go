package command

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/suite"
)

type HostListSuite struct {
	cancel func()
	conf   *internal.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer
	ctx    context.Context

	cmd *listHosts
	suite.Suite
}

func TestHostListSuite(t *testing.T) {
	suite.Run(t, new(HostListSuite))
}

func (s *HostListSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	var err error

	s.comm = client.NewMock("http://localhost.com")
	s.conf = &internal.TaskConfig{Expansions: util.Expansions{"foo": "3"}, Task: task.Task{}, Project: model.Project{}}
	s.logger, err = s.comm.GetLoggerProducer(s.ctx, client.TaskData{ID: s.conf.Task.Id, Secret: s.conf.Task.Secret}, nil)
	s.NoError(err)
	s.cmd = listHostFactory().(*listHosts)
}

func (s *HostListSuite) TearDownTest() {
	s.cancel()
}

func (s *HostListSuite) TestCommandIsProperlyConstructed() {
	s.NotNil(s.cmd)
	s.Equal("host.list", s.cmd.Name())
	f, ok := GetCommandFactory("host.list")
	s.True(ok)
	s.Equal(f(), s.cmd)
}

func (s *HostListSuite) TestEarlyValidationAvoidsBlockingForNothing() {
	s.NoError(s.cmd.ParseParams(nil))
	s.cmd.Wait = true
	s.cmd.NumHosts = "0"
	s.Error(s.cmd.ParseParams(nil))
	s.cmd.NumHosts = "1"
	s.NoError(s.cmd.ParseParams(nil))
}

func (s *HostListSuite) TestEarlyValidationWithoutOutputIsAnError() {
	s.NoError(s.cmd.ParseParams(nil))
	s.cmd.Path = ""
	s.cmd.Silent = true
	s.cmd.Wait = false
	s.Error(s.cmd.ParseParams(nil))
	s.cmd.Path = "foo"
	s.NoError(s.cmd.ParseParams(nil))
}

func (s *HostListSuite) TestEarlyValidationTimeNonsense() {
	s.NoError(s.cmd.ParseParams(nil))
	s.cmd.TimeoutSecs = -1
	s.Error(s.cmd.ParseParams(nil))
	s.cmd.TimeoutSecs = 4
	s.NoError(s.cmd.ParseParams(nil))
}

func (s *HostListSuite) TestMockExecuteUnconfigured() {
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *HostListSuite) TestMockExecuteWithWait() {
	s.cmd.Wait = true
	s.cmd.NumHosts = "1"
	s.cmd.TimeoutSecs = 1
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *HostListSuite) TestExpansions() {
	s.NoError(s.cmd.ParseParams(
		map[string]interface{}{
			"num_hosts": 2,
		},
	))
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Equal("2", s.cmd.NumHosts)

	s.NoError(s.cmd.ParseParams(
		map[string]interface{}{
			"num_hosts": "${foo}",
		},
	))
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Equal("3", s.cmd.NumHosts)
}
