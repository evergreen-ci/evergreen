package agent

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/globals"
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
	ctx              context.Context
	cancel           context.CancelFunc
	a                *Agent
	mockCommunicator *client.Mock
	tc               *taskContext
	sender           *send.InternalSender
}

func TestBackgroundSuite(t *testing.T) {
	suite.Run(t, new(BackgroundSuite))
}

func (s *BackgroundSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	var err error
	s.a = &Agent{
		opts: Options{
			HostID:     "host",
			HostSecret: "secret",
			StatusPort: 2286,
			LogOutput:  globals.LogOutputStdout,
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
	s.tc.taskConfig.Project = model.Project{}
	s.tc.taskConfig.Project.CallbackTimeout = 0
	s.tc.taskConfig.Project.TimeoutSecs = 180
	s.sender = send.MakeInternalLogger()
	s.tc.logger = client.NewSingleChannelLogHarness("test", s.sender)
}

func (s *BackgroundSuite) TearDownTest() {
	s.cancel()
}

func (s *BackgroundSuite) TestStartTimeoutWatcherTimesOut() {
	const testTimeout = 30 * time.Second
	ctx, cancel := context.WithTimeout(s.ctx, testTimeout)
	defer cancel()
	const timeout = time.Nanosecond
	startAt := time.Now()
	timeoutOpts := timeoutWatcherOptions{
		tc:                    s.tc,
		kind:                  globals.CallbackTimeout,
		getTimeout:            func() time.Duration { return timeout },
		canMarkTimeoutFailure: true,
	}
	s.a.startTimeoutWatcher(ctx, cancel, timeoutOpts)

	s.Less(time.Since(startAt), testTimeout)
	s.Error(ctx.Err(), "should have cancelled operation to time out")
	s.True(s.tc.hadTimedOut())
	s.Equal(globals.CallbackTimeout, s.tc.getTimeoutType(), "should have hit callback timeout")
	s.Equal(timeout, s.tc.getTimeoutDuration(), "should have recorded timeout duration")
}

func (s *BackgroundSuite) TestStartTimeoutWatcherExitsWithoutTimeout() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	timeoutWatcherDone := make(chan struct{})
	go func() {
		timeoutOpts := timeoutWatcherOptions{
			tc:                    s.tc,
			kind:                  globals.CallbackTimeout,
			getTimeout:            func() time.Duration { return time.Hour },
			canMarkTimeoutFailure: true,
		}
		s.a.startTimeoutWatcher(ctx, cancel, timeoutOpts)
		close(timeoutWatcherDone)
	}()

	cancel()
	<-timeoutWatcherDone

	s.False(s.tc.hadTimedOut())
	s.Zero(s.tc.getTimeoutType())
	s.Zero(s.tc.getTimeoutDuration())
}

func (s *BackgroundSuite) TestStartTimeoutWatcherTimesOutButDoesNotMarkTimeoutFailure() {
	const testTimeout = 30 * time.Second
	ctx, cancel := context.WithTimeout(s.ctx, testTimeout)
	defer cancel()
	const timeout = time.Nanosecond
	startAt := time.Now()
	timeoutOpts := timeoutWatcherOptions{
		tc:         s.tc,
		kind:       globals.CallbackTimeout,
		getTimeout: func() time.Duration { return timeout },
	}
	s.a.startTimeoutWatcher(ctx, cancel, timeoutOpts)

	s.Less(time.Since(startAt), testTimeout)
	s.Error(ctx.Err(), "should have cancelled operation to time out")
	s.False(s.tc.hadTimedOut(), "should not have marked timeout failure")
	s.Zero(s.tc.getTimeoutType())
	s.Zero(s.tc.getTimeoutDuration())
}

const (
	defaultHeartbeatCheckInterval = 100 * time.Millisecond
	defaultNumHeartbeatChecks     = 3
)

func (s *BackgroundSuite) TestAbortedTaskStillHeartbeats() {
	s.mockCommunicator.HeartbeatShouldAbort = true
	s.a.opts.HeartbeatInterval = time.Millisecond

	heartbeatCtx, heartbeatCancel := context.WithTimeout(s.ctx, time.Second)
	defer heartbeatCancel()

	childCtx, childCancel := context.WithCancel(s.ctx)

	s.tc.setHeartbeatTimeout(heartbeatTimeoutOptions{})
	go s.a.startHeartbeat(heartbeatCtx, childCancel, s.tc)

	lastHeartbeatCount := 0
	s.checkHeartbeatCondition(heartbeatCheckOptions{
		heartbeatCtx:      heartbeatCtx,
		checkInterval:     defaultHeartbeatCheckInterval,
		numRequiredChecks: defaultNumHeartbeatChecks,
		checkCondition: func() bool {
			if childCtx.Err() == nil {
				// If the child context has not errored, the heartbeat has not
				// yet signaled for the task to abort.
				return false
			}

			// This is checking that the task was signaled to abort (via context
			// cancellation) due to getting an explicit abort message.
			// Furthermore, even though the task is aborting, the heartbeat
			// should continue running.
			currentHeartbeatCount := s.mockCommunicator.GetHeartbeatCount()
			s.Greater(currentHeartbeatCount, lastHeartbeatCount, "heartbeat should still be running")
			lastHeartbeatCount = currentHeartbeatCount

			return true
		},
		exitCondition: func() {
			s.FailNow("heartbeat exited before it could finish checks")
		},
	})
}

func (s *BackgroundSuite) TestHeartbeatSignalsAbortOnTaskConflict() {
	s.mockCommunicator.HeartbeatShouldConflict = true
	s.a.opts.HeartbeatInterval = time.Millisecond

	heartbeatCtx, heartbeatCancel := context.WithTimeout(s.ctx, time.Second)
	defer heartbeatCancel()

	childCtx, childCancel := context.WithCancel(s.ctx)

	s.tc.setHeartbeatTimeout(heartbeatTimeoutOptions{})
	go s.a.startHeartbeat(heartbeatCtx, childCancel, s.tc)

	lastHeartbeatCount := 0
	s.checkHeartbeatCondition(heartbeatCheckOptions{
		heartbeatCtx:      heartbeatCtx,
		checkInterval:     defaultHeartbeatCheckInterval,
		numRequiredChecks: defaultNumHeartbeatChecks,
		checkCondition: func() bool {
			if childCtx.Err() == nil {
				// If the child context has not errored, the heartbeat has not
				// yet signaled for the task to abort.
				return false
			}

			// This is checking that the task was signaled to abort (via context
			// cancellation) due to getting a task conflict (i.e. abort and
			// restart task). Furthermore, even though the task is aborting, the
			// heartbeat should continue running.
			currentHeartbeatCount := s.mockCommunicator.GetHeartbeatCount()
			s.Greater(currentHeartbeatCount, lastHeartbeatCount, "heartbeat should still be running")
			lastHeartbeatCount = currentHeartbeatCount

			return true
		},
		exitCondition: func() {
			s.FailNow("heartbeat exited before it could finish checks")
		},
	})
}

func (s *BackgroundSuite) TestHeartbeatSignalsAbortOnHittingMaxFailedHeartbeats() {
	s.mockCommunicator.HeartbeatShouldErr = true
	s.a.opts.HeartbeatInterval = time.Millisecond

	heartbeatCtx, heartbeatCancel := context.WithTimeout(s.ctx, time.Second)
	defer heartbeatCancel()

	childCtx, childCancel := context.WithCancel(s.ctx)

	s.tc.setHeartbeatTimeout(heartbeatTimeoutOptions{})
	go s.a.startHeartbeat(heartbeatCtx, childCancel, s.tc)

	lastHeartbeatCount := 0
	s.checkHeartbeatCondition(heartbeatCheckOptions{
		heartbeatCtx:      heartbeatCtx,
		checkInterval:     defaultHeartbeatCheckInterval,
		numRequiredChecks: defaultNumHeartbeatChecks,
		checkCondition: func() bool {
			if childCtx.Err() == nil {
				// If the child context has not errored, the heartbeat has not
				// yet signaled for the task to abort.
				return false
			}

			// This is checking that the task was signaled to abort (via context
			// cancellation) due to consistently failing to heartbeat.
			// Furthermore, even though the task is aborting, the heartbeat
			// should continue running.
			currentHeartbeatCount := s.mockCommunicator.GetHeartbeatCount()
			s.Greater(currentHeartbeatCount, lastHeartbeatCount, "heartbeat should still be running")
			lastHeartbeatCount = currentHeartbeatCount

			return true
		},
		exitCondition: func() {
			s.FailNow("heartbeat exited before it could finish checks")
		},
	})
}

func (s *BackgroundSuite) TestHeartbeatSignalsAbortWhenHeartbeatStops() {
	s.a.opts.HeartbeatInterval = time.Millisecond

	heartbeatCtx, heartbeatCancel := context.WithTimeout(s.ctx, time.Second)
	defer heartbeatCancel()

	childCtx, childCancel := context.WithCancel(s.ctx)

	s.tc.setHeartbeatTimeout(heartbeatTimeoutOptions{})
	go s.a.startHeartbeat(heartbeatCtx, childCancel, s.tc)

	s.checkHeartbeatCondition(heartbeatCheckOptions{
		heartbeatCtx:      heartbeatCtx,
		checkInterval:     defaultHeartbeatCheckInterval,
		numRequiredChecks: defaultNumHeartbeatChecks,
		checkCondition: func() bool {
			// This is checking that the task does not abort. There should be no
			// reason for the task to abort until the heartbeat exits.
			s.NoError(childCtx.Err())
			return true
		},
		exitCondition: func() {
			// Check that once the heartbeat exits, the task is signaled to
			// abort (if it is still running) in a timely manner.
			checkChildCtx, checkChildCancel := context.WithTimeout(s.ctx, 100*time.Millisecond)
			defer checkChildCancel()
			select {
			case <-childCtx.Done():
			case <-checkChildCtx.Done():
				s.FailNow("child context should be done in a timely manner")
			}
		},
	})
}

func (s *BackgroundSuite) TestHeartbeatSometimesFailsDoesNotFailTask() {
	s.mockCommunicator.HeartbeatShouldSometimesErr = true
	s.a.opts.HeartbeatInterval = time.Millisecond

	heartbeatCtx, heartbeatCancel := context.WithTimeout(s.ctx, time.Second)
	defer heartbeatCancel()

	childCtx, childCancel := context.WithCancel(s.ctx)

	s.tc.setHeartbeatTimeout(heartbeatTimeoutOptions{})
	go s.a.startHeartbeat(heartbeatCtx, childCancel, s.tc)

	s.checkHeartbeatCondition(heartbeatCheckOptions{
		heartbeatCtx:      heartbeatCtx,
		checkInterval:     defaultHeartbeatCheckInterval,
		numRequiredChecks: defaultNumHeartbeatChecks,
		checkCondition: func() bool {
			// This is checking that, even though the heartbeat is sporadically
			// failing, as long as it's succeeding sometimes, the task does not
			// abort. There should be no reason for the task to abort until the
			// heartbeat exits.
			s.NoError(childCtx.Err())
			return true
		},
		exitCondition: func() {
			// Check that once the heartbeat exits, the task is signaled to
			// abort (if it is still running) in a timely manner.
			checkChildCtx, checkChildCancel := context.WithTimeout(s.ctx, 100*time.Millisecond)
			defer checkChildCancel()
			select {
			case <-childCtx.Done():
			case <-checkChildCtx.Done():
				s.FailNow("child context should be done in a timely manner")
			}
		},
	})
}

func (s *BackgroundSuite) TestHeartbeatTimesOut() {
	s.mockCommunicator.HeartbeatShouldAbort = true
	s.a.opts.HeartbeatInterval = time.Millisecond

	heartbeatCtx, heartbeatCancel := context.WithTimeout(s.ctx, time.Second)
	defer heartbeatCancel()

	childCtx, childCancel := context.WithCancel(s.ctx)

	// Set the initial state so the heartbeat has already timed out.
	s.tc.setHeartbeatTimeout(heartbeatTimeoutOptions{
		startAt:    time.Now().Add(-10 * globals.DefaultHeartbeatTimeout),
		getTimeout: func() time.Duration { return globals.DefaultHeartbeatTimeout },
	})
	go s.a.startHeartbeat(heartbeatCtx, childCancel, s.tc)

	s.checkHeartbeatCondition(heartbeatCheckOptions{
		heartbeatCtx:      heartbeatCtx,
		checkInterval:     defaultHeartbeatCheckInterval,
		numRequiredChecks: defaultNumHeartbeatChecks,
		checkCondition: func() bool {
			if childCtx.Err() == nil {
				// If the child context has not errored, the heartbeat has not
				// yet timed out and signaled for the task to abort.
				return false
			}

			// This is checking that heartbeat exited due to timeout and is no
			// longer running.
			currentHeartbeatCount := s.mockCommunicator.GetHeartbeatCount()
			s.Zero(currentHeartbeatCount, "heartbeat should not run when it's timed out")

			return true
		},
		exitCondition: func() {
			s.FailNow("heartbeat exited before it could finish checks")
		},
	})
}

func (s *BackgroundSuite) TestIdleTimeoutIsSetForCommand() {
	s.tc.taskConfig.Timeout = internal.Timeout{}
	cmdFactory, exists := command.GetCommandFactory("shell.exec")
	s.True(exists)
	cmd := cmdFactory()
	cmd.SetIdleTimeout(time.Second)
	s.tc.setCurrentCommand(cmd)
	s.tc.setCurrentIdleTimeout(cmd)
	s.Equal(time.Second, s.tc.getCurrentIdleTimeout())
}

func (s *BackgroundSuite) TestIdleTimeoutIsSetForProject() {
	s.tc.taskConfig.Timeout = internal.Timeout{}
	cmdFactory, exists := command.GetCommandFactory("shell.exec")
	s.True(exists)
	cmd := cmdFactory()
	s.tc.setCurrentCommand(cmd)
	s.tc.setCurrentIdleTimeout(cmd)
	s.Equal(180*time.Second, s.tc.getCurrentIdleTimeout())
}

func (s *BackgroundSuite) TestGetTimeoutDefault() {
	s.Equal(globals.DefaultIdleTimeout, s.tc.getCurrentIdleTimeout())
}

type heartbeatCheckOptions struct {
	heartbeatCtx context.Context

	checkInterval     time.Duration
	numRequiredChecks int
	checkCondition    func() bool

	exitCondition func()
}

// checkHeartbeatCondition periodically checks a condition until the heartbeat
// exits. When the timer fires, it will check the abort condition, which can be
// used to check the current heartbeat/abort state. When the heartbeat exits, it
// will check the exit condition.
func (s *BackgroundSuite) checkHeartbeatCondition(abortCheck heartbeatCheckOptions) {
	timer := time.NewTimer(abortCheck.checkInterval)
	defer timer.Stop()

	numChecksPassed := 0
	for {
		select {
		case <-timer.C:
			timer.Reset(abortCheck.checkInterval)
			if checkPassed := abortCheck.checkCondition(); !checkPassed {
				continue
			}

			numChecksPassed++
			if numChecksPassed >= abortCheck.numRequiredChecks {
				return
			}
		case <-abortCheck.heartbeatCtx.Done():
			abortCheck.exitCondition()
		}
	}
}
