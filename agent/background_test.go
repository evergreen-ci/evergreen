package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/suite"
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
		comm: client.NewMock("url"),
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
	start := time.Now()
	go s.a.startHeartbeat(ctx, cancel, s.tc, heartbeat)
	beat := <-heartbeat
	end := time.Now()
	s.Equal(evergreen.TaskFailed, beat)
	s.True(end.Sub(start) < time.Second) // canceled before context expired
}

func (s *BackgroundSuite) TestMaxHeartbeats() {
	s.mockCommunicator.HeartbeatShouldErr = true
	s.a.opts.HeartbeatInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	heartbeat := make(chan string)
	start := time.Now()
	go s.a.startHeartbeat(ctx, cancel, s.tc, heartbeat)
	beat := <-heartbeat
	end := time.Now()
	s.Equal(evergreen.TaskFailed, beat)
	s.True(end.Sub(start) < time.Second) // canceled before context expired
}

func (s *BackgroundSuite) TestHeartbeatSometimesFailsDoesNotFailTask() {
	s.mockCommunicator.HeartbeatShouldSometimesErr = true
	s.a.opts.HeartbeatInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	heartbeat := make(chan string)
	start := time.Now()
	go s.a.startHeartbeat(ctx, cancel, s.tc, heartbeat)
	beat := <-heartbeat
	end := time.Now()
	s.Equal(evergreen.TaskFailed, beat)
	s.True(end.Sub(start) >= time.Second) // canceled by context
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

func (s *BackgroundSuite) TestEarlyTerminationWatcher() {
	cwd, err := os.Getwd()
	s.Require().NoError(err)
	s.tc.taskDirectory = cwd
	yml := `
early_termination:
- command: shell.exec
  params:
    script: "echo 'spot instance is being taken away'"
`
	p := &model.Project{}
	ctx := context.Background()
	_, err = model.LoadProjectInto(ctx, []byte(yml), nil, "", p)
	s.NoError(err)
	s.tc.project = p
	s.tc.taskConfig = &internal.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id: "task_id",
		},
		Project: p,
		WorkDir: s.tc.taskDirectory,
	}

	alwaysTrue := func() bool {
		return true
	}
	calledActionFunc := false
	action := func() {
		calledActionFunc = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	doneChan := make(chan bool)
	go s.a.startEarlyTerminationWatcher(ctx, s.tc, alwaysTrue, action, doneChan)

	for {
		select {
		case <-doneChan:
			s.True(calledActionFunc)
			successMsg := "spot instance is being taken away"
			foundSuccessMsg := false
			for s.sender.HasMessage() {
				msg, _ := s.sender.GetMessageSafe()
				if msg.Rendered == successMsg {
					foundSuccessMsg = true
				}
			}
			s.True(foundSuccessMsg)
			return
		case <-ctx.Done():
			s.Fail("waited 20s without calling action func")
			return
		}
	}
}
