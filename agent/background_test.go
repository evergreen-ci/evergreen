package agent

import (
	"context"
	"testing"
	"time"

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
			LogOutput:  LogOutputStdout,
			LogPrefix:  "agent",
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

func (s *BackgroundSuite) TestAbortedTaskStillHeartbeats() {
	s.mockCommunicator.HeartbeatShouldAbort = true
	s.a.opts.HeartbeatInterval = time.Millisecond

	heartbeatCtx, heartbeatCancel := context.WithTimeout(context.Background(), time.Second)
	defer heartbeatCancel()

	childCtx, childCancel := context.WithCancel(context.Background())

	go s.a.startHeartbeat(heartbeatCtx, childCancel, s.tc)

	// kim: TODO: wrap this waiting func into a helper method
	const checkInterval = 100 * time.Millisecond
	timer := time.NewTimer(checkInterval)
	defer timer.Stop()
	lastHeartbeatCount := 0
	numHeartbeatChecks := 0
	for {
		select {
		case <-timer.C:
			timer.Reset(checkInterval)
			if childCtx.Err() == nil {
				continue
			}

			currentHeartbeatCount := s.mockCommunicator.GetHeartbeatCount()
			s.Greater(currentHeartbeatCount, lastHeartbeatCount, "heartbeat should still be running")
			lastHeartbeatCount = currentHeartbeatCount
			numHeartbeatChecks++
			if numHeartbeatChecks > 3 {
				// This is the success case - the task should have been signaled
				// to abort via context cancellation due to getting an abort
				// message before the heartbeat exits. Furthermore, the
				// heartbeat counts should be incrementing on each check as the
				// heartbeat continues running.
				return
			}
		case <-heartbeatCtx.Done():
			s.FailNow("heartbeat context errored before it could signal abort")
		}
	}
}

func (s *BackgroundSuite) TestHeartbeatSignalsAbortOnTaskConflict() {
	s.mockCommunicator.HeartbeatShouldConflict = true
	s.a.opts.HeartbeatInterval = time.Millisecond

	heartbeatCtx, heartbeatCancel := context.WithTimeout(context.Background(), time.Second)
	defer heartbeatCancel()

	childCtx, childCancel := context.WithCancel(context.Background())

	go s.a.startHeartbeat(heartbeatCtx, childCancel, s.tc)

	const checkInterval = 100 * time.Millisecond
	timer := time.NewTimer(checkInterval)
	defer timer.Stop()
	lastHeartbeatCount := 0
	numHeartbeatChecks := 0
	for {
		select {
		case <-timer.C:
			timer.Reset(checkInterval)
			if childCtx.Err() == nil {
				continue
			}

			currentHeartbeatCount := s.mockCommunicator.GetHeartbeatCount()
			s.Greater(currentHeartbeatCount, lastHeartbeatCount, "heartbeat should still be running")
			lastHeartbeatCount = currentHeartbeatCount
			numHeartbeatChecks++
			if numHeartbeatChecks > 3 {
				// This is the success case - the task should have been signaled
				// to abort via context cancellation due to getting a task
				// conflict (i.e. abort and restart task) before the heartbeat
				// exits. Furthermore, the heartbeat counts should be
				// incrementing on each check as the heartbeat continues
				// running.
				return
			}
		case <-heartbeatCtx.Done():
			s.FailNow("heartbeat context errored before it could signal abort")
		}
	}
}

func (s *BackgroundSuite) TestHeartbeatSignalsAbortOnHittingMaxFailedHeartbeats() {
	s.mockCommunicator.HeartbeatShouldErr = true
	s.a.opts.HeartbeatInterval = time.Millisecond

	heartbeatCtx, heartbeatCancel := context.WithTimeout(context.Background(), time.Second)
	defer heartbeatCancel()

	childCtx, childCancel := context.WithCancel(context.Background())

	go s.a.startHeartbeat(heartbeatCtx, childCancel, s.tc)

	const checkInterval = 100 * time.Millisecond
	timer := time.NewTimer(checkInterval)
	defer timer.Stop()
	lastHeartbeatCount := 0
	numHeartbeatChecks := 0
	for {
		select {
		case <-timer.C:
			timer.Reset(checkInterval)
			if childCtx.Err() == nil {
				continue
			}

			currentHeartbeatCount := s.mockCommunicator.GetHeartbeatCount()
			s.Greater(currentHeartbeatCount, lastHeartbeatCount, "heartbeat should still be running")
			lastHeartbeatCount = currentHeartbeatCount
			numHeartbeatChecks++
			if numHeartbeatChecks > 3 {
				// This is the success case - the task should have been signaled
				// to abort via context cancellation due to too many failed
				// heartbeats before the heartbeat exits. Furthermore, the
				// heartbeat counts should be incrementing on each check as the
				// heartbeat continues running.
				return
			}
		case <-heartbeatCtx.Done():
			s.FailNow("heartbeat context errored before it could signal abort")
		}
	}
}

func (s *BackgroundSuite) TestHeartbeatSignalsAbortWhenHeartbeatStops() {
	s.a.opts.HeartbeatInterval = time.Millisecond

	heartbeatCtx, heartbeatCancel := context.WithTimeout(context.Background(), time.Second)
	defer heartbeatCancel()

	childCtx, childCancel := context.WithCancel(context.Background())

	go s.a.startHeartbeat(heartbeatCtx, childCancel, s.tc)

	// kim: TODO: wrap this "no abort" checker into a helper method (same with
	// other "no abort" test)
	const checkInterval = 100 * time.Millisecond
	timer := time.NewTimer(checkInterval)
	numHeartbeatChecks := 0
	for {
		select {
		case <-timer.C:
			timer.Reset(checkInterval)
			// There should be no abort signal while the heartbeat is running.
			if numHeartbeatChecks > 3 {
				continue
			}

			// There should be no abort signal even though the heartbeat is
			// failing occasionally. The heartbeat should only send an abort
			// signal for many repeated heartbeat failures in a row.
			s.NoError(childCtx.Err())
			numHeartbeatChecks++
		case <-heartbeatCtx.Done():
			// kim: TODO: check this "eventual signal" more safely with a
			// timeout.
			select {
			case <-childCtx.Done():
			}
			// This is the success case - the heartbeat continued without ever
			// signaling the task to abort. It only signaled once the heartbeat
			// itself exited.
			return
		}
	}
}

func (s *BackgroundSuite) TestHeartbeatSometimesFailsDoesNotFailTask() {
	s.mockCommunicator.HeartbeatShouldSometimesErr = true
	s.a.opts.HeartbeatInterval = time.Millisecond

	heartbeatCtx, heartbeatCancel := context.WithTimeout(context.Background(), time.Second)
	defer heartbeatCancel()

	childCtx, childCancel := context.WithCancel(context.Background())

	go s.a.startHeartbeat(heartbeatCtx, childCancel, s.tc)

	const checkInterval = 100 * time.Millisecond
	timer := time.NewTimer(checkInterval)
	numHeartbeatChecks := 0
	for {
		select {
		case <-timer.C:
			timer.Reset(checkInterval)
			if numHeartbeatChecks > 3 {
				continue
			}

			// There should be no abort signal even though the heartbeat is
			// failing occasionally. The heartbeat should only send an abort
			// signal for many repeated heartbeat failures in a row.
			s.NoError(childCtx.Err())
			numHeartbeatChecks++
		case <-heartbeatCtx.Done():
			select {
			case <-childCtx.Done():
			}
			// s.Error(childCtx.Err(), "heartbeat should signal abort once the heartbeat has exited")
			// This is the success case - the heartbeat continued without ever
			// signaling the task to abort. It only signaled once the heartbeat
			// itself exited.
			return
		}
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
