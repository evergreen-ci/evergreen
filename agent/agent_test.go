package agent

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/suite"
)

const (
	versionId = "v1"
)

type AgentSuite struct {
	suite.Suite
	a                *Agent
	mockCommunicator *client.Mock
	tc               *taskContext
	canceler         context.CancelFunc
	tmpDirName       string
}

func TestAgentSuite(t *testing.T) {
	suite.Run(t, new(AgentSuite))
}

func (s *AgentSuite) SetupTest() {
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
	s.a.jasper, err = jasper.NewSynchronizedManager(true)
	s.Require().NoError(err)

	s.tc = &taskContext{
		task: client.TaskData{
			ID:     "task_id",
			Secret: "task_secret",
		},
		taskConfig: &model.TaskConfig{
			Project: &model.Project{},
			Task:    &task.Task{},
		},
		runGroupSetup: true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.canceler = cancel
	s.tc.logger, err = s.a.comm.GetLoggerProducer(ctx, s.tc.task, nil)
	s.NoError(err)

	factory, ok := command.GetCommandFactory("setup.initial")
	s.True(ok)
	s.tc.setCurrentCommand(factory())
	s.tmpDirName, err = ioutil.TempDir("", "agent-command-suite-")
	s.Require().NoError(err)
	s.tc.taskDirectory = s.tmpDirName
}

func (s *AgentSuite) TearDownTest() {
	s.canceler()
	s.Require().NoError(os.RemoveAll(s.tmpDirName))
}

func (s *AgentSuite) TestNextTaskResponseShouldExit() {
	s.mockCommunicator.NextTaskResponse = &apimodels.NextTaskResponse{
		TaskId:     "mocktaskid",
		TaskSecret: "",
		ShouldExit: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.a.loop(ctx)
	s.NoError(err)
}

func (s *AgentSuite) TestTaskWithoutSecret() {
	s.mockCommunicator.NextTaskResponse = &apimodels.NextTaskResponse{
		TaskId:     "mocktaskid",
		TaskSecret: "",
		ShouldExit: false}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error)
	go func() {
		errChan <- s.a.loop(ctx)
	}()
	time.Sleep(1 * time.Second)
	cancel()
	err := <-errChan
	s.NoError(err)
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
	s.a.opts.MaxAgentSleepInterval = time.Millisecond
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
	s.NoError(err)
}

func (s *AgentSuite) TestNextTaskConflict() {
	s.mockCommunicator.NextTaskShouldConflict = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error)
	go func() {
		errChan <- s.a.loop(ctx)
	}()
	time.Sleep(time.Millisecond)
	select {
	case err := <-errChan:
		s.NoError(err)
	default:
		// pass
	}
}

func (s *AgentSuite) TestFinishTaskReturnsEndTaskResponse() {
	s.mockCommunicator.EndTaskResponse = &apimodels.EndTaskResponse{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, err := s.a.finishTask(ctx, s.tc, evergreen.TaskSucceeded)
	s.Equal(&apimodels.EndTaskResponse{}, resp)
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
	complete := make(chan string)
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
	err := s.a.runCommands(ctx, s.tc, cmds, runCommandsOptions{})
	s.Error(err)
	s.Equal("runCommands canceled", err.Error())
}

func (s *AgentSuite) TestPre() {
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:      "task_id",
			Version: versionId,
		},
		Project: &model.Project{},
		WorkDir: s.tc.taskDirectory,
	}
	projYml := `
pre:
  - command: shell.exec
    params:
      script: "echo hi"
`
	v := &model.Version{
		Id:     versionId,
		Config: projYml,
	}
	s.tc.taskConfig.Version = v
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.NoError(s.a.runPreTaskCommands(ctx, s.tc))
	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Equal("Running pre-task commands.", msgs[1].Message)
	s.Equal("Running command 'shell.exec' (step 1 of 1)", msgs[2].Message)
	s.Contains(msgs[len(msgs)-1].Message, "Finished running pre-task commands")
}

func (s *AgentSuite) TestPreFailsTask() {
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:      "task_id",
			Version: versionId,
		},
		Project: &model.Project{},
		WorkDir: s.tc.taskDirectory,
	}
	projYml := `
pre_error_fails_task: true
pre:
  - command: subprocess.exec
    params:
      command: "doesntexist"
`
	v := &model.Version{
		Id:     versionId,
		Config: projYml,
	}
	s.tc.taskConfig.Version = v
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Error(s.a.runPreTaskCommands(ctx, s.tc))
	s.NoError(s.tc.logger.Close())
}

func (s *AgentSuite) TestPost() {
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:      "task_id",
			Version: versionId,
		},
		Project: &model.Project{},
		WorkDir: s.tc.taskDirectory,
	}
	projYml := `
post:
  - command: shell.exec
    params:
      script: "echo hi"
`
	v := &model.Version{
		Id:     versionId,
		Config: projYml,
	}
	s.tc.taskConfig.Version = v
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.a.runPostTaskCommands(ctx, s.tc)
	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Equal("Running post-task commands.", msgs[1].Message)
	s.Equal("Running command 'shell.exec' (step 1 of 1)", msgs[2].Message)
	s.Contains(msgs[len(msgs)-1].Message, "Finished running post-task commands")
}

func (s *AgentSuite) TestPostContinuesOnError() {
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:      "task_id",
			Version: versionId,
		},
		Project: &model.Project{},
		WorkDir: s.tc.taskDirectory,
	}
	projYml := `
post:
  - command: shell.exec
    params:
      script: "exit 1"
  - command: shell.exec
    params:
      script: "exit 0"
`
	v := &model.Version{
		Id:     versionId,
		Config: projYml,
	}
	s.tc.taskConfig.Version = v
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.a.runPostTaskCommands(ctx, s.tc)
	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Equal("Running post-task commands.", msgs[1].Message)
	s.Equal("Running command 'shell.exec' (step 1 of 2)", msgs[2].Message)
	s.Contains(msgs[len(msgs)-1].Message, "Finished running post-task commands")
	found := map[string]bool{
		"Running post-task commands.":                false,
		"Running command 'shell.exec' (step 1 of 2)": false,
		"Running command 'shell.exec' (step 2 of 2)": false,
	}
	for _, msg := range msgs {
		for f := range found {
			if f == msg.Message {
				found[f] = true
			}
		}
	}
	for f, b := range found {
		s.True(b, fmt.Sprintf("did not find string %s in logs", f))
	}
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
	s.a.opts.HeartbeatInterval = time.Nanosecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := s.a.runTask(ctx, cancel, s.tc)
	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status)
	shouldFind := map[string]bool{
		"initial task setup":              false,
		"Running post-task commands":      false,
		"Sending final status as: failed": false,
	}
	for _, m := range s.mockCommunicator.GetMockMessages()["task_id"] {
		for toFind, _ := range shouldFind {
			if strings.Contains(m.Message, toFind) {
				shouldFind[toFind] = true
			}
		}
	}
	for toFind, found := range shouldFind {
		s.True(found, fmt.Sprintf("Expected to find '%s'", toFind))
	}
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
	var err error
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

	s.tc.logger, err = s.a.comm.GetLoggerProducer(ctx, s.tc.task, nil)
	s.NoError(err)
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
	var err error
	nextTask := &apimodels.NextTaskResponse{}
	tc := &taskContext{}
	tc.logger, err = s.a.comm.GetLoggerProducer(context.Background(), s.tc.task, nil)
	s.NoError(err)
	tc.taskConfig = &model.TaskConfig{
		Task: &task.Task{
			Version: "version_base",
		},
	}
	tc.taskDirectory = "task_directory"
	tc = s.a.prepareNextTask(context.Background(), nextTask, tc)
	s.True(tc.runGroupSetup, "if the next task is not in a group, runGroupSetup should be true")
	s.Equal("", tc.taskGroup)
	s.Empty(tc.taskDirectory)

	nextTask.TaskGroup = "foo"
	tc.taskGroup = "foo"
	nextTask.Version = "version_name"
	tc.taskConfig = &model.TaskConfig{
		Task: &task.Task{
			Version: "version_name",
		},
	}
	tc.logger, err = s.a.comm.GetLoggerProducer(context.Background(), s.tc.task, nil)
	s.NoError(err)
	tc.taskDirectory = "task_directory"
	tc = s.a.prepareNextTask(context.Background(), nextTask, tc)
	s.False(tc.runGroupSetup, "if the next task is in the same group as the previous task, runGroupSetup should be false")
	s.Equal("foo", tc.taskGroup)
	s.Equal("task_directory", tc.taskDirectory)

	tc.taskConfig = &model.TaskConfig{
		Task: &task.Task{
			Version: versionId,
			BuildId: "build_id_1",
		},
	}
	tc.logger, err = s.a.comm.GetLoggerProducer(context.Background(), s.tc.task, nil)
	s.NoError(err)
	nextTask.TaskGroup = "bar"
	nextTask.Version = versionId
	nextTask.Build = "build_id_2"
	tc.taskGroup = "bar"
	tc.taskDirectory = "task_directory"
	tc = s.a.prepareNextTask(context.Background(), nextTask, tc)
	s.True(tc.runGroupSetup, "if the next task in the same version but a different build, runSetupGroup should be true")
	s.Equal("bar", tc.taskGroup)
	s.Empty(tc.taskDirectory)
}

func (s *AgentSuite) TestAgentConstructorSetsHostData() {
	agent, err := New(Options{HostID: "host_id", HostSecret: "host_secret"}, client.NewMock("url"))
	s.Require().NoError(err)
	s.Equal("host_id", agent.comm.GetHostID())
	s.Equal("host_secret", agent.comm.GetHostSecret())
}

func (s *AgentSuite) TestGroupPreGroupCommands() {
	s.tc.taskGroup = "task_group_name"
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:        "task_id",
			TaskGroup: "task_group_name",
			Version:   versionId,
		},
		Project: &model.Project{},
		WorkDir: s.tc.taskDirectory,
	}
	s.tc.taskGroup = "task_group_name"
	projYml := `
task_groups:
- name: task_group_name
  setup_group:
  - command: shell.exec
    params:
      script: "echo hi"
`
	v := &model.Version{
		Id:     versionId,
		Config: projYml,
	}
	s.tc.taskConfig.Version = v
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.NoError(s.a.runPreTaskCommands(ctx, s.tc))
	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Equal("Running pre-task commands.", msgs[1].Message)
	s.Equal("Running command 'shell.exec' (step 1 of 1)", msgs[2].Message)
	s.Equal("Finished running pre-task commands.", msgs[len(msgs)-1].Message)
}

func (s *AgentSuite) TestGroupPreGroupSetupTimeout() {
	s.tc.taskGroup = "task_group_name"
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:        "task_id",
			TaskGroup: "task_group_name",
			Version:   versionId,
		},
		Project: &model.Project{},
		WorkDir: s.tc.taskDirectory,
	}
	s.tc.taskGroup = "task_group_name"
	projYml := `
task_groups:
- name: task_group_name
  setup_group_timeout_secs: 3
  setup_group_can_fail_task: true
  setup_group:
  - command: shell.exec
    params:
      script: "sleep 10"
`
	v := &model.Version{
		Id:     versionId,
		Config: projYml,
	}
	s.tc.taskConfig.Version = v
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := s.a.runPreTaskCommands(ctx, s.tc)
	s.Error(err)
	s.Contains(err.Error(), "context deadline exceeded")
}

func (s *AgentSuite) TestGroupPreGroupCommandsFail() {
	s.tc.taskGroup = "task_group_name"
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:        "task_id",
			TaskGroup: "task_group_name",
			Version:   versionId,
		},
		Project: &model.Project{},
		WorkDir: s.tc.taskDirectory,
	}
	s.tc.taskGroup = "task_group_name"
	s.tc.runGroupSetup = true
	projYml := `
task_groups:
- name: task_group_name
  setup_group_can_fail_task: true
  setup_group:
  - command: thisisnotarealcommand
    params:
      script: "echo hi"
`
	v := &model.Version{
		Id:     versionId,
		Config: projYml,
	}
	s.tc.taskConfig.Version = v
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Error(s.a.runPreTaskCommands(ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Contains(msgs[len(msgs)-1].Message, "error running task setup group")
}

func (s *AgentSuite) TestGroupPreTaskCommands() {
	s.tc.taskGroup = "task_group_name"
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:        "task_id",
			TaskGroup: "task_group_name",
			Version:   versionId,
		},
		Project: &model.Project{},
		WorkDir: s.tc.taskDirectory,
	}
	s.tc.taskGroup = "task_group_name"
	projYml := `
task_groups:
- name: task_group_name
  setup_task:
  - command: shell.exec
    params:
      script: "echo hi"
`
	v := &model.Version{
		Id:     versionId,
		Config: projYml,
	}
	s.tc.taskConfig.Version = v
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.NoError(s.a.runPreTaskCommands(ctx, s.tc))
	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Equal("Running pre-task commands.", msgs[1].Message)
	s.Equal("Running command 'shell.exec' (step 1 of 1)", msgs[2].Message)
	s.Equal("Finished running pre-task commands.", msgs[len(msgs)-1].Message)
}

func (s *AgentSuite) TestGroupPostTaskCommands() {
	s.tc.taskGroup = "task_group_name"
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:        "task_id",
			TaskGroup: "task_group_name",
			Version:   versionId,
		},
		Project: &model.Project{},
		WorkDir: s.tc.taskDirectory,
	}
	s.tc.taskGroup = "task_group_name"
	projYml := `
task_groups:
- name: task_group_name
  teardown_task:
  - command: shell.exec
    params:
      script: "echo hi"
`
	v := &model.Version{
		Id:     versionId,
		Config: projYml,
	}
	s.tc.taskConfig.Version = v
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.a.runPostTaskCommands(ctx, s.tc)
	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Equal("Running command 'shell.exec' (step 1 of 1)", msgs[2].Message)
	s.Contains(msgs[len(msgs)-2].Message, "Finished 'shell.exec'")
	s.Contains(msgs[len(msgs)-1].Message, "Finished running post-task commands")
}

func (s *AgentSuite) TestGroupPostGroupCommands() {
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:        "task_id",
			TaskGroup: "task_group_name",
			Version:   versionId,
		},
		Project: &model.Project{},
		WorkDir: s.tc.taskDirectory,
	}
	s.tc.taskGroup = "task_group_name"
	projYml := `
task_groups:
- name: task_group_name
  teardown_group:
  - command: shell.exec
    params:
      script: "echo hi"
`
	v := &model.Version{
		Id:     versionId,
		Config: projYml,
	}
	s.tc.taskConfig.Version = v
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.a.runPostGroupCommands(ctx, s.tc)
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Equal("Running command 'shell.exec' (step 1 of 1)", msgs[1].Message)
	s.Contains(msgs[len(msgs)-1].Message, "Finished 'shell.exec'")
}

func (s *AgentSuite) TestGroupTimeoutCommands() {
	s.tc.task = client.TaskData{
		ID:     "task_id",
		Secret: "task_secret",
	}
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:        "task_id",
			TaskGroup: "task_group_name",
			Version:   versionId,
		},
		Project: &model.Project{},
		WorkDir: s.tc.taskDirectory,
	}
	s.tc.taskGroup = "task_group_name"
	projYml := `
task_groups:
- name: task_group_name
  timeout:
  - command: shell.exec
    params:
      script: "echo hi"
`
	v := &model.Version{
		Id:     versionId,
		Config: projYml,
	}
	s.tc.taskConfig.Version = v
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.a.runTaskTimeoutCommands(ctx, s.tc)
	_ = s.tc.logger.Close()
	msgs := s.mockCommunicator.GetMockMessages()["task_id"]
	s.Equal("Running command 'shell.exec' (step 1 of 1)", msgs[2].Message)
	s.Contains(msgs[len(msgs)-2].Message, "Finished 'shell.exec'")
}

func (s *AgentSuite) TestTimeoutDoesNotWaitForChildProcs() {
	s.tc.task = client.TaskData{
		ID:     "task_id",
		Secret: "task_secret",
	}
	s.tc.taskConfig = &model.TaskConfig{
		BuildVariant: &model.BuildVariant{
			Name: "buildvariant_id",
		},
		Task: &task.Task{
			Id:      "task_id",
			Version: versionId,
		},
		Project: &model.Project{
			CallbackTimeout: 2,
		},
		WorkDir: s.tc.taskDirectory,
	}
	projYml := `
timeout:
  - command: shell.exec
    params:
      shell: bash
      script: |
        echo "hi"
        sleep 5
        echo "bye"
`
	v := &model.Version{
		Id:     versionId,
		Config: projYml,
	}
	s.tc.taskConfig.Version = v
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	now := time.Now()
	s.a.runTaskTimeoutCommands(ctx, s.tc)
	then := time.Now()
	s.True(then.Sub(now) < 4*time.Second)
	_ = s.tc.logger.Close()
}
