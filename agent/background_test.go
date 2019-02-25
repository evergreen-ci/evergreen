package agent

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/stretchr/testify/suite"
)

type BackgroundSuite struct {
	suite.Suite
	a                *Agent
	mockCommunicator *client.Mock
	tc               *taskContext
}

func TestBackgroundSuite(t *testing.T) {
	suite.Run(t, new(BackgroundSuite))
}

func (s *BackgroundSuite) SetupTest() {
	var err error
	s.a = &Agent{
		opts: Options{
			HostID:     "host",
			HostSecret: "secret",
			StatusPort: 2286,
			LogPrefix:  evergreen.LocalLoggingOverride,
		},
		comm: client.NewMock("url"),
	}
	s.mockCommunicator = s.a.comm.(*client.Mock)

	s.tc = &taskContext{}
	s.tc.taskConfig = &model.TaskConfig{}
	s.tc.taskConfig.Project = &model.Project{}
	s.tc.taskConfig.Project.CallbackTimeout = 0
	s.tc.logger, err = s.a.comm.GetLoggerProducer(context.Background(), s.tc.task, nil)
	s.NoError(err)
}

func (s *BackgroundSuite) TestWithCallbackTimeoutDefault() {
	ctx, _ := s.a.withCallbackTimeout(context.Background(), s.tc)
	deadline, ok := ctx.Deadline()
	s.True(deadline.Sub(time.Now()) > (defaultCallbackCmdTimeout - time.Second)) // nolint
	s.True(ok)
}

func (s *BackgroundSuite) TestWithCallbackTimeoutSetByProject() {
	s.tc.taskConfig.Project.CallbackTimeout = 100
	ctx, _ := s.a.withCallbackTimeout(context.Background(), s.tc)
	deadline, ok := ctx.Deadline()
	s.True(deadline.Sub(time.Now()) > 99) // nolint
	s.True(ok)
}

func (s *BackgroundSuite) TestStartHeartbeat() {
	s.a.opts.HeartbeatInterval = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	heartbeat := make(chan string)
	go s.a.startHeartbeat(ctx, cancel, s.tc, heartbeat)
	s.Equal(evergreen.TaskFailed, <-heartbeat)
}

func (s *BackgroundSuite) TestTaskAbort() {
	s.mockCommunicator.HeartbeatShouldAbort = true
	s.a.opts.HeartbeatInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	heartbeat := make(chan string)
	go s.a.startHeartbeat(ctx, cancel, s.tc, heartbeat)
	beat := <-heartbeat
	s.Equal(evergreen.TaskFailed, beat)
}

func (s *BackgroundSuite) TestMaxHeartbeats() {
	s.mockCommunicator.HeartbeatShouldErr = true
	s.a.opts.HeartbeatInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	heartbeat := make(chan string)
	start := time.Now()
	go s.a.startHeartbeat(ctx, cancel, s.tc, heartbeat)
	beat := <-heartbeat
	end := time.Now()
	s.Equal(evergreen.TaskFailed, beat)
	s.True(end.Sub(start) < 50*time.Millisecond) // canceled before context expired
}

func (s *BackgroundSuite) TestHeartbeatSometimesFailsDoesNotFailTask() {
	s.mockCommunicator.HeartbeatShouldSometimesErr = true
	s.a.opts.HeartbeatInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	heartbeat := make(chan string)
	start := time.Now()
	go s.a.startHeartbeat(ctx, cancel, s.tc, heartbeat)
	beat := <-heartbeat
	end := time.Now()
	s.Equal(evergreen.TaskFailed, beat)
	s.True(end.Sub(start) > 49*time.Millisecond) // canceled by context
}

func (s *BackgroundSuite) TestGetCurrentTimeout() {
	s.tc.taskConfig.Timeout = &model.Timeout{}
	cmdFactory, exists := command.GetCommandFactory("shell.exec")
	s.True(exists)
	cmd := cmdFactory()
	cmd.SetIdleTimeout(time.Second)
	s.tc.setCurrentCommand(cmd)
	s.tc.setCurrentTimeout(cmd)
	s.Equal(time.Second, s.tc.getCurrentTimeout())
}

func (s *BackgroundSuite) TestGetTimeoutDefault() {
	s.Equal(defaultIdleTimeout, s.tc.getCurrentTimeout())
}
