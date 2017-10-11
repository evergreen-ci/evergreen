package agent

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type AgentTestSuite struct {
	suite.Suite
	a                Agent
	mockCommunicator *client.Mock
	tc               *taskContext
	canceler         context.CancelFunc
}

func TestAgentTestSuite(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}

func (s *AgentTestSuite) SetupTest() {
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

	s.tc = &taskContext{
		task: client.TaskData{
			ID:     "task_id",
			Secret: "task_secret",
		},
		taskConfig: &model.TaskConfig{
			Project: &model.Project{},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.canceler = cancel
	s.tc.logger = s.a.comm.GetLoggerProducer(ctx, s.tc.task)

	factory, ok := command.GetCommandFactory("setup.initial")
	s.True(ok)
	s.tc.setCurrentCommand(factory())
}

func (s *AgentTestSuite) TearDownTest() { s.canceler() }

func (s *AgentTestSuite) TestNextTaskResponseShouldExit() {
	s.mockCommunicator.NextTaskResponse = &apimodels.NextTaskResponse{
		TaskId:     "mocktaskid",
		TaskSecret: "",
		ShouldExit: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.a.loop(ctx)
	s.Error(err)
}

func (s *AgentTestSuite) TestTaskWithoutSecret() {
	s.mockCommunicator.NextTaskResponse = &apimodels.NextTaskResponse{
		TaskId:     "mocktaskid",
		TaskSecret: "",
		ShouldExit: false}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.a.loop(ctx)
	s.Error(err)
}

func (s *AgentTestSuite) TestErrorGettingNextTask() {
	s.mockCommunicator.NextTaskShouldFail = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.a.loop(ctx)
	s.Error(err)
}

func (s *AgentTestSuite) TestCanceledContext() {
	s.a.opts.AgentSleepInterval = time.Millisecond
	s.mockCommunicator.NextTaskIsNil = true
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := s.a.loop(ctx)
	s.NoError(err)
}

func (s *AgentTestSuite) TestAgentEndTaskShouldExit() {
	s.mockCommunicator.EndTaskResponse = &apimodels.EndTaskResponse{ShouldExit: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.a.loop(ctx)
	s.Error(err)
}

func (s *AgentTestSuite) TestFinishTaskReturnsEndTaskResponse() {
	endTaskResponse := &apimodels.EndTaskResponse{Message: "end task response"}
	s.mockCommunicator.EndTaskResponse = endTaskResponse
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, err := s.a.finishTask(ctx, s.tc, evergreen.TaskSucceeded)
	s.Equal(endTaskResponse, resp)
	s.NoError(err)
}

func (s *AgentTestSuite) TestFinishTaskEndTaskError() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.mockCommunicator.EndTaskShouldFail = true
	resp, err := s.a.finishTask(ctx, s.tc, evergreen.TaskSucceeded)
	s.Nil(resp)
	s.Error(err)
}

func (s *AgentTestSuite) TestCancelStartTask() {
	resetIdleTimeout := make(chan time.Duration)
	complete := make(chan string)
	go func() {
		for range resetIdleTimeout {
			// discard
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.a.startTask(ctx, s.tc, complete)
	msgs := s.mockCommunicator.GetMockMessages()
	s.Zero(len(msgs))
}

func (s *AgentTestSuite) TestCancelRunCommands() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := model.PluginCommandConf{
		Command: "shell.exec",
		Params: map[string]interface{}{
			"script": "echo hi",
		},
	}
	cmds := []model.PluginCommandConf{cmd}
	err := s.a.runCommands(ctx, s.tc, cmds, false)
	s.Error(err)
	s.Equal("runCommands canceled", err.Error())
}

func (s *AgentTestSuite) TestRunPreTaskCommands() {
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id: "task_id",
		},
		Project: &model.Project{
			Pre: &model.YAMLCommandSet{
				SingleCommand: &model.PluginCommandConf{
					Command: "shell.exec",
					Params: map[string]interface{}{
						"script": "echo hi",
					},
				},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.a.runPreTaskCommands(ctx, s.tc)

	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Equal("Running pre-task commands.", msgs[1].Message)
	s.Equal("Running command 'shell.exec' (step 1 of 1)", msgs[2].Message)
	s.Contains(msgs[len(msgs)-1].Message, "Finished running pre-task commands")
}

func (s *AgentTestSuite) TestRunPostTaskCommands() {
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id: "task_id",
		},
		Project: &model.Project{
			Post: &model.YAMLCommandSet{
				SingleCommand: &model.PluginCommandConf{
					Command: "shell.exec",
					Params: map[string]interface{}{
						"working_dir": testutil.GetDirectoryOfFile(),
						"script":      "echo hi",
					},
				},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.a.runPostTaskCommands(ctx, s.tc)
	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Equal("Running post-task commands.", msgs[1].Message)
	s.Equal("Running command 'shell.exec' (step 1 of 1)", msgs[2].Message)
	s.Contains(msgs[len(msgs)-1].Message, "Finished running post-task commands")
}

func (s *AgentTestSuite) TestEndTaskResponse() {
	factory, ok := command.GetCommandFactory("setup.initial")
	s.True(ok)
	s.tc.setCurrentCommand(factory())

	s.tc.timedOut = true
	detail := s.a.endTaskResponse(s.tc, evergreen.TaskSucceeded)
	s.True(detail.TimedOut)
	s.Equal(evergreen.TaskSucceeded, detail.Status)

	s.tc.timedOut = false
	detail = s.a.endTaskResponse(s.tc, evergreen.TaskSucceeded)
	s.False(detail.TimedOut)
	s.Equal(evergreen.TaskSucceeded, detail.Status)

	s.tc.timedOut = true
	detail = s.a.endTaskResponse(s.tc, evergreen.TaskFailed)
	s.True(detail.TimedOut)
	s.Equal(evergreen.TaskFailed, detail.Status)

	s.tc.timedOut = false
	detail = s.a.endTaskResponse(s.tc, evergreen.TaskFailed)
	s.False(detail.TimedOut)
	s.Equal(evergreen.TaskFailed, detail.Status)
}

func (s *AgentTestSuite) TestAbort() {
	s.mockCommunicator.HeartbeatShouldAbort = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := s.a.runTask(ctx, s.tc)
	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.Equal("initial task setup", s.mockCommunicator.EndTaskResult.Detail.Description)
}

func (s *AgentTestSuite) TestAgentConstructorSetsHostData() {
	agent := New(Options{HostID: "host_id", HostSecret: "host_secret"}, client.NewMock("url"))
	s.Equal("host_id", agent.comm.GetHostID())
	s.Equal("host_secret", agent.comm.GetHostSecret())
}

func (s *AgentTestSuite) TestWaitCompleteSuccess() {
	heartbeat := make(chan string)
	complete := make(chan string)
	go func() {
		complete <- evergreen.TaskSucceeded
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	innerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	status := s.a.wait(ctx, innerCtx, s.tc, heartbeat, complete)
	s.Equal(evergreen.TaskSucceeded, status)
	s.False(s.tc.hadTimedOut())
}

func (s *AgentTestSuite) TestWaitCompleteFailure() {
	heartbeat := make(chan string)
	complete := make(chan string)
	go func() {
		complete <- evergreen.TaskFailed
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	innerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	status := s.a.wait(ctx, innerCtx, s.tc, heartbeat, complete)
	s.Equal(evergreen.TaskFailed, status)
	s.False(s.tc.hadTimedOut())
}

func (s *AgentTestSuite) TestWaitExecTimeout() {
	heartbeat := make(chan string)
	complete := make(chan string)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	innerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	status := s.a.wait(ctx, innerCtx, s.tc, heartbeat, complete)
	s.Equal(evergreen.TaskFailed, status)
	s.False(s.tc.hadTimedOut())
}

func (s *AgentTestSuite) TestWaitHeartbeatTimeout() {
	heartbeat := make(chan string)
	complete := make(chan string)
	go func() {
		heartbeat <- evergreen.TaskUndispatched
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	innerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	status := s.a.wait(ctx, innerCtx, s.tc, heartbeat, complete)
	s.Equal(evergreen.TaskUndispatched, status)
	s.False(s.tc.hadTimedOut())
}

func (s *AgentTestSuite) TestWaitIdleTimeout() {
	s.tc = &taskContext{
		task: client.TaskData{
			ID:     "task_id",
			Secret: "task_secret",
		},
		taskConfig: &model.TaskConfig{
			BuildVariant: &model.BuildVariant{
				Name: "buildvariant_id",
			},
			Task: &task.Task{
				Id: "task_id",
			},
			Project: &model.Project{
				Timeout: &model.YAMLCommandSet{
					SingleCommand: &model.PluginCommandConf{
						Command: "shell.exec",
						Params: map[string]interface{}{
							"script": "echo hi",
						},
					},
				},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.tc.logger = s.a.comm.GetLoggerProducer(ctx, s.tc.task)
	factory, ok := command.GetCommandFactory("setup.initial")
	s.True(ok)
	s.tc.setCurrentCommand(factory())

	heartbeat := make(chan string)
	complete := make(chan string)
	var innerCtx context.Context
	innerCtx, cancel = context.WithCancel(context.Background())
	cancel()
	status := s.a.wait(ctx, innerCtx, s.tc, heartbeat, complete)
	s.Equal(evergreen.TaskFailed, status)
	s.False(s.tc.hadTimedOut())
}
