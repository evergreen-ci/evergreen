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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type AgentSuite struct {
	suite.Suite
	a                *Agent
	mockCommunicator *client.Mock
	tc               *taskContext
	canceler         context.CancelFunc
}

func TestAgentSuite(t *testing.T) {
	suite.Run(t, new(AgentSuite))
}

func (s *AgentSuite) SetupTest() {
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

	s.tc = &taskContext{
		task: client.TaskData{
			ID:     "task_id",
			Secret: "task_secret",
		},
		taskConfig: &model.TaskConfig{
			Project: &model.Project{},
		},
		runGroupSetup: true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.canceler = cancel
	s.tc.logger = s.a.comm.GetLoggerProducer(ctx, s.tc.task)

	factory, ok := command.GetCommandFactory("setup.initial")
	s.True(ok)
	s.tc.setCurrentCommand(factory())
}

func (s *AgentSuite) TearDownTest() { s.canceler() }

func (s *AgentSuite) TestNextTaskResponseShouldExit() {
	s.mockCommunicator.NextTaskResponse = &apimodels.NextTaskResponse{
		TaskId:     "mocktaskid",
		TaskSecret: "",
		ShouldExit: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.a.loop(ctx)
	s.Error(err)
}

func (s *AgentSuite) TestTaskWithoutSecret() {
	s.mockCommunicator.NextTaskResponse = &apimodels.NextTaskResponse{
		TaskId:     "mocktaskid",
		TaskSecret: "",
		ShouldExit: false}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.a.loop(ctx)
	s.Error(err)
}

func (s *AgentSuite) TestErrorGettingNextTask() {
	s.mockCommunicator.NextTaskShouldFail = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.a.loop(ctx)
	s.Error(err)
}

func (s *AgentSuite) TestCanceledContext() {
	s.a.opts.AgentSleepInterval = time.Millisecond
	s.mockCommunicator.NextTaskIsNil = true
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := s.a.loop(ctx)
	s.NoError(err)
}

func (s *AgentSuite) TestAgentEndTaskShouldExit() {
	s.mockCommunicator.EndTaskResponse = &apimodels.EndTaskResponse{ShouldExit: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.a.loop(ctx)
	s.Error(err)
}

func (s *AgentSuite) TestFinishTaskReturnsEndTaskResponse() {
	endTaskResponse := &apimodels.EndTaskResponse{Message: "end task response"}
	s.mockCommunicator.EndTaskResponse = endTaskResponse
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, err := s.a.finishTask(ctx, s.tc, evergreen.TaskSucceeded)
	s.Equal(endTaskResponse, resp)
	s.NoError(err)
}

func (s *AgentSuite) TestFinishTaskEndTaskError() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.mockCommunicator.EndTaskShouldFail = true
	resp, err := s.a.finishTask(ctx, s.tc, evergreen.TaskSucceeded)
	s.Nil(resp)
	s.Error(err)
}

func (s *AgentSuite) TestCancelStartTask() {
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

func (s *AgentSuite) TestCancelRunCommands() {
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

func (s *AgentSuite) TestRunPreTaskCommands() {
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
		WorkDir: ".",
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

func (s *AgentSuite) TestRunPost() {
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

func (s *AgentSuite) TestEndTaskResponse() {
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

func (s *AgentSuite) TestAbort() {
	s.mockCommunicator.HeartbeatShouldAbort = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := s.a.runTask(ctx, s.tc)
	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.Equal("initial task setup", s.mockCommunicator.EndTaskResult.Detail.Description)
}

func (s *AgentSuite) TestWaitCompleteSuccess() {
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

func (s *AgentSuite) TestWaitCompleteFailure() {
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

func (s *AgentSuite) TestWaitExecTimeout() {
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

func (s *AgentSuite) TestWaitHeartbeatTimeout() {
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

func (s *AgentSuite) TestWaitIdleTimeout() {
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

func (s *AgentSuite) TestPrepareNextTask() {
	nextTask := &apimodels.NextTaskResponse{}
	tc := taskContext{}
	tc.taskDirectory = "task_directory"
	tc = s.a.prepareNextTask(context.Background(), nextTask, &tc)
	s.True(tc.runGroupSetup, "if the next task is not in a group, runGroupSetup should be true")
	s.Equal("", tc.taskGroup)
	s.Empty(tc.taskDirectory)

	nextTask.TaskGroup = "foo"
	tc.taskGroup = "foo"
	tc.taskDirectory = "task_directory"
	tc = s.a.prepareNextTask(context.Background(), nextTask, &tc)
	s.False(tc.runGroupSetup, "if the next task is in the same group as the previous task, runGroupSetup should be false")
	s.Equal("foo", tc.taskGroup)
	s.Equal("task_directory", tc.taskDirectory)

	nextTask.TaskGroup = "bar"
	tc.taskGroup = "foo"
	tc.taskDirectory = "task_directory"
	tc = s.a.prepareNextTask(context.Background(), nextTask, &tc)
	s.True(tc.runGroupSetup, "if the next task is in a different group from the previous task, runGroupSetup should be true")
	s.Equal("bar", tc.taskGroup)
	s.Empty(tc.taskDirectory)
}

func (s *AgentSuite) TestPreGroupCommands() {
	s.tc.taskGroup = "task_group_name"
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:        "task_id",
			TaskGroup: "task_group_name",
		},
		Project: &model.Project{
			TaskGroups: []model.TaskGroup{
				model.TaskGroup{
					Name: "task_group_name",
					SetupGroup: &model.YAMLCommandSet{
						SingleCommand: &model.PluginCommandConf{
							Command: "shell.exec",
							Params: map[string]interface{}{
								"working_dir": testutil.GetDirectoryOfFile(),
								"script":      "echo hi",
							},
						},
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
	s.Equal("Finished running pre-task commands.", msgs[len(msgs)-1].Message)
}

func (s *AgentSuite) TestPreTaskCommands() {
	s.tc.taskGroup = "task_group_name"
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:        "task_id",
			TaskGroup: "task_group_name",
		},
		Project: &model.Project{
			TaskGroups: []model.TaskGroup{
				model.TaskGroup{
					Name: "task_group_name",
					SetupTask: &model.YAMLCommandSet{
						SingleCommand: &model.PluginCommandConf{
							Command: "shell.exec",
							Params: map[string]interface{}{
								"working_dir": testutil.GetDirectoryOfFile(),
								"script":      "echo hi",
							},
						},
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
	s.Equal("Finished running pre-task commands.", msgs[len(msgs)-1].Message)
}

func (s *AgentSuite) TestRunPostTaskCommands() {
	s.tc.taskGroup = "task_group_name"
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:        "task_id",
			TaskGroup: "task_group_name",
		},
		Project: &model.Project{
			TaskGroups: []model.TaskGroup{
				model.TaskGroup{
					Name: "task_group_name",
					TeardownTask: &model.YAMLCommandSet{
						SingleCommand: &model.PluginCommandConf{
							Command: "shell.exec",
							Params: map[string]interface{}{
								"working_dir": testutil.GetDirectoryOfFile(),
								"script":      "echo hi",
							},
						},
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
	s.Equal("Running command 'shell.exec' (step 1 of 1)", msgs[1].Message)
	s.Contains(msgs[len(msgs)-2].Message, "Finished 'shell.exec'")
	s.Contains(msgs[len(msgs)-1].Message, "Finished running post-task commands")
}

func (s *AgentSuite) TestPostGroupCommands() {
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id: "task_id",
		},
		Project: &model.Project{},
	}
	tg := &model.TaskGroup{
		TeardownGroup: &model.YAMLCommandSet{
			SingleCommand: &model.PluginCommandConf{
				Command: "shell.exec",
				Params: map[string]interface{}{
					"working_dir": testutil.GetDirectoryOfFile(),
					"script":      "echo hi",
				},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.a.runPostGroupCommands(ctx, s.tc, tg)
	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Equal("Running command 'shell.exec' (step 1 of 1)", msgs[1].Message)
	s.Contains(msgs[len(msgs)-1].Message, "Finished 'shell.exec'")
}

func (s *AgentSuite) TestTimeoutGroupCommands() {
	s.tc.task = client.TaskData{
		ID:     "task_id",
		Secret: "task_secret",
	}
	s.tc.taskConfig = &model.TaskConfig{
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
		WorkDir: ".",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.a.runTaskTimeoutCommands(ctx, s.tc)
	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Equal("Running command 'shell.exec' (step 1 of 1)", msgs[2].Message)
	s.Contains(msgs[len(msgs)-2].Message, "Finished 'shell.exec'")
}

func TestAgentConstructorSetsHostData(t *testing.T) {
	assert := assert.New(t) // nolint
	agent := New(Options{HostID: "host_id", HostSecret: "host_secret"}, client.NewMock("url"))

	assert.Equal("host_id", agent.comm.GetHostID())
	assert.Equal("host_secret", agent.comm.GetHostSecret())
}
