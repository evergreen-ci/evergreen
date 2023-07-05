package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
)

type AgentSuite struct {
	suite.Suite
	a                *Agent
	mockCommunicator *client.Mock
	tc               *taskContext
	ctx              context.Context
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
			LogOutput:  LogOutputStdout,
			LogPrefix:  "agent",
		},
		comm:   client.NewMock("url"),
		tracer: otel.GetTracerProvider().Tracer("noop_tracer"),
	}
	s.mockCommunicator = s.a.comm.(*client.Mock)
	s.a.jasper, err = jasper.NewSynchronizedManager(true)
	s.Require().NoError(err)

	const versionID = "v1"
	const bvName = "some_build_variant"
	tsk := &task.Task{
		Id:           "task_id",
		BuildVariant: bvName,
		Version:      versionID,
	}

	s.tmpDirName, err = os.MkdirTemp("", filepath.Base(s.T().Name()))
	s.Require().NoError(err)

	taskConfig, err := internal.NewTaskConfig(s.tmpDirName, &apimodels.DistroView{}, &model.Project{
		BuildVariants: []model.BuildVariant{{Name: bvName}},
	}, tsk, &model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
	}, &patch.Patch{}, util.Expansions{})
	s.Require().NoError(err)

	s.tc = &taskContext{
		task: client.TaskData{
			ID:     "task_id",
			Secret: "task_secret",
		},
		taskConfig:    taskConfig,
		taskModel:     tsk,
		ranSetupGroup: false,
		oomTracker:    &mock.OOMTracker{},
		expansions:    util.Expansions{},
		taskDirectory: s.tmpDirName,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	s.canceler = cancel
	s.ctx = ctx
	s.tc.logger, err = s.mockCommunicator.GetLoggerProducer(ctx, s.tc.task, nil)
	s.NoError(err)

	factory, ok := command.GetCommandFactory("setup.initial")
	s.True(ok)
	s.tc.setCurrentCommand(factory())
	s.tmpDirName, err = os.MkdirTemp("", filepath.Base(s.T().Name()))
	s.Require().NoError(err)
	s.tc.taskDirectory = s.tmpDirName
	sender, err := s.a.GetSender(ctx, LogOutputStdout, "agent")
	s.Require().NoError(err)
	s.a.SetDefaultLogger(sender)
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

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	errs := make(chan error, 1)
	go func() {
		errs <- s.a.loop(ctx)
	}()
	select {
	case err := <-errs:
		s.NoError(err)
	case <-ctx.Done():
		s.FailNow(ctx.Err().Error())
	}
}

func (s *AgentSuite) TestTaskWithoutSecret() {
	s.mockCommunicator.NextTaskResponse = &apimodels.NextTaskResponse{
		TaskId:     "mocktaskid",
		TaskSecret: "",
		ShouldExit: false}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	agentCtx, agentCancel := context.WithCancel(ctx)
	errs := make(chan error, 1)
	go func() {
		errs <- s.a.loop(agentCtx)
	}()
	time.Sleep(1 * time.Second)
	agentCancel()
	select {
	case err := <-errs:
		s.NoError(err)
	case <-ctx.Done():
		s.FailNow(ctx.Err().Error())
	}
}

func (s *AgentSuite) TestErrorGettingNextTask() {
	s.mockCommunicator.NextTaskShouldFail = true
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	errs := make(chan error, 1)
	go func() {
		errs <- s.a.loop(ctx)
	}()
	select {
	case err := <-errs:
		s.Error(err)
	case <-ctx.Done():
		s.FailNow(ctx.Err().Error())
	}
}

func (s *AgentSuite) TestCanceledContext() {
	s.a.opts.AgentSleepInterval = time.Millisecond
	s.a.opts.MaxAgentSleepInterval = time.Millisecond
	s.mockCommunicator.NextTaskIsNil = true
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	errs := make(chan error, 1)

	agentCtx, agentCancel := context.WithCancel(ctx)
	agentCancel()
	go func() {
		errs <- s.a.loop(agentCtx)
	}()
	select {
	case err := <-errs:
		s.NoError(err)
	case <-ctx.Done():
		s.FailNow(ctx.Err().Error())
	}
}

func (s *AgentSuite) TestAgentEndTaskShouldExit() {
	s.mockCommunicator.EndTaskResponse = &apimodels.EndTaskResponse{ShouldExit: true}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	errs := make(chan error, 1)
	go func() {
		errs <- s.a.loop(ctx)
	}()
	select {
	case err := <-errs:
		s.NoError(err)
	case <-ctx.Done():
		s.FailNow(ctx.Err().Error())
	}
}

func (s *AgentSuite) TestNextTaskConflict() {
	s.mockCommunicator.NextTaskShouldConflict = true
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	agentCtx, agentCancel := context.WithCancel(ctx)
	defer agentCancel()

	errs := make(chan error, 1)
	go func() {
		errs <- s.a.loop(agentCtx)
	}()
	time.Sleep(1 * time.Second)
	agentCancel()

	select {
	case err := <-errs:
		s.NoError(err)
	case <-ctx.Done():
		s.FailNow(ctx.Err().Error())
	}
}

func (s *AgentSuite) TestFinishTaskReturnsEndTaskResponse() {
	s.mockCommunicator.EndTaskResponse = &apimodels.EndTaskResponse{}
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	resp, err := s.a.finishTask(ctx, s.tc, evergreen.TaskSucceeded, "")
	s.Equal(&apimodels.EndTaskResponse{}, resp)
	s.NoError(err)
}

func (s *AgentSuite) TestFinishTaskEndTaskError() {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.mockCommunicator.EndTaskShouldFail = true
	resp, err := s.a.finishTask(ctx, s.tc, evergreen.TaskSucceeded, "")
	s.Nil(resp)
	s.Error(err)
}

func (s *AgentSuite) TestCancelledStartTaskIsNonBlocking() {
	complete := make(chan string, 1)
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()
	s.a.startTask(ctx, s.tc, complete)
	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
}

func (s *AgentSuite) TestStartTaskIsPanicSafe() {
	// Just having the logger is enough to verify if a panic gets logged, but
	// still produces a panic since it relies on a lot of taskContext
	// fields.
	tc := &taskContext{
		logger: s.tc.logger,
	}
	s.a.startTask(s.ctx, tc, nil)
	s.NoError(tc.logger.Close())
	s.NotEmpty(s.getPanicLogs())
}

func (s *AgentSuite) TestStartTaskResultChannelIsNonBlocking() {
	complete := make(chan string, 1)

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	// Simulate a situation where the task is not allowed to start, which should
	// result in system failure. Also, startTask should not block if there is
	// no consumer running in parallel to pick up the complete status.
	s.mockCommunicator.StartTaskShouldFail = true
	s.a.startTask(ctx, s.tc, complete)

	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())

	select {
	case <-ctx.Done():
		s.Fail("test context cancelled before startTask returned: %s", ctx.Err())
	case status := <-complete:
		s.Equal(evergreen.TaskSystemFailed, status)
	}
}

func (s *AgentSuite) TestCancelledRunCommandsIsNonBlocking() {
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()
	cmd := model.PluginCommandConf{
		Command: "shell.exec",
		Params: map[string]interface{}{
			"script": "echo hi",
		},
	}
	cmds := []model.PluginCommandConf{cmd}
	err := s.a.runCommandsInBlock(ctx, s.tc, cmds, runCommandsOptions{}, "")
	s.Require().Error(err)

	s.True(utility.IsContextError(errors.Cause(err)))
	s.Empty(s.getPanicLogs())
}

func (s *AgentSuite) TestRunCommandsIsPanicSafe() {
	tc := &taskContext{
		logger: s.tc.logger,
	}
	cmd := model.PluginCommandConf{
		Command: "shell.exec",
		Params: map[string]interface{}{
			"script": "echo hi",
		},
	}
	cmds := []model.PluginCommandConf{cmd}
	err := s.a.runCommandsInBlock(s.ctx, tc, cmds, runCommandsOptions{}, "")
	s.NoError(s.tc.logger.Close())

	s.Require().Error(err)
	s.NotEmpty(s.getPanicLogs())
}

func (s *AgentSuite) TestPre() {
	projYml := `
buildvariants:
- name: some_build_variant

pre:
  - command: shell.exec
    params:
      script: "echo hi"
`
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Project = p

	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())

	s.Empty(s.getPanicLogs())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id,
		"Running pre-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'pre'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'pre'",
		"Finished running pre-task commands",
	)
}

func (s *AgentSuite) TestPreFailsTask() {
	projYml := `
pre_error_fails_task: true
pre:
  - command: subprocess.exec
    params:
      command: "doesntexist"
`
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Project = p
	s.Error(s.a.runPreTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
}

func (s *AgentSuite) TestPostFailsTask() {
	projYml := `
buildvariants:
- name: some_build_variant

post_error_fails_task: true
post:
  - command: subprocess.exec
    params:
      command: "doesntexist"
`
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Project = p
	err = s.a.runPostTaskCommands(s.ctx, s.tc)
	s.Require().Error(err)
	s.NotContains(err.Error(), "panic")
	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
}

func (s *AgentSuite) TestPost() {
	projYml := `
post:
  - command: shell.exec
    params:
      script: "echo hi"
`
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Project = p
	s.NoError(s.a.runPostTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id,
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'post'",
		"Finished running post-task commands",
	)
}

func (s *AgentSuite) TestPostContinuesOnError() {
	projYml := `
post:
  - command: shell.exec
    params:
      script: "exit 1"
  - command: shell.exec
    params:
      script: "exit 0"
`
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Project = p
	s.NoError(s.a.runPostTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id,
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 2) in block 'post'",
		"Running command 'shell.exec' (step 2 of 2) in block 'post'",
		"Finished running post-task commands",
	)
}

func (s *AgentSuite) TestEndTaskResponse() {
	factory, ok := command.GetCommandFactory("setup.initial")
	s.True(ok)
	s.tc.setCurrentCommand(factory())

	s.tc.setTimedOut(true, idleTimeout)
	detail := s.a.endTaskResponse(s.tc, evergreen.TaskSucceeded, "message")
	s.True(detail.TimedOut)
	s.Equal(evergreen.TaskSucceeded, detail.Status)
	s.Equal("message", detail.Message)

	s.tc.setTimedOut(false, idleTimeout)
	detail = s.a.endTaskResponse(s.tc, evergreen.TaskSucceeded, "message")
	s.False(detail.TimedOut)
	s.Equal(evergreen.TaskSucceeded, detail.Status)
	s.Equal("message", detail.Message)

	s.tc.setTimedOut(true, idleTimeout)
	detail = s.a.endTaskResponse(s.tc, evergreen.TaskFailed, "message")
	s.True(detail.TimedOut)
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal("message", detail.Message)

	s.tc.setTimedOut(false, idleTimeout)
	detail = s.a.endTaskResponse(s.tc, evergreen.TaskFailed, "message")
	s.False(detail.TimedOut)
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal("message", detail.Message)
}

func (s *AgentSuite) TestAbort() {
	s.mockCommunicator.HeartbeatShouldAbort = true
	s.a.opts.HeartbeatInterval = time.Nanosecond
	// TODO (EVG-19590): fix this test by splitting runTask.
	// This is a little weird, but it's necessary for now because the test loads
	// mock data, which changes the task ID.
	// Once this is fixed, we should uncomment the assertion below about
	// post-task commands (an aborted task should not run post).
	originalTaskID := s.tc.taskConfig.Task.Id
	_, err := s.a.runTask(s.ctx, s.tc)
	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status, "aborting a task should send a failed status")

	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
	checkMockLogs(s.T(), s.mockCommunicator, originalTaskID,
		"Heartbeat received signal to abort task",
		"Task completed - FAILURE",
		"Sending final task status: 'failed'",
	)

	// for _, msg := range s.mockCommunicator.GetMockMessages()[originalTaskID] {
	//     s.NotContains(msg.Message, "Running post-task commands", "aborted task should not run post block")
	// }
}

func (s *AgentSuite) TestOOMTracker() {
	pids := []int{1, 2, 3}
	lines := []string{"line 1", "line 2", "line 3"}
	s.tc.oomTracker = &mock.OOMTracker{
		Lines: lines,
		PIDs:  pids,
	}

	_, err := s.a.runTask(s.ctx, s.tc)
	s.NoError(err)
	s.Equal(evergreen.TaskSucceeded, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.True(s.mockCommunicator.EndTaskResult.Detail.OOMTracker.Detected)
	s.Equal(pids, s.mockCommunicator.EndTaskResult.Detail.OOMTracker.Pids)
}

func (s *AgentSuite) TestWaitCompleteSuccess() {
	heartbeat := make(chan string, 1)
	complete := make(chan string, 1)
	go func() {
		select {
		case <-s.ctx.Done():
		case complete <- evergreen.TaskSucceeded:
		}
	}()
	s.tc.project = &model.Project{}
	status := s.a.wait(s.ctx, s.ctx, s.tc, heartbeat, complete)
	s.Equal(evergreen.TaskSucceeded, status)
	s.False(s.tc.hadTimedOut())
}

func (s *AgentSuite) TestWaitCompleteFailure() {
	heartbeat := make(chan string, 1)
	complete := make(chan string, 1)
	go func() {
		select {
		case <-s.ctx.Done():
		case complete <- evergreen.TaskFailed:
		}
	}()
	s.tc.project = &model.Project{}
	status := s.a.wait(s.ctx, s.ctx, s.tc, heartbeat, complete)
	s.Equal(evergreen.TaskFailed, status)
	s.False(s.tc.hadTimedOut())
}

func (s *AgentSuite) TestWaitIdleTimeout() {
	var err error
	s.tc = &taskContext{
		task: client.TaskData{
			ID:     "task_id",
			Secret: "task_secret",
		},
		taskConfig: &internal.TaskConfig{
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
		oomTracker: &mock.OOMTracker{},
		project:    &model.Project{},
	}

	s.tc.logger, err = s.a.comm.GetLoggerProducer(s.ctx, s.tc.task, nil)
	s.NoError(err)
	factory, ok := command.GetCommandFactory("setup.initial")
	s.True(ok)
	s.tc.setCurrentCommand(factory())

	heartbeat := make(chan string, 1)
	complete := make(chan string, 1)
	var idleTimeoutCtx context.Context
	idleTimeoutCtx, idleTimeoutCancel := context.WithCancel(s.ctx)
	idleTimeoutCancel()

	status := s.a.wait(s.ctx, idleTimeoutCtx, s.tc, heartbeat, complete)
	s.Equal(evergreen.TaskFailed, status)
	s.False(s.tc.hadTimedOut())
}

func (s *AgentSuite) TestPrepareNextTask() {
	var err error
	nextTask := &apimodels.NextTaskResponse{}
	tc := &taskContext{}
	tc.logger, err = s.a.comm.GetLoggerProducer(s.ctx, s.tc.task, nil)
	s.NoError(err)
	tc.taskModel = &task.Task{}
	tc.taskConfig = &internal.TaskConfig{
		Task: &task.Task{
			Version: "not_a_task_group_version",
		},
	}
	tc.taskDirectory = "task_directory"
	tc = s.a.prepareNextTask(s.ctx, nextTask, tc)
	s.False(tc.ranSetupGroup, "if the next task is not in a group, ranSetupGroup should be false")
	s.Equal("", tc.taskGroup)
	s.Empty(tc.taskDirectory)

	const versionID = "task_group_version"
	nextTask.TaskGroup = "foo"
	tc.taskGroup = "foo"
	nextTask.Version = versionID
	tc.taskConfig = &internal.TaskConfig{
		Task: &task.Task{
			Version: versionID,
		},
	}
	tc.logger, err = s.a.comm.GetLoggerProducer(s.ctx, s.tc.task, nil)
	s.NoError(err)
	tc.taskDirectory = "task_directory"
	tc.ranSetupGroup = false
	tc = s.a.prepareNextTask(context.Background(), nextTask, tc)
	s.False(tc.ranSetupGroup, "if the next task is in the same group as the previous task but ranSetupGroup was false, ranSetupGroup should be false")
	s.Equal("foo", tc.taskGroup)
	s.Equal("", tc.taskDirectory)

	tc.taskConfig = &internal.TaskConfig{
		Task: &task.Task{
			Version: versionID,
		},
	}
	tc.ranSetupGroup = true
	tc.taskDirectory = "task_directory"
	tc = s.a.prepareNextTask(s.ctx, nextTask, tc)
	s.True(tc.ranSetupGroup, "if the next task is in the same group as the previous task and we already ran the setup group, ranSetupGroup should be true")
	s.Equal("foo", tc.taskGroup)
	s.Equal("task_directory", tc.taskDirectory)

	const newVersionID = "new_task_group_version"
	tc.taskConfig = &internal.TaskConfig{
		Task: &task.Task{
			Version: newVersionID,
			BuildId: "build_id_1",
		},
	}
	tc.logger, err = s.a.comm.GetLoggerProducer(s.ctx, s.tc.task, nil)
	s.NoError(err)
	nextTask.TaskGroup = "bar"
	nextTask.Version = newVersionID
	nextTask.Build = "build_id_2"
	tc.taskGroup = "bar"
	tc.taskDirectory = "task_directory"
	tc.taskModel = &task.Task{}
	tc = s.a.prepareNextTask(s.ctx, nextTask, tc)
	s.False(tc.ranSetupGroup, "if the next task in the same version but a different build, ranSetupGroup should be false")
	s.Equal("bar", tc.taskGroup)
	s.Empty(tc.taskDirectory)
}

func (s *AgentSuite) TestSetupGroup() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
- name: task_group_name
  setup_group:
  - command: shell.exec
    params:
      script: "echo hi"
`
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Task.TaskGroup = taskGroup
	s.tc.taskConfig.Project = p

	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id,
		"Running pre-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Finished running pre-task commands",
	)
}

func (s *AgentSuite) TestSetupGroupTimeout() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
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
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Project = p
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	err = s.a.runPreTaskCommands(s.ctx, s.tc)
	s.Require().Error(err)
	s.True(utility.IsContextError(errors.Cause(err)))
}

func (s *AgentSuite) TestSetupGroupFails() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	s.tc.ranSetupGroup = false
	projYml := `
task_groups:
- name: task_group_name
  setup_group_can_fail_task: true
  setup_group:
  - command: thisisnotarealcommand
    params:
      script: "echo hi"
`
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Project = p
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	s.Error(s.a.runPreTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, "running task setup group")
}

func (s *AgentSuite) TestTeardownTaskFails() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
- name: task_group_name
  teardown_task_can_fail_task: true
  teardown_task:
  - command: thisisnotarealcommand
    params:
      script: "echo hi"
`
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Project = p
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	s.Error(s.a.runPostTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, "running post-task commands")
}

func (s *AgentSuite) TestSetupTask() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
- name: task_group_name
  setup_task:
  - command: shell.exec
    params:
      script: "echo hi"
`
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Project = p
	s.tc.taskConfig.Task.TaskGroup = taskGroup
	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id,
		"Running pre-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Finished running pre-task commands",
	)
}

func (s *AgentSuite) TestTeardownTask() {
	s.tc.taskGroup = "task_group_name"
	projYml := `
task_groups:
- name: task_group_name
  teardown_task:
  - command: shell.exec
    params:
      script: "echo hi"
`
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Task.TaskGroup = s.tc.taskGroup
	s.tc.taskConfig.Project = p
	s.NoError(s.a.runPostTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id,
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Finished running post-task commands",
	)
}

func (s *AgentSuite) TestTeardownGroup() {
	s.tc.taskModel = &task.Task{}
	s.tc.taskGroup = "task_group_name"
	projYml := `
task_groups:
- name: task_group_name
  teardown_group:
  - command: shell.exec
    params:
      script: "echo hi"
`
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Project = p
	s.tc.taskConfig.Task.TaskGroup = s.tc.taskGroup
	s.a.runPostGroupCommands(s.ctx, s.tc)
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id,
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_group'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'teardown_group'",
	)
}

func (s *AgentSuite) TestTaskGroupTimeout() {
	const taskGroup = "task_group_name"
	s.tc.task = client.TaskData{
		ID:     "task_id",
		Secret: "task_secret",
	}
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
- name: task_group_name
  timeout:
  - command: shell.exec
    params:
      script: "echo hi"
`
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Project = p
	s.tc.taskConfig.Task.TaskGroup = taskGroup
	s.a.runTaskTimeoutCommands(s.ctx, s.tc)
	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id,
		"Running task-timeout commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'timeout'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'timeout'",
		"Finished running timeout commands",
	)
}

func (s *AgentSuite) TestTimeoutDoesNotWaitForCommandsToFinish() {
	s.tc.task = client.TaskData{
		ID:     "task_id",
		Secret: "task_secret",
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
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	p.CallbackTimeout = 2
	s.tc.taskConfig.Project = p
	now := time.Now()
	s.a.runTaskTimeoutCommands(s.ctx, s.tc)
	then := time.Now()
	s.True(then.Sub(now) < 4*time.Second)
	s.NoError(s.tc.logger.Close())
	s.Empty(s.getPanicLogs())
}

func (s *AgentSuite) TestFetchProjectConfig() {
	s.mockCommunicator.GetProjectResponse = &model.Project{
		Identifier: "some_cool_project",
	}

	s.NoError(s.a.fetchProjectConfig(s.ctx, s.tc))

	s.Require().NotZero(s.tc.project)
	s.Equal(s.mockCommunicator.GetProjectResponse.Identifier, s.tc.project.Identifier)
	s.Require().NotZero(s.tc.expansions)
	s.Equal("bar", s.tc.expansions["foo"], "should include mock communicator expansions")
	s.Equal("new-parameter-value", s.tc.expansions["overwrite-this-parameter"], "user-specified parameter should overwrite any other conflicting expansion")
	s.Require().NotZero(s.tc.privateVars)
	s.True(s.tc.privateVars["some_private_var"], "should include mock communicator private variables")
}

// getPanicLogs returns any panic-related mock logging.
func (s *AgentSuite) getPanicLogs() []string {
	var panicLogs []string
	msgsByTask := s.mockCommunicator.GetMockMessages()
	for _, msgs := range msgsByTask {
		for _, msg := range msgs {
			if strings.Contains(msg.Message, "panic") {
				panicLogs = append(panicLogs, msg.Message)
			}
		}
	}
	return panicLogs
}

func checkMockLogs(t *testing.T, mc *client.Mock, taskID string, logsToFind ...string) {
	logFound := make(map[string]bool)
	for _, log := range logsToFind {
		logFound[log] = false
	}
	var allLogs []string
	for _, msg := range mc.GetMockMessages()[taskID] {
		for log := range logFound {
			if strings.Contains(msg.Message, log) {
				logFound[log] = true
			}
			allLogs = append(allLogs, msg.Message)
		}
	}
	var displayLogs bool
	for log, found := range logFound {
		if !assert.True(t, found, "expected log not found: %s", log) {
			displayLogs = true
		}
	}
	if displayLogs {
		grip.Infof("Logs for task '%s':\n%s\n", taskID, strings.Join(allLogs, "\n"))
	}
}
