package agent

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

type BackgroundTestSuite struct {
	suite.Suite
	a                Agent
	mockCommunicator *client.Mock
	tc               *taskContext
}

func TestBackgroundTestSuite(t *testing.T) {
	suite.Run(t, new(BackgroundTestSuite))
}

func (s *BackgroundTestSuite) SetupTest() {
	s.a = Agent{
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
	s.tc.logger = s.a.comm.GetLoggerProducer(context.Background(), s.tc.task)
}

func (s *BackgroundTestSuite) TestWithCallbackTimeoutDefault() {
	ctx, _ := s.a.withCallbackTimeout(context.Background(), s.tc)
	deadline, ok := ctx.Deadline()
	s.True(deadline.Sub(time.Now()) > (defaultCallbackCmdTimeout - time.Second))
	s.True(ok)
}

func (s *BackgroundTestSuite) TestWithCallbackTimeoutSetByProject() {
	s.tc.taskConfig.Project.CallbackTimeout = 100
	ctx, _ := s.a.withCallbackTimeout(context.Background(), s.tc)
	deadline, ok := ctx.Deadline()
	s.True(deadline.Sub(time.Now()) > (99))
	s.True(ok)
}

func (s *BackgroundTestSuite) TestStartHeartbeat() {
	s.a.opts.HeartbeatInterval = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	heartbeat := make(chan string)
	s.a.startHeartbeat(ctx, s.tc, heartbeat)
	close(heartbeat)
	for _ = range heartbeat {
		// There should be no values in the channel
		s.True(false)
	}
}

func (s *BackgroundTestSuite) TestTaskAbort() {
	s.mockCommunicator.HeartbeatShouldAbort = true
	s.a.opts.HeartbeatInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	heartbeat := make(chan string)
	go s.a.startHeartbeat(ctx, s.tc, heartbeat)
	beat := <-heartbeat
	s.Equal(evergreen.TaskUndispatched, beat)
}

func (s *BackgroundTestSuite) TestMaxHeartbeats() {
	s.mockCommunicator.HeartbeatShouldErr = true
	s.a.opts.HeartbeatInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	heartbeat := make(chan string)
	go s.a.startHeartbeat(ctx, s.tc, heartbeat)
	beat := <-heartbeat
	s.Equal(evergreen.TaskFailed, beat)
}

func (s *BackgroundTestSuite) TestIdleTimeoutWatch() {
	s.a.opts.IdleTimeoutInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	idleTimeout := make(chan struct{})
	resetIdleTimeout := make(chan time.Duration)
	s.a.startIdleTimeoutWatch(ctx, s.tc, idleTimeout, resetIdleTimeout)
	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()
	s.Len(msgs, 1)
	for _, v := range msgs {
		s.Equal("Hit idle timeout", v[0].Message)
	}
}

func (s *BackgroundTestSuite) TestIdleTimeoutWatchMessageTimeout() {
	s.a.opts.IdleTimeoutInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	idleTimeout := make(chan struct{})
	resetIdleTimeout := make(chan time.Duration)
	s.a.comm.(*client.Mock).LastMessageSent = time.Now().Add(-time.Minute)
	s.a.startIdleTimeoutWatch(ctx, s.tc, idleTimeout, resetIdleTimeout)
	_, ok := <-idleTimeout
	s.False(ok)
	s.Panics(func() { close(idleTimeout) })
}

func (s *BackgroundTestSuite) TestIdleTimeoutWatchWithoutMessageTimeout() {
	s.a.opts.IdleTimeoutInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	idleTimeout := make(chan struct{})
	resetIdleTimeout := make(chan time.Duration)

	s.a.comm.(*client.Mock).LastMessageSent = time.Now().Add(time.Second)

	s.a.startIdleTimeoutWatch(ctx, s.tc, idleTimeout, resetIdleTimeout)
	s.NotPanics(func() { close(idleTimeout) })
	s.Panics(func() { close(idleTimeout) })
	_, ok := <-idleTimeout
	s.False(ok)
}

func (s *BackgroundTestSuite) TestExecTimeoutWatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	execTimeout := make(chan struct{})
	s.a.startMaxExecTimeoutWatch(ctx, s.tc, time.Millisecond, execTimeout)
	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()
	s.Len(msgs, 1)
	for _, v := range msgs {
		s.Equal("Hit exec timeout", v[0].Message)
	}
}
