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

func init() {
	grip.EmergencyPanic(errors.Wrap(command.RegisterCommand("command.mock", command.MockCommandFactory), "initializing mock command for testing"))
}

const defaultProjYml = `
buildvariants:
  - name: mock_build_variant

tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: echo hi

post:
  - command: shell.exec
    params:
      script: echo hi
`

type AgentSuite struct {
	suite.Suite
	a                *Agent
	mockCommunicator *client.Mock
	tc               *taskContext
	task             task.Task
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
	const bvName = "mock_build_variant"
	s.task = task.Task{
		Id:           "task_id",
		DisplayName:  "this_is_a_task_name",
		BuildVariant: bvName,
		Version:      versionID,
	}
	s.mockCommunicator.GetTaskResponse = &s.task

	s.tmpDirName, err = os.MkdirTemp("", filepath.Base(s.T().Name()))
	s.Require().NoError(err)

	project := &model.Project{
		Tasks: []model.ProjectTask{
			{
				Name: s.task.DisplayName,
			},
		},
		BuildVariants: []model.BuildVariant{{Name: bvName}},
	}
	taskConfig, err := internal.NewTaskConfig(s.tmpDirName, &apimodels.DistroView{}, project, &s.task, &model.ProjectRef{
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
		project:       project,
		taskModel:     &s.task,
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
	sender, err := s.a.GetSender(ctx, LogOutputStdout, "agent", "task_id", 2)
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
	nextTask := &apimodels.NextTaskResponse{
		TaskId:     "mocktaskid",
		TaskSecret: "",
		ShouldExit: false}

	ntr, err := s.a.processNextTask(s.ctx, nextTask, s.tc, false)

	s.NoError(err)
	s.Require().NotNil(ntr)
	s.False(ntr.shouldExit)
	s.True(ntr.noTaskToRun)
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

func (s *AgentSuite) TestLoopWithCancelledContext() {
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
	s.setupRunTask(defaultProjYml)
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

	endDetail := s.mockCommunicator.EndTaskResult.Detail
	s.Empty("", endDetail.Message, "the end message should not include any errors")
	s.Equal(evergreen.TaskSucceeded, endDetail.Status, "the task should succeed")
}

func (s *AgentSuite) TestFinishTaskWithNormalCompletedTask() {
	s.mockCommunicator.EndTaskResponse = &apimodels.EndTaskResponse{}

	for _, status := range evergreen.TaskCompletedStatuses {
		resp, err := s.a.finishTask(s.ctx, s.tc, status, "")
		s.Equal(&apimodels.EndTaskResponse{}, resp)
		s.NoError(err)
		s.NoError(s.tc.logger.Close())

		s.Equal(status, s.mockCommunicator.EndTaskResult.Detail.Status, "normal task completion should record the task status")
		checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{"Running post-task commands"}, []string{panicLog})
	}
}

func (s *AgentSuite) TestFinishTaskWithAbnormallyCompletedTask() {
	s.mockCommunicator.EndTaskResponse = &apimodels.EndTaskResponse{}

	const status = evergreen.TaskSystemFailed
	resp, err := s.a.finishTask(s.ctx, s.tc, status, "")
	s.Equal(&apimodels.EndTaskResponse{}, resp)
	s.NoError(err)
	s.NoError(s.tc.logger.Close())

	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status, "task that failed due to non-task-related reasons should record the final status")
	s.Equal(evergreen.CommandTypeSystem, s.mockCommunicator.EndTaskResult.Detail.Type)
	s.NotEmpty(s.mockCommunicator.EndTaskResult.Detail.Description)
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, nil, []string{
		panicLog,
		"Running post-task commands",
	})
}

func (s *AgentSuite) TestFinishTaskEndTaskError() {
	s.mockCommunicator.EndTaskShouldFail = true
	resp, err := s.a.finishTask(s.ctx, s.tc, evergreen.TaskSucceeded, "")
	s.Nil(resp)
	s.Error(err)
}

const panicLog = "hit panic"

func (s *AgentSuite) TestCancelledRunPreAndMainIsNonBlocking() {
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()
	status := s.a.runPreAndMain(ctx, s.tc)
	s.Equal(evergreen.TaskSystemFailed, status, "task that aborts before it even can run should system fail")
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, nil, []string{panicLog})
}

func (s *AgentSuite) TestRunPreAndMainIsPanicSafe() {
	// Just having the logger is enough to verify if a panic gets logged, but
	// still produces a panic since it relies on a lot of taskContext
	// fields.
	tc := &taskContext{
		logger: s.tc.logger,
	}
	s.NotPanics(func() {
		status := s.a.runPreAndMain(s.ctx, tc)
		s.Equal(evergreen.TaskSystemFailed, status, "panic in agent should system-fail the task")
	})
	s.NoError(tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{panicLog}, nil)
}

func (s *AgentSuite) TestStartTaskFailureInRunPreAndMainCausesSystemFailure() {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	// Simulate a situation where the task is not allowed to start, which should
	// result in system failure. Also, runPreAndMain should not block if there is
	// no consumer running in parallel to pick up the complete status.
	s.mockCommunicator.StartTaskShouldFail = true
	status := s.a.runPreAndMain(ctx, s.tc)
	s.Equal(evergreen.TaskSystemFailed, status, "task should system-fail when it cannot start the task")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, nil, []string{panicLog})
}

func (s *AgentSuite) TestRunCommandsEventuallyReturnsForCommandThatIgnoresContext() {
	const cmdSleepSecs = 100
	s.setupRunTask(`
pre:
  - command: command.mock
    params:
      sleep_seconds: 100
`)
	ctx, cancel := context.WithCancel(s.ctx)

	const waitUntilAbort = 2 * time.Second
	go func() {
		// Cancel the long-running command after giving the command some time to
		// start running.
		time.Sleep(waitUntilAbort)
		cancel()
	}()

	startAt := time.Now()
	err := s.a.runCommandsInBlock(ctx, s.tc, s.tc.taskConfig.Project.Pre.List(), runCommandsOptions{block: command.PreBlock})
	cmdDuration := time.Since(startAt)

	s.Error(err)
	s.True(utility.IsContextError(errors.Cause(err)), "command should have stopped due to context cancellation")

	s.True(cmdDuration > waitUntilAbort, "command should have only stopped when it received cancel")
	s.True(cmdDuration < cmdSleepSecs*time.Second, "command should not block if it's taking too long to stop")
}

func (s *AgentSuite) TestCancelledRunCommandsIsNonBlocking() {
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()

	s.setupRunTask(`
pre:
  - command: shell.exec
    params:
      script: exit 0
`)
	err := s.a.runCommandsInBlock(ctx, s.tc, s.tc.taskConfig.Project.Pre.List(), runCommandsOptions{block: command.PreBlock})
	s.Require().Error(err)

	s.True(utility.IsContextError(errors.Cause(err)))
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, nil, []string{panicLog})
}

func (s *AgentSuite) TestRunCommandsIsPanicSafe() {
	s.setupRunTask(`
pre:
  - command: shell.exec
    params:
      script: exit 0
`)
	tcMissingInfo := &taskContext{
		logger: s.tc.logger,
	}
	s.NotPanics(func() {
		// Intentionally provide in a task context which is lacking a lot of
		// information necessary to run commands for that task, which should
		// force a panic.
		err := s.a.runCommandsInBlock(s.ctx, tcMissingInfo, s.tc.taskConfig.Project.Pre.List(), runCommandsOptions{block: command.PreBlock})
		s.Require().Error(err)
	})

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{panicLog}, nil)
}

func (s *AgentSuite) TestPreSucceeds() {
	projYml := `
buildvariants:
  - name: mock_build_variant

pre:
  - command: shell.exec
    params:
      script: echo hi
`
	s.setupRunTask(projYml)

	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'pre'",
		"Running command 'shell.exec' (step 1 of 1) in block 'pre'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'pre'",
		"Finished running pre-task commands",
	}, []string{panicLog})
}

func (s *AgentSuite) TestPreTimeoutDoesNotFailTask() {
	projYml := `
buildvariants:
  - name: mock_build_variant

pre_timeout_secs: 1
pre:
  - command: shell.exec
    params:
      script: sleep 5
`
	s.setupRunTask(projYml)

	startAt := time.Now()
	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc))

	s.Less(time.Since(startAt), 5*time.Second, "pre command should have stopped early")
	s.False(s.tc.hadTimedOut(), "should not record pre timeout when pre cannot fail task")
	s.Zero(s.tc.getTimeoutType())
	s.Zero(s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'pre'",
		"Hit pre timeout (1s)",
		"Running pre-task commands failed",
	}, []string{panicLog})
}

func (s *AgentSuite) TestPreFailsTask() {
	projYml := `
pre_error_fails_task: true
pre:
  - command: shell.exec
    params:
      script: exit 1
`
	s.setupRunTask(projYml)

	s.Error(s.a.runPreTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'pre'",
		"Running pre-task commands failed",
	}, []string{panicLog})
}
func (s *AgentSuite) TestPreTimeoutFailsTask() {
	projYml := `
buildvariants:
  - name: mock_build_variant

pre_timeout_secs: 1
pre_error_fails_task: true
pre:
  - command: shell.exec
    params:
      script: sleep 5
`
	s.setupRunTask(projYml)

	startAt := time.Now()
	err := s.a.runPreTaskCommands(s.ctx, s.tc)
	s.Error(err)
	s.True(utility.IsContextError(errors.Cause(err)))

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.True(s.tc.hadTimedOut(), "should have recorded pre timeout because it fails the task")
	s.EqualValues(preTimeout, s.tc.getTimeoutType())
	s.Equal(time.Second, s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'pre'",
		"Hit pre timeout (1s)",
		"Running pre-task commands failed",
	}, []string{panicLog})
}

func (s *AgentSuite) TestMainTaskSucceeds() {
	projYml := `
tasks:
- name: this_is_a_task_name
  commands:
  - command: shell.exec
    params:
      script: exit 0
`
	s.setupRunTask(projYml)

	s.NoError(s.a.runTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task commands",
		"Set idle timeout for 'shell.exec'",
		"Running command 'shell.exec' (step 1 of 1)",
		"Finished command 'shell.exec' (step 1 of 1)",
		"Finished running task commands",
	}, []string{
		panicLog,
	})
}

func (s *AgentSuite) TestMainTaskFails() {
	projYml := `
tasks:
- name: this_is_a_task_name
  commands:
  - command: shell.exec
    params:
      script: exit 1
`
	s.setupRunTask(projYml)

	s.Error(s.a.runTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task commands",
		"Set idle timeout for 'shell.exec'",
		"Running command 'shell.exec' (step 1 of 1)",
		"Finished running task commands",
	}, []string{
		panicLog,
	})
}

func (s *AgentSuite) TestPostSucceeds() {
	projYml := `
post:
  - command: shell.exec
    params:
      script: exit 0
`
	s.setupRunTask(projYml)

	s.NoError(s.a.runPostTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'post'",
		"Finished running post-task commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec'",
	})
}

func (s *AgentSuite) TestPostTimeoutDoesNotFailTask() {
	projYml := `
buildvariants:
  - name: mock_build_variant

post_timeout_secs: 1
post:
  - command: shell.exec
    params:
      script: sleep 5
`
	s.setupRunTask(projYml)

	startAt := time.Now()
	s.NoError(s.a.runPostTaskCommands(s.ctx, s.tc))

	s.Less(time.Since(startAt), 5*time.Second, "post command should have stopped early")
	s.False(s.tc.hadTimedOut(), "should not record post timeout when post cannot fail task")
	s.Zero(s.tc.getTimeoutType())
	s.Zero(s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Hit post timeout (1s)",
		"Running post-task commands failed",
	}, []string{panicLog})
}

func (s *AgentSuite) TestPostFailsTask() {
	projYml := `
buildvariants:
  - name: mock_build_variant

post_error_fails_task: true
post:
  - command: shell.exec
    params:
      script: exit 1
`
	s.setupRunTask(projYml)

	s.Error(s.a.runPostTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, nil, []string{panicLog})
}

func (s *AgentSuite) TestPostTimeoutFailsTask() {
	projYml := `
buildvariants:
  - name: mock_build_variant

post_timeout_secs: 1
post_error_fails_task: true
post:
  - command: shell.exec
    params:
      script: sleep 5
`
	s.setupRunTask(projYml)

	startAt := time.Now()
	err := s.a.runPostTaskCommands(s.ctx, s.tc)
	s.Error(err)
	s.True(utility.IsContextError(errors.Cause(err)))

	s.Less(time.Since(startAt), 5*time.Second, "post command should have stopped early")
	s.True(s.tc.hadTimedOut(), "should have recorded post timeout because it fails the task")
	s.Equal(postTimeout, s.tc.getTimeoutType())
	s.Equal(time.Second, s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Hit post timeout (1s)",
		"Running post-task commands failed",
	}, []string{panicLog})
}

// setupRunTask sets up a project YAML to run in an agent suite test by reading
// the YAML, parsing it, and setting the necessary fields for it to run.
func (s *AgentSuite) setupRunTask(projYml string) {
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.Require().NoError(err)
	s.tc.taskConfig.Project = p
	s.tc.project = p
	s.mockCommunicator.GetProjectResponse = p
}

func (s *AgentSuite) TestFailingPostWithPostErrorFailsTaskSetsEndTaskResults() {
	projYml := `
buildvariants:
  - name: mock_build_variant

tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: exit 0

post_error_fails_task: true
post_timeout_secs: 1
post:
  - command: shell.exec
    params:
      script: sleep 5
`
	s.setupRunTask(projYml)
	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
		TaskGroup:  s.tc.taskGroup,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tc.taskDirectory)

	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.Equal("'shell.exec' (step 1 of 1) in block 'post'", s.mockCommunicator.EndTaskResult.Detail.Description)
	s.True(s.mockCommunicator.EndTaskResult.Detail.TimedOut)
	s.EqualValues(postTimeout, s.mockCommunicator.EndTaskResult.Detail.TimeoutType)
	s.Equal(time.Second, s.mockCommunicator.EndTaskResult.Detail.TimeoutDuration)

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) (test) to 2h0m0s.",
		"Running command 'shell.exec' (step 1 of 1)",
		"Finished command 'shell.exec' (step 1 of 1)",
		"Finished running task commands",
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'post'",
	})
}

func (s *AgentSuite) TestFailingPostDoesNotChangeEndTaskResults() {
	projYml := `
buildvariants:
  - name: mock_build_variant

tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: exit 0

post:
  - command: shell.exec
    params:
      script: exit 1
`
	s.setupRunTask(projYml)

	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
		TaskGroup:  s.tc.taskGroup,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tc.taskDirectory)

	s.NoError(err)
	s.Equal(evergreen.TaskSucceeded, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.Zero(s.mockCommunicator.EndTaskResult.Detail.Description, "should not include command failure description for a successful task")
	s.Zero(s.mockCommunicator.EndTaskResult.Detail.Type, "should not include command failure type for a successful task")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) (test) to 2h0m0s.",
		"Running command 'shell.exec' (step 1 of 1)",
		"Finished command 'shell.exec' (step 1 of 1)",
		"Finished running task commands",
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'post'",
	})
}

func (s *AgentSuite) TestSucceedingPostShowsCorrectEndTaskResults() {
	projYml := `
buildvariants:
  - name: mock_build_variant

post_error_fails_task: true
tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: exit 0

post:
  - command: shell.exec
    params:
      script: exit 0
`
	s.setupRunTask(projYml)
	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
		TaskGroup:  s.tc.taskGroup,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tc.taskDirectory)

	s.NoError(err)
	s.Equal(evergreen.TaskSucceeded, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.Zero(s.mockCommunicator.EndTaskResult.Detail.Description, "should not include command failure description for a successful task")
	s.Zero(s.mockCommunicator.EndTaskResult.Detail.Type, "should not include command failure type for a successful task")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) (test) to 2h0m0s.",
		"Running command 'shell.exec' (step 1 of 1)",
		"Finished command 'shell.exec' (step 1 of 1)",
		"Finished running task commands",
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'post'",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'post'",
	})
}

func (s *AgentSuite) TestTimedOutMainAndFailingPostShowsMainInEndTaskResults() {
	projYml := `
buildvariants:
  - name: mock_build_variant

post_error_fails_task: true
tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        timeout_secs: 1
        params:
          script: sleep 5

post:
  - command: shell.exec
    params:
       script: exit 1
`
	s.setupRunTask(projYml)
	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
		TaskGroup:  s.tc.taskGroup,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tc.taskDirectory)

	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.Equal("'shell.exec' (step 1 of 1)", s.mockCommunicator.EndTaskResult.Detail.Description, "should show main block command as the failing command if both main and post block commands fail")
	s.True(s.mockCommunicator.EndTaskResult.Detail.TimedOut, "should show main block command hitting timeout")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running command 'shell.exec' (step 1 of 1)",
		"Set idle timeout for 'shell.exec' (step 1 of 1) (test) to 1s.",
		"Hit idle timeout",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'post'",
	})
}

func (s *AgentSuite) TestSucceedingPostAfterMainDoesNotChangeEndTaskResults() {
	projYml := `
buildvariants:
  - name: mock_build_variant

post_error_fails_task: true
tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: exit 1

post:
  - command: shell.exec
    params:
      script: exit 0
`
	s.setupRunTask(projYml)
	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
		TaskGroup:  s.tc.taskGroup,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tc.taskDirectory)

	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.Equal("'shell.exec' (step 1 of 1)", s.mockCommunicator.EndTaskResult.Detail.Description)

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) (test) to 2h0m0s.",
		"Running command 'shell.exec' (step 1 of 1)",
		"Finished command 'shell.exec' (step 1 of 1)",
		"Finished running task commands",
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'post'",
	})
}

func (s *AgentSuite) TestPostContinuesOnError() {
	projYml := `
post:
  - command: shell.exec
    params:
      script: exit 1
  - command: shell.exec
    params:
      script: exit 0
`
	s.setupRunTask(projYml)

	s.NoError(s.a.runPostTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 2) in block 'post'",
		"Running command 'shell.exec' (step 2 of 2) in block 'post'",
		"Finished command 'shell.exec' (step 2 of 2) in block 'post'",
		"Finished running post-task commands",
	}, []string{panicLog})
}

func (s *AgentSuite) TestEndTaskResponse() {
	factory, ok := command.GetCommandFactory("setup.initial")
	s.True(ok)
	s.tc.setCurrentCommand(factory())

	s.T().Run("TaskHitsIdleTimeoutButTheTaskAlreadyFinishedRunningResultsInSuccessWithTimeout", func(t *testing.T) {
		// Simulate a (rare) scenario where the idle timeout is reached, but the
		// last command in the main block already finished. It does record that
		// the timeout occurred, but the task commands nonethelessstill
		// succeeded.
		s.tc.setTimedOut(true, idleTimeout)
		detail := s.a.endTaskResponse(s.ctx, s.tc, evergreen.TaskSucceeded, "message")
		s.True(detail.TimedOut)
		s.Equal(evergreen.TaskSucceeded, detail.Status)
		// TODO (EVG-20729): replace message, which is never used.
		s.Equal("message", detail.Message)
	})
	s.T().Run("TaskClearsIdleTimeoutAndTheTaskAlreadyFinishedRunningResultsInSuccessWithoutTimeout", func(t *testing.T) {
		s.tc.setTimedOut(false, idleTimeout)
		detail := s.a.endTaskResponse(s.ctx, s.tc, evergreen.TaskSucceeded, "message")
		s.False(detail.TimedOut)
		s.Equal(evergreen.TaskSucceeded, detail.Status)
		// TODO (EVG-20729): replace message, which is never used.
		s.Equal("message", detail.Message)
	})

	s.T().Run("TaskHitsIdleTimeoutAndFailsResultsInFailureWithTimeout", func(t *testing.T) {
		s.tc.setTimedOut(true, idleTimeout)
		detail := s.a.endTaskResponse(s.ctx, s.tc, evergreen.TaskFailed, "message")
		s.True(detail.TimedOut)
		s.Equal(evergreen.TaskFailed, detail.Status)
		// TODO (EVG-20729): replace message, which is never used.
		s.Equal("message", detail.Message)
	})

	s.T().Run("TaskClearsIdleTimeoutAndFailsResultsInFailureWithoutTimeout", func(t *testing.T) {
		s.tc.setTimedOut(false, idleTimeout)
		detail := s.a.endTaskResponse(s.ctx, s.tc, evergreen.TaskFailed, "message")
		s.False(detail.TimedOut)
		s.Equal(evergreen.TaskFailed, detail.Status)
		// TODO (EVG-20729): replace message, which is never used.
		s.Equal("message", detail.Message)
	})
}

func (s *AgentSuite) TestOOMTracker() {
	projYml := `
oom_tracker: true
buildvariants:
 - name: mock_build_variant
tasks: 
 - name: this_is_a_task_name
   commands: 
    - command: shell.exec
      params:
        script: "echo hi"
post:
  - command: shell.exec
    params:
      script: "echo hi"
`
	s.setupRunTask(projYml)
	s.a.opts.CloudProvider = "provider"
	pids := []int{1, 2, 3}
	lines := []string{"line 1", "line 2", "line 3"}
	s.tc.oomTracker = &mock.OOMTracker{
		Lines: lines,
		PIDs:  pids,
	}

	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
		TaskGroup:  s.tc.taskGroup,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tc.taskDirectory)
	s.NoError(err)
	s.Equal(evergreen.TaskSucceeded, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.True(s.mockCommunicator.EndTaskResult.Detail.OOMTracker.Detected)
	s.Equal(pids, s.mockCommunicator.EndTaskResult.Detail.OOMTracker.Pids)
}

func (s *AgentSuite) TestPrepareNextTask() {
	var err error
	nextTask := &apimodels.NextTaskResponse{}
	tc := &taskContext{}
	tc.logger, err = s.a.comm.GetLoggerProducer(s.ctx, s.tc.task, nil)
	s.Require().NoError(err)
	tc.taskModel = &task.Task{}
	tc.taskConfig = &internal.TaskConfig{
		Task: &task.Task{
			Version: "not_a_task_group_version",
		},
	}
	tc.taskDirectory = "task_directory"
	shouldSetupGroup, taskDirectory := s.a.finishPrevTask(s.ctx, nextTask, tc)
	s.True(shouldSetupGroup, "if the next task is not in a group, shouldSetupGroup should be true")
	s.Empty(taskDirectory)

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
	shouldSetupGroup, taskDirectory = s.a.finishPrevTask(s.ctx, nextTask, tc)
	s.True(shouldSetupGroup, "if the next task is in the same group as the previous task but ranSetupGroup was false, ranSetupGroup should be true")
	s.Empty(taskDirectory)

	tc.taskConfig = &internal.TaskConfig{
		Task: &task.Task{
			Version: versionID,
		},
	}
	tc.ranSetupGroup = true
	tc.taskDirectory = "task_directory"
	shouldSetupGroup, taskDirectory = s.a.finishPrevTask(s.ctx, nextTask, tc)
	s.False(shouldSetupGroup, "if the next task is in the same group as the previous task and we already ran the setup group, shouldSetupGroup should be false")
	s.Equal("task_directory", taskDirectory)

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
	shouldSetupGroup, taskDirectory = s.a.finishPrevTask(s.ctx, nextTask, tc)
	s.True(shouldSetupGroup, "if the next task is in the same version and task group name but a different build, shouldSetupGroup should be true")
	s.Empty(taskDirectory)
}

func (s *AgentSuite) TestGeneralTaskSetupSucceeds() {
	nextTask := &apimodels.NextTaskResponse{}
	s.setupRunTask(defaultProjYml)
	s.tc.taskDirectory = "task_directory"
	shouldSetupGroup, taskDirectory := s.a.finishPrevTask(s.ctx, nextTask, s.tc)
	_, shouldExit, err := s.a.setupTask(s.ctx, s.ctx, s.tc, nextTask, shouldSetupGroup, taskDirectory)
	s.False(shouldExit)
	s.NoError(err)
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Current command set to initial task setup (system).",
		"Making new folder",
		"Task logger initialized",
		"Execution logger initialized.",
		"System logger initialized.",
		"Starting task 'task_id', execution 0.",
	}, []string{panicLog})
}

func (s *AgentSuite) TestRunTaskWithUserDefinedTaskStatus() {
	projYml := `
buildvariants:
  - name: mock_build_variant

tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: exit 0
      - command: shell.exec
        params:
          script: exit 0
`
	s.setupRunTask(projYml)

	resp := &triggerEndTaskResp{
		Status:      evergreen.TaskFailed,
		Type:        evergreen.CommandTypeSetup,
		Description: "task failed",
	}
	s.tc.setUserEndTaskResponse(resp)

	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tc.taskDirectory)
	s.NoError(err)

	s.Equal(resp.Status, s.mockCommunicator.EndTaskResult.Detail.Status, "should set user-defined task status")
	s.Equal(resp.Type, s.mockCommunicator.EndTaskResult.Detail.Type, "should set user-defined command failure type")
	s.Equal(resp.Description, s.mockCommunicator.EndTaskResult.Detail.Description, "should set user-defined task description")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task commands",
		"Running command 'shell.exec' (step 1 of 2)",
		"Task status set to 'failed' with HTTP endpoint.",
	}, []string{
		panicLog,
		"Running 'shell.exec' (step 2 of 2)",
	})
}

func (s *AgentSuite) TestRunTaskWithUserDefinedTaskStatusOverwritesFailingCommand() {
	projYml := `
buildvariants:
  - name: mock_build_variant

tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: exit 1
`
	s.setupRunTask(projYml)

	resp := &triggerEndTaskResp{
		Status:      evergreen.TaskSucceeded,
		Description: "task succeeded",
	}
	s.tc.setUserEndTaskResponse(resp)

	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tc.taskDirectory)
	s.NoError(err)

	s.Equal(resp.Status, s.mockCommunicator.EndTaskResult.Detail.Status, "should set user-defined task status")
	s.Equal(resp.Type, s.mockCommunicator.EndTaskResult.Detail.Type, "should set user-defined command failure type")
	s.Equal(resp.Description, s.mockCommunicator.EndTaskResult.Detail.Description, "should set user-defined task description")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task commands",
		"Running command 'shell.exec' (step 1 of 1)",
		"Task status set to 'success' with HTTP endpoint.",
	}, []string{
		panicLog,
	})
}

func (s *AgentSuite) TestRunTaskWithUserDefinedTaskStatusContinuesCommands() {
	projYml := `
buildvariants:
  - name: mock_build_variant

tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: exit 0
      - command: shell.exec
        params:
          script: exit 0
`
	s.setupRunTask(projYml)

	resp := &triggerEndTaskResp{
		Status:         evergreen.TaskFailed,
		Type:           evergreen.CommandTypeTest,
		Description:    "task failed",
		ShouldContinue: true,
	}
	s.tc.setUserEndTaskResponse(resp)

	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tc.taskDirectory)
	s.NoError(err)

	s.Equal(resp.Status, s.mockCommunicator.EndTaskResult.Detail.Status, "should set user-defined task status")
	s.Equal(resp.Type, s.mockCommunicator.EndTaskResult.Detail.Type, "should set user-defined command failure type")
	s.Equal(resp.Description, s.mockCommunicator.EndTaskResult.Detail.Description, "should set user-defined task description")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task commands",
		"Running command 'shell.exec' (step 1 of 2)",
		"Finished command 'shell.exec' (step 1 of 2)",
		"Running command 'shell.exec' (step 2 of 2)",
		"Finished command 'shell.exec' (step 2 of 2)",
		"Task status set to 'failed' with HTTP endpoint.",
	}, []string{
		panicLog,
	})
}

func (s *AgentSuite) TestSetupGroupSucceeds() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
  - name: task_group_name
    setup_group:
      - command: shell.exec
        params:
          script: echo hi
`

	s.tc.taskConfig.Task.TaskGroup = taskGroup
	s.setupRunTask(projYml)

	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Finished running pre-task commands",
	}, []string{panicLog})
}

func (s *AgentSuite) TestSetupGroupFails() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
  - name: task_group_name
    setup_group_can_fail_task: true
    setup_group:
      - command: shell.exec
        params:
          script: exit 1
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	s.Error(s.a.runPreTaskCommands(s.ctx, s.tc), "setup group command error should fail task")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running setup-group commands for task group 'task_group_name'",
	}, []string{panicLog})
}

func (s *AgentSuite) TestSetupGroupTimeoutDoesNotFailTask() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
  - name: task_group_name
    setup_group_timeout_secs: 1
    setup_group:
      - command: shell.exec
        params:
          script: sleep 5
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	startAt := time.Now()
	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc), "setup group timeout should not fail task")

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.False(s.tc.hadTimedOut(), "should not have hit task timeout")
	s.Zero(s.tc.getTimeoutType())
	s.Zero(s.tc.getTimeoutDuration())
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Running setup-group commands for task group 'task_group_name'",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Hit setup group timeout (1s)",
	}, []string{panicLog})
}

func (s *AgentSuite) TestSetupGroupTimeoutFailsTask() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
  - name: task_group_name
    setup_group_can_fail_task: true
    setup_group_timeout_secs: 1
    setup_group:
      - command: shell.exec
        params:
          script: sleep 5
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	startAt := time.Now()
	err := s.a.runPreTaskCommands(s.ctx, s.tc)
	s.Error(err, "setup group timeout should fail task")
	s.True(utility.IsContextError(errors.Cause(err)))

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.True(s.tc.hadTimedOut(), "should have hit task timeout")
	s.Equal(setupGroupTimeout, s.tc.getTimeoutType())
	s.Equal(time.Second, s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Running setup-group commands for task group 'task_group_name'",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Hit setup group timeout (1s)",
	}, []string{panicLog})
}

func (s *AgentSuite) TestSetupTaskSucceeds() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
  - name: task_group_name
    setup_task:
      - command: shell.exec
        params:
          script: exit 0
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Finished running pre-task commands",
	}, []string{panicLog})
}

func (s *AgentSuite) TestSetupTaskFails() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
  - name: task_group_name
    setup_task_can_fail_task: true
    setup_task:
      - command: shell.exec
        params:
          script: exit 1
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	s.Error(s.a.runPreTaskCommands(s.ctx, s.tc), "setup task command error should fail task")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Running pre-task commands failed",
	}, []string{panicLog})
}

func (s *AgentSuite) TestSetupTaskTimeoutDoesNotFailTask() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
  - name: task_group_name
    setup_task_timeout_secs: 1
    setup_task:
      - command: shell.exec
        params:
          script: sleep 5
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	startAt := time.Now()
	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc), "setup task timeout should not fail task")

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.False(s.tc.hadTimedOut(), "should not have hit task timeout")
	s.Zero(s.tc.getTimeoutType())
	s.Zero(s.tc.getTimeoutDuration())
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Hit setup task timeout (1s)",
	}, []string{panicLog})
}

func (s *AgentSuite) TestSetupTaskTimeoutFailsTask() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
  - name: task_group_name
    setup_task_timeout_secs: 1
    setup_task_can_fail_task: true
    setup_task:
      - command: shell.exec
        params:
          script: sleep 5
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	startAt := time.Now()
	err := s.a.runPreTaskCommands(s.ctx, s.tc)
	s.Error(err, "setup task timeout should fail task")
	s.True(utility.IsContextError(errors.Cause(err)))

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.True(s.tc.hadTimedOut(), "should have hit task timeout")
	s.Equal(setupTaskTimeout, s.tc.getTimeoutType())
	s.Equal(time.Second, s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Hit setup task timeout (1s)",
	}, []string{panicLog})
}

func (s *AgentSuite) TestTeardownTaskSucceeds() {
	s.tc.taskGroup = "task_group_name"
	projYml := `
task_groups:
  - name: task_group_name
    teardown_task:
      - command: shell.exec
        params:
          script: exit 0
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = s.tc.taskGroup

	s.NoError(s.a.runPostTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Finished running post-task commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec'",
	})
}

func (s *AgentSuite) TestTeardownTaskFails() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
  - name: task_group_name
    teardown_task_can_fail_task: true
    teardown_task:
      - command: shell.exec
        params:
          script: exit 1
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	s.Error(s.a.runPostTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-task commands",
	}, []string{panicLog})
}

func (s *AgentSuite) TestTeardownTaskTimeoutDoesNotFailTask() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
  - name: task_group_name
    teardown_task_timeout_secs: 1
    teardown_task:
      - command: shell.exec
        params:
          script: sleep 5
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	startAt := time.Now()
	s.NoError(s.a.runPostTaskCommands(s.ctx, s.tc), "teardown task timeout should not fail task")

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.False(s.tc.hadTimedOut(), "should not have hit task timeout")
	s.Zero(s.tc.getTimeoutType())
	s.Zero(s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Hit teardown task timeout (1s)",
	}, []string{panicLog})
}

func (s *AgentSuite) TestTeardownTaskTimeoutFailsTask() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
  - name: task_group_name
    teardown_task_can_fail_task: true
    teardown_task_timeout_secs: 1
    teardown_task:
      - command: shell.exec
        params:
          script: sleep 5
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	startAt := time.Now()
	err := s.a.runPostTaskCommands(s.ctx, s.tc)
	s.Error(err, "teardown task timeout should fail task")
	s.True(utility.IsContextError(errors.Cause(err)))

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.True(s.tc.hadTimedOut(), "should have hit task timeout")
	s.Equal(teardownTaskTimeout, s.tc.getTimeoutType())
	s.Equal(time.Second, s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Hit teardown task timeout (1s)",
	}, []string{panicLog})
}

func (s *AgentSuite) TestTeardownGroupSucceeds() {
	s.tc.taskGroup = "task_group_name"
	projYml := `
task_groups:
  - name: task_group_name
    teardown_group:
      - command: shell.exec
        params:
          script: echo hi
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = s.tc.taskGroup

	s.a.runTeardownGroupCommands(s.ctx, s.tc)

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_group'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'teardown_group'",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec'",
	})
}

func (s *AgentSuite) TestTeardownGroupTimeout() {
	const taskGroup = "task_group_name"
	s.tc.taskGroup = taskGroup
	projYml := `
task_groups:
  - name: task_group_name
    teardown_group_timeout_secs: 1
    teardown_group:
      - command: shell.exec
        params:
          script: sleep 5
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	startAt := time.Now()
	s.a.runTeardownGroupCommands(s.ctx, s.tc)
	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-group commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_group'",
		"Hit teardown group timeout (1s)",
	}, []string{panicLog})
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
          script: echo hi
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup

	s.a.runTaskTimeoutCommands(s.ctx, s.tc)

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task-timeout commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'timeout'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'timeout'",
		"Finished running timeout commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec'",
	})
}

func (s *AgentSuite) TestTimeoutHitsCallbackTimeout() {
	s.tc.task = client.TaskData{
		ID:     "task_id",
		Secret: "task_secret",
	}

	projYml := `
timeout:
  - command: shell.exec
    params:
      script: sleep 5

callback_timeout_secs: 1
`
	s.setupRunTask(projYml)

	startAt := time.Now()
	s.a.runTaskTimeoutCommands(s.ctx, s.tc)

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.False(s.tc.hadTimedOut(), "should not record timeout for timeout block")
	s.Zero(s.tc.getTimeoutType())
	s.Zero(s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task-timeout commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'timeout'",
		"Hit callback timeout (1s)",
	}, []string{panicLog})
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

func (s *AgentSuite) TestAbortExitsMainAndRunsPost() {
	s.mockCommunicator.HeartbeatShouldAbort = true
	s.a.opts.HeartbeatInterval = 500 * time.Millisecond

	projYml := `
buildvariants:
  - name: mock_build_variant

tasks:
  - name: this_is_a_task_name
    commands:
    - command: shell.exec
      params:
        script: sleep 10

post:
  - command: shell.exec
    params:
      script: sleep 1

timeout:
  - commands: shell.exec
    params:
      script: exit 0
`
	s.setupRunTask(projYml)
	start := time.Now()
	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
		TaskGroup:  s.tc.taskGroup,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tc.taskDirectory)
	s.NoError(err)

	s.WithinDuration(start, time.Now(), 4*time.Second, "abort should prevent commands in the main block from continuing to run")
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status, "task that aborts during main block should fail")
	// The exact count is not of particular importance, we're only interested in
	// knowing that the heartbeat is still going despite receiving an abort.
	s.GreaterOrEqual(s.mockCommunicator.GetHeartbeatCount(), 1, "heartbeat should be still running for teardown_task block even when initial abort signal is received")
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Heartbeat received signal to abort task.",
		"Task completed - FAILURE",
		"Running post-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
	}, []string{
		panicLog,
		"Running task-timeout commands",
	})
}

func (s *AgentSuite) TestAbortExitsMainAndRunsTeardownTask() {
	s.mockCommunicator.HeartbeatShouldAbort = true
	s.a.opts.HeartbeatInterval = 500 * time.Millisecond

	projYml := `
buildvariants:
  - name: mock_build_variant

tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: sleep 5

task_groups:
  - name: some_task_group
    tasks:
      - this_is_a_task_name
    teardown_task:
      - command: shell.exec
        params:
          script: sleep 1

timeout:
  - commands: shell.exec
    params:
      script: exit 0
`
	s.setupRunTask(projYml)
	s.tc.taskGroup = "some_task_group"
	start := time.Now()
	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
		TaskGroup:  s.tc.taskGroup,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tc.taskDirectory)
	s.NoError(err)

	s.WithinDuration(start, time.Now(), 4*time.Second, "abort should prevent commands in the main block from continuing to run")
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status, "task that aborts during main block should fail")
	// The exact count is not of particular importance, we're only interested in
	// knowing that the heartbeat is still going despite receiving an abort.
	s.GreaterOrEqual(s.mockCommunicator.GetHeartbeatCount(), 1, "heartbeat should be still running for teardown_task block even when initial abort signal is received")
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Heartbeat received signal to abort task.",
		"Task completed - FAILURE",
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
	}, []string{
		panicLog,
		"Running task-timeout commands",
	})
}

// checkMockLogs checks the mock communicator's received task logs. Note that
// callers should flush the task logs before checking them to ensure that they
// are up-to-date.
func checkMockLogs(t *testing.T, mc *client.Mock, taskID string, logsToFind []string, logsToNotFind []string) {
	expectedLog := make(map[string]bool)
	for _, log := range logsToFind {
		expectedLog[log] = false
	}
	unexpectedLog := make(map[string]bool)
	for _, log := range logsToNotFind {
		unexpectedLog[log] = false
	}

	var allLogs []string
	for _, msg := range mc.GetMockMessages()[taskID] {
		for log := range expectedLog {
			if strings.Contains(msg.Message, log) {
				expectedLog[log] = true
			}
		}
		for log := range unexpectedLog {
			if strings.Contains(msg.Message, log) {
				unexpectedLog[log] = true
			}
		}
		allLogs = append(allLogs, msg.Message)
	}
	var displayLogs bool
	for log, found := range expectedLog {
		if !assert.True(t, found, "expected log, but was not found: %s", log) {
			displayLogs = true
		}
	}
	for log, found := range unexpectedLog {
		if !assert.False(t, found, "expected log to NOT be found, but it was found: %s", log) {
			displayLogs = true
		}
	}

	if displayLogs {
		grip.Infof("Logs for task '%s':\n%s\n", taskID, strings.Join(allLogs, "\n"))
	}
}
