package agent

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
)

type BackgroundSuite struct {
	suite.Suite
	a                *Agent
	mockCommunicator *client.Mock
	tc               *taskContext
	sender           *send.InternalSender
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
		comm:   client.NewMock("url"),
		tracer: otel.GetTracerProvider().Tracer("noop_tracer"),
	}
	s.a.jasper, err = jasper.NewSynchronizedManager(true)
	s.Require().NoError(err)
	s.mockCommunicator = s.a.comm.(*client.Mock)

	s.tc = &taskContext{}
	s.tc.taskConfig = &internal.TaskConfig{}
	s.tc.taskConfig.Project = &model.Project{}
	s.tc.taskConfig.Project.CallbackTimeout = 0
	s.sender = send.MakeInternalLogger()
	s.tc.logger = client.NewSingleChannelLogHarness("test", s.sender)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	heartbeat := make(chan string)
	go s.a.startHeartbeat(ctx, cancel, s.tc, heartbeat)
	select {
	case beat := <-heartbeat:
		s.Equal(evergreen.TaskFailed, beat)
	case <-ctx.Done():
		s.FailNow("heartbeat context errored before it could send a value back")
	}
}

func (s *BackgroundSuite) TestMaxHeartbeats() {
	s.mockCommunicator.HeartbeatShouldErr = true
	s.a.opts.HeartbeatInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	heartbeat := make(chan string)
	go s.a.startHeartbeat(ctx, cancel, s.tc, heartbeat)
	select {
	case beat := <-heartbeat:
		s.Equal(evergreen.TaskFailed, beat)
	case <-ctx.Done():
		s.FailNow("heartbeat context errored before it could send a value back")
	}
}

func (s *BackgroundSuite) TestHeartbeatSometimesFailsDoesNotFailTask() {
	s.mockCommunicator.HeartbeatShouldSometimesErr = true
	s.a.opts.HeartbeatInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	heartbeat := make(chan string)
	go s.a.startHeartbeat(ctx, cancel, s.tc, heartbeat)
	select {
	case <-heartbeat:
		s.FailNow("heartbeat should never receive signal when abort value remains false - timeout should have occurred.")
	case <-ctx.Done():
		beat := <-heartbeat
		s.Equal(evergreen.TaskFailed, beat)
	}
}

func (s *BackgroundSuite) TestHeartbeatFailsOnTaskConflict() {
	s.mockCommunicator.HeartbeatShouldConflict = true
	s.a.opts.HeartbeatInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	heartbeat := make(chan string)
	go s.a.startHeartbeat(ctx, cancel, s.tc, heartbeat)
	select {
	case <-heartbeat:
		s.FailNow("heartbeat should never receive signal when task conflicts - context cancel should have occurred.")
	case <-ctx.Done():
		beat := <-heartbeat
		s.Equal(evergreen.TaskFailed, beat)
	}
}

func (s *BackgroundSuite) TestGetCurrentTimeout() {
	s.tc.taskConfig.Timeout = &internal.Timeout{}
	cmdFactory, exists := command.GetCommandFactory("shell.exec")
	s.True(exists)
	cmd := cmdFactory()
	cmd.SetIdleTimeout(time.Second)
	s.tc.setCurrentCommand(cmd)
	s.tc.setCurrentIdleTimeout(cmd)
	s.Equal(time.Second, s.tc.getCurrentTimeout())
}

func (s *BackgroundSuite) TestGetTimeoutDefault() {
	s.Equal(defaultIdleTimeout, s.tc.getCurrentTimeout())
}
