package agent

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/testutil"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/taskoutput"
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
          script: exit 0

post:
  - command: shell.exec
    params:
      script: exit 0
`

type AgentSuite struct {
	suite.Suite
	a                *Agent
	mockCommunicator *client.Mock
	tc               *taskContext
	task             task.Task
	ctx              context.Context
	canceler         context.CancelFunc
	suiteTmpDirName  string
	testTmpDirName   string
}

func TestAgentSuite(t *testing.T) {
	suite.Run(t, new(AgentSuite))
}

func (s *AgentSuite) SetupSuite() {
	s.suiteTmpDirName = s.T().TempDir()
}

func (s *AgentSuite) TearDownSuite() {
	if runtime.GOOS == "windows" {
		// This is a hack to give extra time for processes in Windows to finish
		// using the temporary working directory before the Go testing framework
		// cna attempt to clean it up. When using (testing.T).TempDir, the Go
		// testing framework will automatically clean up the directory at the
		// end of the test, and will fail the test if it cannot clean it up.
		// Furthermore, some agent tests are intentionally testing that the
		// agent will continue without waiting for a command after a context
		// error. Unfortunately, this means that by the time the test is
		// cleaning up, there may still be lingering processes accessing the
		// temporary task working directory. In Windows, if a process is still
		// using the directory, it can cause the Go testing framework to fail to
		// remove the directory, which fails the test. Therefore, the sleep here
		// gives the processes time to all shut down and stop using the
		// temporary working directory.
		time.Sleep(10 * time.Second)
	}
}

func (s *AgentSuite) SetupTest() {
	var err error

	s.testTmpDirName, err = os.MkdirTemp(s.suiteTmpDirName, filepath.Base(s.T().Name()))
	s.Require().NoError(err)

	s.a = &Agent{
		opts: Options{
			HostID:           "host",
			HostSecret:       "secret",
			StatusPort:       2286,
			LogOutput:        globals.LogOutputStdout,
			LogPrefix:        "agent",
			WorkingDirectory: s.testTmpDirName,
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
		Id:             "task_id",
		DisplayName:    "this_is_a_task_name",
		BuildVariant:   bvName,
		Version:        versionID,
		TaskOutputInfo: testutil.InitializeTaskOutput(s.T()),
	}
	s.mockCommunicator.GetTaskResponse = &s.task

	project := &model.Project{
		Tasks: []model.ProjectTask{
			{
				Name: s.task.DisplayName,
			},
		},
		BuildVariants: []model.BuildVariant{{Name: bvName}},
	}
	taskConfig, err := internal.NewTaskConfig(s.testTmpDirName, &apimodels.DistroView{}, project, &s.task, &model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
	}, &patch.Patch{}, &apimodels.ExpansionsAndVars{Expansions: util.Expansions{}})
	s.Require().NoError(err)

	s.tc = &taskContext{
		task: client.TaskData{
			ID:     "task_id",
			Secret: "task_secret",
		},
		taskConfig: taskConfig,
		oomTracker: &mock.OOMTracker{},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	s.canceler = cancel
	s.ctx = ctx
	s.tc.logger, err = s.mockCommunicator.GetLoggerProducer(ctx, &s.task, nil)
	s.NoError(err)

	factory, ok := command.GetCommandFactory("setup.initial")
	s.True(ok)
	s.tc.setCurrentCommand(factory())
	sender, err := s.a.GetSender(ctx, globals.LogOutputStdout, "agent", "task_id", 2)
	s.Require().NoError(err)
	s.a.SetDefaultLogger(sender)
}

func (s *AgentSuite) TearDownTest() {
	s.canceler()
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
	s.Equal(evergreen.TaskSucceeded, endDetail.Status, "the task should succeed")
	s.Empty(endDetail.Description, "should not include end task failure description for successful task")
}

func (s *AgentSuite) TestFinishTaskWithNormalCompletedTask() {
	s.mockCommunicator.EndTaskResponse = &apimodels.EndTaskResponse{}

	for _, status := range evergreen.TaskCompletedStatuses {
		resp, err := s.a.finishTask(s.ctx, s.tc, status, "")
		s.Equal(&apimodels.EndTaskResponse{}, resp)
		s.NoError(err)
		s.NoError(s.tc.logger.Close())

		s.Equal(status, s.mockCommunicator.EndTaskResult.Detail.Status, "normal task completion should record the task status")
		checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, nil, []string{panicLog})
	}
}

func (s *AgentSuite) TestFinishTaskWithAbnormallyCompletedTask() {
	s.mockCommunicator.EndTaskResponse = &apimodels.EndTaskResponse{}

	const status = evergreen.TaskSystemFailed
	resp, err := s.a.finishTask(s.ctx, s.tc, status, "")
	s.Equal(&apimodels.EndTaskResponse{}, resp)
	s.NoError(err)

	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status, "task that failed due to non-task-related reasons should record the final status")
	s.Equal(evergreen.CommandTypeSystem, s.mockCommunicator.EndTaskResult.Detail.Type)
	s.NotEmpty(s.mockCommunicator.EndTaskResult.Detail.Description)
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Task encountered unexpected task lifecycle system failure",
	}, []string{
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
		logger:     s.tc.logger,
		oomTracker: &mock.OOMTracker{},
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
	cmdBlock := commandBlock{
		block:    command.PreBlock,
		commands: s.tc.taskConfig.Project.Pre,
	}
	err := s.a.runCommandsInBlock(ctx, s.tc, cmdBlock)
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

	cmdBlock := commandBlock{
		block:    command.PreBlock,
		commands: s.tc.taskConfig.Project.Pre,
	}
	err := s.a.runCommandsInBlock(ctx, s.tc, cmdBlock)
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
		logger:     s.tc.logger,
		oomTracker: &mock.OOMTracker{},
	}
	s.NotPanics(func() {
		cmdBlock := commandBlock{
			block:    command.PreBlock,
			commands: s.tc.taskConfig.Project.Pre,
		}
		// Intentionally provide in a task context which is lacking a lot of
		// information necessary to run commands for that task, which should
		// force a panic.
		err := s.a.runCommandsInBlock(s.ctx, tcMissingInfo, cmdBlock)
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
      script: exit 0
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
	}, []string{
		panicLog,
		"Running pre-task commands failed",
	})
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
		"Finished command 'shell.exec' (step 1 of 1) in block 'pre'",
		"Running pre-task commands failed",
		"Finished running pre-task commands",
	}, []string{
		panicLog,
	})
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
		"Finished command 'shell.exec' (step 1 of 1) in block 'pre'",
		"Running pre-task commands failed",
		"Finished running pre-task commands",
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
	s.EqualValues(globals.PreTimeout, s.tc.getTimeoutType())
	s.Equal(time.Second, s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'pre'",
		"Hit pre timeout (1s)",
		"Running pre-task commands failed",
		"Finished running pre-task commands",
	}, []string{panicLog})
}

func (s *AgentSuite) TestPreContinuesOnError() {
	projYml := `
pre:
  - command: shell.exec
    params:
      script: exit 1
  - command: shell.exec
    params:
      script: exit 0
`
	s.setupRunTask(projYml)

	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Running command 'shell.exec' (step 1 of 2) in block 'pre'",
		"Finished command 'shell.exec' (step 1 of 2) in block 'pre'",
		"Running command 'shell.exec' (step 2 of 2) in block 'pre'",
		"Finished command 'shell.exec' (step 2 of 2) in block 'pre'",
		"Finished running pre-task commands",
	}, []string{
		panicLog,
	})
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
		"Running task commands failed",
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
		"Running task commands failed",
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

	s.NoError(s.a.runPostOrTeardownTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-task commands",
		"Setting heartbeat timeout to type 'post'",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'post'",
		"Resetting heartbeat timeout from type 'post' back to default",
		"Finished running post-task commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec'",
		"Running post-task commands failed",
	})
}

func (s *AgentSuite) TestPostSucceedsButErrorIsStored() {
	projYml := `
post:
  - command: shell.exec
    params:
      script: exit 1
`
	s.setupRunTask(projYml)
	s.NoError(s.a.runPostOrTeardownTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	s.True(s.tc.getPostErrored())
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
	s.NoError(s.a.runPostOrTeardownTaskCommands(s.ctx, s.tc))

	s.Less(time.Since(startAt), 5*time.Second, "post command should have stopped early")
	s.False(s.tc.hadTimedOut(), "should not record post timeout when post cannot fail task")
	s.Zero(s.tc.getTimeoutType())
	s.Zero(s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-task commands",
		"Setting heartbeat timeout to type 'post'",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Hit post timeout (1s)",
		"Finished command 'shell.exec' (step 1 of 1) in block 'post'",
		"Resetting heartbeat timeout from type 'post' back to default",
		"Running post-task commands failed",
		"Finished running post-task commands",
	}, []string{
		panicLog,
	})
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

	s.Error(s.a.runPostOrTeardownTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, nil, []string{panicLog})
	s.True(s.tc.getPostErrored())
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
	err := s.a.runPostOrTeardownTaskCommands(s.ctx, s.tc)
	s.Error(err)
	s.True(utility.IsContextError(errors.Cause(err)))

	s.Less(time.Since(startAt), 5*time.Second, "post command should have stopped early")
	s.True(s.tc.hadTimedOut(), "should have recorded post timeout because it fails the task")
	s.Equal(globals.PostTimeout, s.tc.getTimeoutType())
	s.Equal(time.Second, s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-task commands",
		"Setting heartbeat timeout to type 'post'",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Hit post timeout (1s)",
		"Resetting heartbeat timeout from type 'post' back to default",
		"Running post-task commands failed",
		"Finished running post-task commands",
	}, []string{panicLog})
}

// setupRunTask sets up a project YAML to run in an agent suite test by reading
// the YAML, parsing it, and setting the necessary fields for it to run.
func (s *AgentSuite) setupRunTask(projYml string) {
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.Require().NoError(err)
	s.tc.taskConfig.Project = *p
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
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)

	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.Equal("'shell.exec' (step 1 of 1) in block 'post'", s.mockCommunicator.EndTaskResult.Detail.Description)
	s.True(s.mockCommunicator.EndTaskResult.Detail.TimedOut)
	s.EqualValues(globals.PostTimeout, s.mockCommunicator.EndTaskResult.Detail.TimeoutType)
	s.Equal(time.Second, s.mockCommunicator.EndTaskResult.Detail.TimeoutDuration)

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) (test) to 2h0m0s.",
		"Running command 'shell.exec' (step 1 of 1)",
		"Finished command 'shell.exec' (step 1 of 1)",
		"Finished running task commands",
		"Running post-task commands",
		"Setting heartbeat timeout to type 'post'",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'post'",
		"Resetting heartbeat timeout from type 'post' back to default",
		"Running post-task commands failed",
		"Finished running post-task commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'post'",
	})
}

func (s *AgentSuite) TestRetryOnFailure() {
	projYml := `
tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        retry_on_failure: true
        params:
          script: exit 1
`
	s.setupRunTask(projYml)

	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)

	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.True(s.mockCommunicator.TaskShouldRetryOnFail)
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task commands",
		"Running command 'shell.exec' (step 1 of 1)",
		"Running task commands failed",
		fmt.Sprintf("Command is set to automatically restart on completion, this can be done %d total times per task.", evergreen.MaxAutomaticRestarts),
	}, []string{
		panicLog,
	})
}

func (s *AgentSuite) TestRetryOnFailureWithPreErrorFailsTask() {
	projYml := `
pre_error_fails_task: true

pre:
  - command: shell.exec
    retry_on_failure: true
    params:
      script: exit 1

tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: exit 0
`
	s.setupRunTask(projYml)

	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)

	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.True(s.mockCommunicator.TaskShouldRetryOnFail)
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Running command 'shell.exec' (step 1 of 1)",
		"Running pre-task commands failed",
		fmt.Sprintf("Command is set to automatically restart on completion, this can be done %d total times per task.", evergreen.MaxAutomaticRestarts),
	}, []string{
		panicLog,
	})
}

func (s *AgentSuite) TestRetryOnFailureWithoutPreErrorFailsTask() {
	projYml := `
pre_error_fails_task: false

pre:
  - command: shell.exec
    retry_on_failure: true
    params:
      script: exit 1

tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: exit 0
`
	s.setupRunTask(projYml)

	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)

	s.NoError(err)
	s.Equal(evergreen.TaskSucceeded, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.False(s.mockCommunicator.TaskShouldRetryOnFail)

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running pre-task commands",
		"Running command 'shell.exec' (step 1 of 1)",
	}, []string{
		panicLog,
		fmt.Sprintf("Command is set to automatically restart on completion, this can be done %d total times per task.", evergreen.MaxAutomaticRestarts),
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
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)

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
		"Setting heartbeat timeout to type 'post'",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'post'",
		"Resetting heartbeat timeout from type 'post' back to default",
		"Finished running post-task commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'post'",
		"Running post-task commands failed",
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
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)

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
		"Setting heartbeat timeout to type 'post'",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'post'",
		"Resetting heartbeat timeout from type 'post' back to default",
		"Finished running post-task commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'post'",
		"Running post-task commands failed",
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
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)

	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.Equal("'shell.exec' (step 1 of 1)", s.mockCommunicator.EndTaskResult.Detail.Description, "should show main block command as the failing command if both main and post block commands fail")
	s.True(s.mockCommunicator.EndTaskResult.Detail.TimedOut, "should show main block command hitting timeout")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running command 'shell.exec' (step 1 of 1)",
		"Set idle timeout for 'shell.exec' (step 1 of 1) (test) to 1s.",
		"Hit idle timeout",
		"Running post-task commands",
		"Setting heartbeat timeout to type 'post'",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'post'",
		"Resetting heartbeat timeout from type 'post' back to default",
		"Running post-task commands failed",
		"Finished running post-task commands",
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
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)

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
		"Setting heartbeat timeout to type 'post'",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'post'",
		"Resetting heartbeat timeout from type 'post' back to default",
		"Finished running post-task commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'post'",
		"Running post-task commands failed",
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

	s.NoError(s.a.runPostOrTeardownTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running post-task commands",
		"Setting heartbeat timeout to type 'post'",
		"Running command 'shell.exec' (step 1 of 2) in block 'post'",
		"Finished command 'shell.exec' (step 1 of 2) in block 'post'",
		"Running command 'shell.exec' (step 2 of 2) in block 'post'",
		"Finished command 'shell.exec' (step 2 of 2) in block 'post'",
		"Resetting heartbeat timeout from type 'post' back to default",
		"Finished running post-task commands",
	}, []string{
		panicLog,
	})
}

func (s *AgentSuite) TestEndTaskResponse() {
	factory, ok := command.GetCommandFactory("setup.initial")
	s.Require().True(ok)
	s.tc.setCurrentCommand(factory())

	const systemFailureDescription = "failure message"
	s.T().Run("TaskFailingWithCurrentCommandOverridesEmptyDescription", func(t *testing.T) {
		detail := s.a.endTaskResponse(s.ctx, s.tc, evergreen.TaskFailed, "")
		s.Equal(evergreen.TaskFailed, detail.Status)
		s.Contains(detail.Description, s.tc.getCurrentCommand().FullDisplayName())
	})
	s.T().Run("TaskFailingWithCurrentCommandIsOverriddenBySystemFailureDescription", func(t *testing.T) {
		detail := s.a.endTaskResponse(s.ctx, s.tc, evergreen.TaskFailed, systemFailureDescription)
		s.Equal(evergreen.TaskFailed, detail.Status)
		s.Equal(systemFailureDescription, detail.Description)
	})
	s.T().Run("TaskSucceedsWithEmptyDescription", func(t *testing.T) {
		detail := s.a.endTaskResponse(s.ctx, s.tc, evergreen.TaskSucceeded, "")
		s.False(detail.TimedOut)
		s.Equal(evergreen.TaskSucceeded, detail.Status)
		s.Empty(detail.Description)
	})
	s.T().Run("TaskSucceedsWithSystemFailureDescription", func(t *testing.T) {
		s.tc.setTimedOut(true, globals.IdleTimeout)
		defer s.tc.setTimedOut(false, "")
		detail := s.a.endTaskResponse(s.ctx, s.tc, evergreen.TaskSucceeded, systemFailureDescription)
		s.True(detail.TimedOut)
		s.Equal(evergreen.TaskSucceeded, detail.Status)
		s.Equal(systemFailureDescription, detail.Description)
	})
	s.T().Run("TaskWithUserDefinedTaskStatusAndDescriptionOverridesDescription", func(t *testing.T) {
		s.tc.userEndTaskResp = &triggerEndTaskResp{
			Description: "user description of what failed",
			Status:      evergreen.TaskFailed,
		}
		defer func() {
			s.tc.userEndTaskResp = nil
		}()
		detail := s.a.endTaskResponse(s.ctx, s.tc, evergreen.TaskSucceeded, systemFailureDescription)
		s.Equal(s.tc.userEndTaskResp.Status, detail.Status)
		s.Equal(s.tc.userEndTaskResp.Description, detail.Description)
	})
	s.T().Run("TaskHitsIdleTimeoutAndFailsResultsInFailureWithTimeout", func(t *testing.T) {
		s.tc.setTimedOut(true, globals.IdleTimeout)
		detail := s.a.endTaskResponse(s.ctx, s.tc, evergreen.TaskFailed, systemFailureDescription)
		s.True(detail.TimedOut)
		s.Equal(evergreen.TaskFailed, detail.Status)
		s.Equal(systemFailureDescription, detail.Description)
	})
	s.T().Run("TaskClearsIdleTimeoutAndFailsResultsInFailureWithoutTimeout", func(t *testing.T) {
		s.tc.setTimedOut(false, globals.IdleTimeout)
		defer s.tc.setTimedOut(false, "")
		detail := s.a.endTaskResponse(s.ctx, s.tc, evergreen.TaskFailed, systemFailureDescription)
		s.False(detail.TimedOut)
		s.Equal(evergreen.TaskFailed, detail.Status)
		s.Equal(systemFailureDescription, detail.Description)
	})
	s.T().Run("TaskClearsIdleTimeoutAndTheTaskAlreadyFinishedRunningResultsInSuccessWithoutTimeout", func(t *testing.T) {
		// Simulate a (rare) scenario where the idle timeout is reached, but the
		// last command in the main block already finished. It does record that
		// the timeout occurred, but the task commands nonetheless still
		// succeeded.
		s.tc.setTimedOut(true, globals.IdleTimeout)
		defer s.tc.setTimedOut(false, "")
		detail := s.a.endTaskResponse(s.ctx, s.tc, evergreen.TaskSucceeded, "")
		s.True(detail.TimedOut)
		s.Equal(evergreen.TaskSucceeded, detail.Status)
		s.Empty(detail.Description)
	})
}

func (s *AgentSuite) TestOOMTracker() {
	projYml := `
buildvariants:
 - name: mock_build_variant
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
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)
	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.True(s.mockCommunicator.EndTaskResult.Detail.OOMTracker.Detected)
	s.Equal(pids, s.mockCommunicator.EndTaskResult.Detail.OOMTracker.Pids)
}

func (s *AgentSuite) TestFinishPrevTaskWithoutTaskGroup() {
	const buildID = "build_id"
	const versionID = "not_a_task_group_version"
	tc := &taskContext{
		taskConfig: &internal.TaskConfig{
			Task: task.Task{
				Id:             "some_task_id",
				BuildId:        buildID,
				Version:        versionID,
				TaskOutputInfo: testutil.InitializeTaskOutput(s.T()),
			},
			WorkDir: "task_directory",
		},
		oomTracker:    &mock.OOMTracker{},
		logger:        s.tc.logger,
		ranSetupGroup: true,
	}
	nextTask := &apimodels.NextTaskResponse{
		TaskId:  "another_task_id",
		Build:   buildID,
		Version: versionID,
	}

	shouldSetupGroup, taskDirectory := s.a.finishPrevTask(s.ctx, nextTask, tc)

	s.True(shouldSetupGroup, "if the next task is not in a group, shouldSetupGroup should be true")
	s.Empty(taskDirectory)
}

func (s *AgentSuite) TestFinishPrevTaskAndNextTaskIsInNewTaskGroup() {
	const buildID = "build_id"
	const versionID = "not_a_task_group_version"
	tc := &taskContext{
		taskConfig: &internal.TaskConfig{
			Task: task.Task{
				Id:             "some_task_id",
				BuildId:        buildID,
				Version:        versionID,
				TaskOutputInfo: testutil.InitializeTaskOutput(s.T()),
			},
			WorkDir: "task_directory",
		},
		oomTracker:    &mock.OOMTracker{},
		logger:        s.tc.logger,
		ranSetupGroup: true,
	}
	nextTask := &apimodels.NextTaskResponse{
		TaskId:    "another_task_id",
		TaskGroup: "task_group_name",
		Build:     buildID,
		Version:   versionID,
	}

	shouldSetupGroup, taskDirectory := s.a.finishPrevTask(s.ctx, nextTask, tc)

	s.True(shouldSetupGroup, "if the next task is in a new task group, shouldSetupGroup should be true")
	s.Empty(taskDirectory)

}

func (s *AgentSuite) TestFinishPrevTaskWithSameTaskGroupAndAlreadyRanSetupGroup() {
	const taskGroup = "task_group_name"
	const versionID = "task_group_version"
	const buildID = "build_id"
	tc := &taskContext{
		taskConfig: &internal.TaskConfig{
			Task: task.Task{
				Id:             "some_task_id",
				TaskGroup:      taskGroup,
				BuildId:        buildID,
				Version:        versionID,
				TaskOutputInfo: testutil.InitializeTaskOutput(s.T()),
			},
			TaskGroup: &model.TaskGroup{Name: taskGroup},
			WorkDir:   "task_directory",
		},
		logger:        s.tc.logger,
		ranSetupGroup: true,
		oomTracker:    &mock.OOMTracker{},
	}
	nextTask := &apimodels.NextTaskResponse{
		TaskId:    "another_task_id",
		TaskGroup: taskGroup,
		Version:   versionID,
		Build:     buildID,
	}

	shouldSetupGroup, taskDirectory := s.a.finishPrevTask(s.ctx, nextTask, tc)

	s.False(shouldSetupGroup, "if the next task is in the same group as the previous task and we already ran the setup group, shouldSetupGroup should be false")
	s.Equal("task_directory", taskDirectory)
}

func (s *AgentSuite) TestFinishPrevTaskWithSameTaskGroupButDidNotRunSetupGroup() {
	const taskGroup = "task_group_name"
	const versionID = "task_group_version"
	const buildID = "build_id"
	tc := &taskContext{
		taskConfig: &internal.TaskConfig{
			Task: task.Task{
				Id:             "task_id1",
				TaskGroup:      taskGroup,
				Version:        versionID,
				BuildId:        buildID,
				TaskOutputInfo: testutil.InitializeTaskOutput(s.T()),
			},
			TaskGroup: &model.TaskGroup{Name: taskGroup},
			WorkDir:   "task_directory",
		},
		logger:     s.tc.logger,
		oomTracker: &mock.OOMTracker{},
	}
	nextTask := &apimodels.NextTaskResponse{
		TaskId:    "task_id2",
		TaskGroup: taskGroup,
		Version:   versionID,
		Build:     buildID,
	}

	shouldSetupGroup, taskDirectory := s.a.finishPrevTask(s.ctx, nextTask, tc)

	s.True(shouldSetupGroup, "if the next task is in the same group as the previous task but ranSetupGroup was false, shouldSetupGroup should be true")
	s.Empty(taskDirectory)

}

func (s *AgentSuite) TestFinishPrevTaskWithSameBuildButDifferentTaskGroup() {
	const taskGroup1 = "task_group_name"
	const versionID = "task_group_version"
	const buildID = "build_id"
	tc := &taskContext{
		taskConfig: &internal.TaskConfig{
			Task: task.Task{
				Id:             "task_id1",
				TaskGroup:      taskGroup1,
				Version:        versionID,
				BuildId:        buildID,
				TaskOutputInfo: testutil.InitializeTaskOutput(s.T()),
			},
			TaskGroup: &model.TaskGroup{Name: taskGroup1},
			WorkDir:   "task_directory",
		},
		logger:        s.tc.logger,
		ranSetupGroup: true,
		oomTracker:    &mock.OOMTracker{},
	}
	nextTask := &apimodels.NextTaskResponse{
		TaskId:    "task_id2",
		TaskGroup: "task_group2",
		Version:   versionID,
		Build:     buildID,
	}

	shouldSetupGroup, taskDirectory := s.a.finishPrevTask(s.ctx, nextTask, tc)

	s.True(shouldSetupGroup, "if the next task is in the same build but a different task group, shouldSetupGroup should be true")
	s.Empty(taskDirectory)
}

func (s *AgentSuite) TestFinishPrevTaskWithSameTaskGroupButDifferentTaskExecution() {
	const taskGroup1 = "task_group_name"
	const versionID = "task_group_version"
	const buildID = "build_id"
	tc := &taskContext{
		taskConfig: &internal.TaskConfig{
			Task: task.Task{
				Id:             "task_id1",
				Execution:      1,
				TaskGroup:      taskGroup1,
				Version:        versionID,
				BuildId:        buildID,
				TaskOutputInfo: testutil.InitializeTaskOutput(s.T()),
			},
			TaskGroup: &model.TaskGroup{Name: taskGroup1},
			WorkDir:   "task_directory",
		},
		logger:        s.tc.logger,
		ranSetupGroup: true,
		oomTracker:    &mock.OOMTracker{},
	}
	nextTask := &apimodels.NextTaskResponse{
		TaskId:        "task_id2",
		TaskExecution: 2,
		TaskGroup:     taskGroup1,
		Version:       versionID,
		Build:         buildID,
	}

	shouldSetupGroup, taskDirectory := s.a.finishPrevTask(s.ctx, nextTask, tc)

	s.True(shouldSetupGroup, "if the next task is in the same task group but has a different task execution number, shouldSetupGroup should be true")
	s.Empty(taskDirectory)
}

func (s *AgentSuite) TestGeneralPreviousTaskCleanupAndNextTaskSetupSucceeds() {
	nextTask := &apimodels.NextTaskResponse{}
	s.setupRunTask(defaultProjYml)
	shouldSetupGroup, taskDirectory := s.a.finishPrevTask(s.ctx, nextTask, s.tc)
	s.True(shouldSetupGroup, "should set up task directory again")
	s.Empty(taskDirectory, "task directory should not carry over to next task unless they're part of the same task group")
	tc, shouldExit, err := s.a.setupTask(s.ctx, s.ctx, s.tc, nextTask, shouldSetupGroup, taskDirectory)
	s.False(shouldExit)
	s.NoError(err)

	s.Require().NotZero(tc, "task context should be populated with initial data")
	s.Require().NotZero(tc.taskConfig)
	s.Equal(s.tc.taskConfig, tc.taskConfig)
	s.NotZero(tc.logger, "logger should be set")

	s.Contains(tc.taskConfig.WorkDir, s.a.opts.WorkingDirectory)
	taskDir := s.getTaskWorkingDirectory(s.a.opts.WorkingDirectory)
	s.NotZero(taskDir, "should have created task working directory")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Current command set to initial task setup (system).",
		"Making new directory",
		"Task logger initialized",
		"Execution logger initialized.",
		"System logger initialized.",
		"Starting task 'task_id', execution 0.",
	}, []string{panicLog})
}

func (s *AgentSuite) TestGeneralPreviousTaskCleanupAndNextTaskSetupSucceedsWithTasksInSameTaskGroup() {
	const taskGroup = "this_is_a_task_group"
	projYml := `
buildvariants:
  - name: mock_build_variant
    tasks:
      - this_is_a_task_group

tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: exit 0
  - name: this_is_another_task_name
    commands:
      - command: shell.exec
        params:
          script: exit 0

task_groups:
  - name: this_is_a_task_group
    tasks:
      - this_is_a_task_name
      - this_is_another_task_name
`
	s.setupRunTask(projYml)

	// Fake out the data so that the previous task already set up the task
	// group, and next task is part of the same task group.
	s.tc.ranSetupGroup = true
	s.tc.taskConfig.Task.TaskGroup = taskGroup
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)
	s.Require().NotNil(s.tc.taskConfig.TaskGroup, "task group should be defined in project")
	nextTask := &apimodels.NextTaskResponse{
		TaskGroup: taskGroup,
	}

	shouldSetupGroup, taskDirectory := s.a.finishPrevTask(s.ctx, nextTask, s.tc)
	s.False(shouldSetupGroup, "should not set up task directory again for task in same task group")
	s.Equal(s.tc.taskConfig.WorkDir, taskDirectory, "task directory should carry over to next task since it's part of the same task group")

	tc, shouldExit, err := s.a.setupTask(s.ctx, s.ctx, s.tc, nextTask, shouldSetupGroup, taskDirectory)
	s.False(shouldExit)
	s.NoError(err)

	s.Require().NotZero(tc)
	s.Require().NotZero(tc.taskConfig)
	s.Equal(taskDirectory, tc.taskConfig.WorkDir, "should reuse same working directory for task in same task group")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Current command set to initial task setup (system).",
		"Task logger initialized",
		"Execution logger initialized.",
		"System logger initialized.",
		"Starting task 'task_id', execution 0.",
	}, []string{
		panicLog,
		"Making new directory",
	})
}

func (s *AgentSuite) checkTaskSystemFailed() {
	s.Require().NotZero(s.mockCommunicator.EndTaskResult)
	detail := s.mockCommunicator.EndTaskResult.Detail
	s.Require().NotZero(detail)
	s.Equal(evergreen.TaskFailed, detail.Status, "task should fail")
	s.Equal(evergreen.CommandTypeSystem, detail.Type, "task should fail due to system failure")
	s.NotEmpty(detail.Description, "task failure description should be included")
}

func (s *AgentSuite) getTaskWorkingDirectory(baseDir string) fs.DirEntry {
	entries, err := os.ReadDir(baseDir)
	s.NoError(err)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.Contains(entry.Name(), taskLogDirectory) {
			// The task log directory is an unused logging directory, and is not
			// the task working directory.
			continue
		}
		if _, err := hex.DecodeString(entry.Name()); err != nil {
			// The task working directory name is always hex-encoded.
			continue
		}

		return entry
	}
	return nil
}

func (s *AgentSuite) TestSetupInitialWithTaskDataLoadingErrorResultsInSystemFailure() {
	const (
		taskID     = "task_id"
		taskName   = "task_name"
		taskSecret = "task_secret"
		buildID    = "build_id"
		versionID  = "version_id"
	)

	s.mockCommunicator.GetTaskResponse = &task.Task{
		Id:             taskID,
		DisplayName:    taskName,
		Secret:         taskSecret,
		BuildVariant:   "nonexistent_bv",
		BuildId:        buildID,
		Version:        versionID,
		TaskOutputInfo: testutil.InitializeTaskOutput(s.T()),
	}
	s.mockCommunicator.GetProjectResponse = &model.Project{
		Tasks: []model.ProjectTask{
			{
				Name: taskName,
			},
		},
	}
	ntr := &apimodels.NextTaskResponse{
		TaskId:     taskID,
		TaskSecret: taskSecret,
	}
	tc, shouldExit, err := s.a.setupTask(s.ctx, s.ctx, nil, ntr, true, "")
	s.Error(err, "setup.initial should error because task does not have a matching build variant in the project")
	s.False(shouldExit)

	s.Require().NotZero(tc, "task context should be populated with initial data")
	s.Empty(tc.taskConfig)
	s.NotZero(tc.logger)

	taskDir := s.getTaskWorkingDirectory(s.a.opts.WorkingDirectory)
	s.Zero(taskDir, "should not have created a task working directory")

	s.checkTaskSystemFailed()
}

func (s *AgentSuite) TestSetupInitialWithLoggingSetupErrorResultsInSystemFailure() {
	s.mockCommunicator.GetLoggerProducerShouldFail = true
	ntr := &apimodels.NextTaskResponse{
		TaskId:     s.task.Id,
		TaskSecret: s.task.Secret,
	}

	tc, shouldExit, err := s.a.setupTask(s.ctx, s.ctx, s.tc, ntr, true, "")
	s.Error(err, "setup.initial should error because logging setup errored")
	s.False(shouldExit)

	s.Require().NotZero(tc, "task context should be populated with initial data")
	s.Equal(s.tc.taskConfig, tc.taskConfig)
	s.NotZero(tc.logger)

	taskDir := s.getTaskWorkingDirectory(s.a.opts.WorkingDirectory)
	s.Zero(taskDir, "should not have created a task working directory")

	s.checkTaskSystemFailed()
}

func (s *AgentSuite) TestSetupInitialWithTaskDirectoryCreationErrorResultsInSystemFailure() {
	_, thisFile, _, _ := runtime.Caller(1)
	s.a.opts.WorkingDirectory = thisFile
	ntr := &apimodels.NextTaskResponse{
		TaskId:     s.task.Id,
		TaskSecret: s.task.Secret,
	}

	tc, shouldExit, err := s.a.setupTask(s.ctx, s.ctx, s.tc, ntr, true, "")
	s.Error(err, "setup.initial should error because task working directory could not be created")
	s.False(shouldExit)

	s.Require().NotZero(tc, "task context should be populated with initial data")
	s.Equal(s.tc.taskConfig, tc.taskConfig)
	s.NotZero(tc.logger)

	fileInfo, err := os.Stat(thisFile)
	s.NoError(err)
	s.False(fileInfo.IsDir(), "cannot use file as prefix path for task working directory")

	s.checkTaskSystemFailed()
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
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)
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
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)
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
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)
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
	projYml := `
task_groups:
  - name: task_group_name
    setup_group:
      - command: shell.exec
        params:
          script: exit 0
`

	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc))
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running setup-group commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Finished running setup-group commands",
	}, []string{
		panicLog,
		"Running setup-group commands failed",
	})
}

func (s *AgentSuite) TestSetupGroupFails() {
	const taskGroup = "task_group_name"
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
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	s.Error(s.a.runPreTaskCommands(s.ctx, s.tc), "setup group command error should fail task")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running setup-group commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Running setup-group commands failed",
		"Finished running setup-group commands",
	}, []string{panicLog})
}

func (s *AgentSuite) TestSetupGroupTimeoutDoesNotFailTask() {
	const taskGroup = "task_group_name"
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
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	startAt := time.Now()
	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc), "setup group timeout should not fail task")

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.False(s.tc.hadTimedOut(), "should not have hit task timeout")
	s.Zero(s.tc.getTimeoutType())
	s.Zero(s.tc.getTimeoutDuration())
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running setup-group commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Hit setup_group timeout (1s)",
		"Finished command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Running setup-group commands failed",
		"Finished running setup-group commands",
	}, []string{
		panicLog,
	})
}

func (s *AgentSuite) TestSetupGroupTimeoutFailsTask() {
	const taskGroup = "task_group_name"
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
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	startAt := time.Now()
	err := s.a.runPreTaskCommands(s.ctx, s.tc)
	s.Error(err, "setup group timeout should fail task")
	s.True(utility.IsContextError(errors.Cause(err)))

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.True(s.tc.hadTimedOut(), "should have hit task timeout")
	s.Equal(globals.SetupGroupTimeout, s.tc.getTimeoutType())
	s.Equal(time.Second, s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running setup-group commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Hit setup_group timeout (1s)",
		"Finished command 'shell.exec' (step 1 of 1) in block 'setup_group'",
		"Running setup-group commands failed",
		"Finished running setup-group commands",
	}, []string{panicLog})
}

func (s *AgentSuite) TestSetupTaskSucceeds() {
	const taskGroup = "task_group_name"
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
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running setup-task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Finished running setup-task commands",
	}, []string{
		panicLog,
		"Running setup-task commands failed",
	})
}

func (s *AgentSuite) TestSetupTaskFails() {
	const taskGroup = "task_group_name"
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
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	s.Error(s.a.runPreTaskCommands(s.ctx, s.tc), "setup task command error should fail task")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running setup-task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Running setup-task commands failed",
		"Finished running setup-task commands",
	}, []string{panicLog})
}

func (s *AgentSuite) TestSetupTaskTimeoutDoesNotFailTask() {
	const taskGroup = "task_group_name"
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
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	startAt := time.Now()
	s.NoError(s.a.runPreTaskCommands(s.ctx, s.tc), "setup task timeout should not fail task")

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.False(s.tc.hadTimedOut(), "should not have hit task timeout")
	s.Zero(s.tc.getTimeoutType())
	s.Zero(s.tc.getTimeoutDuration())
	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running setup-task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Hit setup_task timeout (1s)",
		"Finished command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Running setup-task commands failed",
		"Finished running setup-task commands",
	}, []string{
		panicLog,
	})
}

func (s *AgentSuite) TestSetupTaskTimeoutFailsTask() {
	const taskGroup = "task_group_name"
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
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	startAt := time.Now()
	err := s.a.runPreTaskCommands(s.ctx, s.tc)
	s.Error(err, "setup task timeout should fail task")
	s.True(utility.IsContextError(errors.Cause(err)))

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.True(s.tc.hadTimedOut(), "should have hit task timeout")
	s.Equal(globals.SetupTaskTimeout, s.tc.getTimeoutType())
	s.Equal(time.Second, s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running setup-task commands",
		"Set idle timeout for 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Running command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Hit setup_task timeout (1s)",
		"Finished command 'shell.exec' (step 1 of 1) in block 'setup_task'",
		"Running setup-task commands failed",
		"Finished running setup-task commands",
	}, []string{panicLog})
}

func (s *AgentSuite) TestTeardownTaskSucceeds() {
	taskGroup := "task_group_name"
	projYml := `
task_groups:
  - name: task_group_name
    teardown_task:
      - command: shell.exec
        params:
          script: exit 0
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	s.NoError(s.a.runPostOrTeardownTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running teardown-task commands",
		"Setting heartbeat timeout to type 'teardown_task'",
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Resetting heartbeat timeout from type 'teardown_task' back to default",
		"Finished running teardown-task commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec'",
		"Running setup-task commands failed",
	})
}

func (s *AgentSuite) TestTeardownTaskFails() {
	const taskGroup = "task_group_name"
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
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	s.Error(s.a.runPostOrTeardownTaskCommands(s.ctx, s.tc))

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running teardown-task commands",
		"Setting heartbeat timeout to type 'teardown_task'",
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Running teardown-task commands failed",
		"Resetting heartbeat timeout from type 'teardown_task' back to default",
		"Finished running teardown-task commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec'",
	})
}

func (s *AgentSuite) TestTeardownTaskTimeoutDoesNotFailTask() {
	const taskGroup = "task_group_name"
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
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	startAt := time.Now()
	s.NoError(s.a.runPostOrTeardownTaskCommands(s.ctx, s.tc), "teardown task timeout should not fail task")

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.False(s.tc.hadTimedOut(), "should not have hit task timeout")
	s.Zero(s.tc.getTimeoutType())
	s.Zero(s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running teardown-task commands",
		"Setting heartbeat timeout to type 'teardown_task'",
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Setting heartbeat timeout to type 'teardown_task'",
		"Hit teardown_task timeout (1s)",
		"Finished command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Running teardown-task commands failed",
		"Resetting heartbeat timeout from type 'teardown_task' back to default",
		"Finished running teardown-task commands",
	}, []string{
		panicLog,
	})
}

func (s *AgentSuite) TestTeardownTaskTimeoutFailsTask() {
	const taskGroup = "task_group_name"
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
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	startAt := time.Now()
	err := s.a.runPostOrTeardownTaskCommands(s.ctx, s.tc)
	s.Error(err, "teardown task timeout should fail task")
	s.True(utility.IsContextError(errors.Cause(err)))

	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")
	s.True(s.tc.hadTimedOut(), "should have hit task timeout")
	s.Equal(globals.TeardownTaskTimeout, s.tc.getTimeoutType())
	s.Equal(time.Second, s.tc.getTimeoutDuration())

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running teardown-task commands",
		"Setting heartbeat timeout to type 'teardown_task'",
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Setting heartbeat timeout to type 'teardown_task'",
		"Hit teardown_task timeout (1s)",
		"Finished command 'shell.exec' (step 1 of 1) in block 'teardown_task'",
		"Running teardown-task commands failed",
		"Resetting heartbeat timeout from type 'teardown_task' back to default",
		"Finished running teardown-task commands",
	}, []string{panicLog})
}

func (s *AgentSuite) TestTeardownGroupSucceeds() {
	taskGroup := "task_group_name"
	projYml := `
task_groups:
  - name: task_group_name
    teardown_group:
      - command: shell.exec
        params:
          script: exit 0
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	s.a.runTeardownGroupCommands(s.ctx, s.tc)

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running teardown-group commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_group'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'teardown_group'",
		"Finished running teardown-group commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec'",
		"Running teardown-group commands failed",
	})
}

func (s *AgentSuite) TestTeardownGroupTimeout() {
	const taskGroup = "task_group_name"
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
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	startAt := time.Now()
	s.a.runTeardownGroupCommands(s.ctx, s.tc)
	s.Less(time.Since(startAt), 5*time.Second, "timeout should have triggered after 1s")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running teardown-group commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'teardown_group'",
		"Hit teardown_group timeout (1s)",
		"Finished command 'shell.exec' (step 1 of 1) in block 'teardown_group'",
		"Running teardown-group commands failed",
		"Finished running teardown-group commands",
	}, []string{
		panicLog,
	})
}

func (s *AgentSuite) TestTaskGroupTimeout() {
	const taskGroup = "task_group_name"
	s.tc.task = client.TaskData{
		ID:     "task_id",
		Secret: "task_secret",
	}
	projYml := `
task_groups:
  - name: task_group_name
    timeout:
      - command: shell.exec
        params:
          script: exit 0
`
	s.setupRunTask(projYml)
	s.tc.taskConfig.Task.TaskGroup = taskGroup
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	s.a.runTaskTimeoutCommands(s.ctx, s.tc)

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Running task-timeout commands",
		"Running command 'shell.exec' (step 1 of 1) in block 'timeout'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'timeout'",
		"Finished running task-timeout commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec'",
		"Running task-timeout commands failed",
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
		"Setting heartbeat timeout to type 'callback'",
		"Running command 'shell.exec' (step 1 of 1) in block 'timeout'",
		"Hit callback timeout (1s)",
		"Finished command 'shell.exec' (step 1 of 1) in block 'timeout'",
		"Running task-timeout commands failed",
		"Resetting heartbeat timeout from type 'callback' back to default",
		"Finished running task-timeout commands",
	}, []string{
		panicLog,
		"Set idle timeout for 'shell.exec'",
	})
}

func (s *AgentSuite) TestFetchTaskInfo() {
	s.mockCommunicator.GetProjectResponse = &model.Project{
		Identifier: "some_cool_project",
	}

	_, project, expansionsAndVars, err := s.a.fetchTaskInfo(s.ctx, s.tc)
	s.NoError(err)

	s.Require().NotZero(s.tc.taskConfig.Project)
	s.Equal(s.mockCommunicator.GetProjectResponse.Identifier, project.Identifier)
	s.Require().NotZero(expansionsAndVars.Expansions)
	s.Equal("bar", expansionsAndVars.Expansions["foo"], "should include mock communicator expansions")
	s.Equal("new-parameter-value", expansionsAndVars.Expansions["overwrite-this-parameter"], "user-specified parameter should overwrite any other conflicting expansion")
	s.Require().NotZero(expansionsAndVars.PrivateVars)
	s.True(expansionsAndVars.PrivateVars["some_private_var"], "should include mock communicator private variables")
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
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)
	s.NoError(err)

	s.WithinDuration(start, time.Now(), 4*time.Second, "abort should prevent commands in the main block from continuing to run")
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status, "task that aborts during main block should fail")
	// The exact count is not of particular importance, we're only interested in
	// knowing that the heartbeat is still going despite receiving an abort.
	s.GreaterOrEqual(s.mockCommunicator.GetHeartbeatCount(), 1, "heartbeat should be still running for teardown_task block even when initial abort signal is received")

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Heartbeat received signal to abort task.",
		"Task completed - FAILURE",
		"Running post-task commands",
		"Setting heartbeat timeout to type 'post'",
		"Running command 'shell.exec' (step 1 of 1) in block 'post'",
		"Finished command 'shell.exec' (step 1 of 1) in block 'post'",
		"Resetting heartbeat timeout from type 'post' back to default",
		"Finished running post-task commands",
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
	taskGroup := "some_task_group"
	s.tc.taskConfig.Task.TaskGroup = taskGroup
	s.tc.taskConfig.TaskGroup = s.tc.taskConfig.Project.FindTaskGroup(taskGroup)

	start := time.Now()
	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
		TaskGroup:  taskGroup,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)
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

func (s *AgentSuite) TestUpsertCheckRun() {
	s.setupRunTask(defaultProjYml)

	f, err := os.CreateTemp(os.TempDir(), "")
	s.NoError(err)
	defer os.Remove(f.Name())

	outputString := `
	{
	        "title": "This is my report ${checkRun_key}",
	        "summary": "We found 6 failures and 2 warnings",
	        "text": "It looks like there are some errors on lines 2 and 4.",
	        "annotations": [
	            {
	                "path": "README.md",
	                "annotation_level": "warning",
	                "title": "Error Detector",
	                "message": "message",
	                "raw_details": "Do you mean this other thing?",
	                "start_line": 2,
	                "end_line": 4
	            }
	        ]
	}
	`
	_, err = f.WriteString(outputString)
	s.NoError(err)
	s.NoError(f.Close())

	s.tc.taskConfig.Task.CheckRunPath = utility.ToStringPtr(f.Name())
	s.tc.taskConfig.Task.Requester = evergreen.GithubPRRequester

	s.tc.taskConfig.Expansions.Put("checkRun_key", "checkRun_value")
	checkRunOutput, err := buildCheckRun(s.ctx, s.tc)
	s.NoError(err)
	s.NotNil(checkRunOutput)
	s.Equal(checkRunOutput.Title, "This is my report checkRun_value")
	s.Equal(checkRunOutput.Summary, "We found 6 failures and 2 warnings")
	s.Equal(checkRunOutput.Text, "It looks like there are some errors on lines 2 and 4.")
	s.Assert().Len(checkRunOutput.Annotations, 1)
	s.Equal(checkRunOutput.Annotations[0].Path, "README.md")
	s.Equal(checkRunOutput.Annotations[0].AnnotationLevel, "warning")
	s.Equal(checkRunOutput.Annotations[0].Title, "Error Detector")
	s.Equal(checkRunOutput.Annotations[0].Message, "message")
	s.Equal(checkRunOutput.Annotations[0].RawDetails, "Do you mean this other thing?")
	s.Equal(checkRunOutput.Annotations[0].StartLine, utility.ToIntPtr(2))
	s.Equal(checkRunOutput.Annotations[0].EndLine, utility.ToIntPtr(4))
}

func (s *AgentSuite) TestUpsertEmptyCheckRun() {
	s.setupRunTask(defaultProjYml)

	f, err := os.CreateTemp(os.TempDir(), "")
	s.NoError(err)
	defer os.Remove(f.Name())

	s.tc.taskConfig.Task.CheckRunPath = utility.ToStringPtr("")
	s.tc.taskConfig.Task.Requester = evergreen.GithubPRRequester

	checkRunOutput, err := buildCheckRun(s.ctx, s.tc)
	s.NoError(err)
	s.NotNil(checkRunOutput)

	s.NoError(s.tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, s.tc.taskConfig.Task.Id, []string{
		"Upserting check run with no output file specified.",
	}, []string{panicLog})
}

func (s *AgentSuite) TestTaskOutputDirectoryTestLogs() {
	projYml := `
tasks:
  - name: this_is_a_task_name
    commands:
      - command: shell.exec
        params:
          script: |
            cat >> build/TestLogs/test.log <<EOF
            I am test log.
            I should get ingested automatically by the agent.
            And stored as well.
            EOF
`
	s.setupRunTask(projYml)
	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
	}
	_, _, err := s.a.runTask(s.ctx, s.tc, nextTask, false, s.testTmpDirName)
	s.Require().NoError(err)

	it, err := s.task.GetTestLogs(s.ctx, taskoutput.TestLogGetOptions{LogPaths: []string{"test.log"}})
	s.Require().NoError(err)

	var actualLines string
	for it.Next() {
		actualLines += it.Item().Data + "\n"
	}
	expectedLines := "I am test log.\nI should get ingested automatically by the agent.\nAnd stored as well.\n"
	s.Equal(expectedLines, actualLines)
}

func (s *AgentSuite) TestShouldRunSetupGroup() {
	nextTask := &apimodels.NextTaskResponse{
		TaskGroup: "",
		TaskId:    "task1",
	}
	tc := &taskContext{
		taskConfig: &internal.TaskConfig{
			Task: task.Task{
				Execution: 0,
				BuildId:   "build1",
			},
			TaskGroup: &model.TaskGroup{
				Name: "group1",
			},
		},
		ranSetupGroup: false,
	}

	shouldRun := shouldRunSetupGroup(nextTask, tc)
	s.Equal(true, shouldRun)

	tc.ranSetupGroup = true

	shouldRun = shouldRunSetupGroup(nextTask, &taskContext{})
	s.Equal(true, shouldRun)

	shouldRun = shouldRunSetupGroup(nextTask, tc)
	s.Equal(true, shouldRun)

	nextTask.TaskGroup = "not same"
	shouldRun = shouldRunSetupGroup(nextTask, tc)
	s.Equal(true, shouldRun)

	nextTask.Build = "build1"
	shouldRun = shouldRunSetupGroup(nextTask, tc)
	s.Equal(true, shouldRun)

	nextTask.TaskGroup = "group1"
	shouldRun = shouldRunSetupGroup(nextTask, tc)
	s.Equal(false, shouldRun)

	nextTask.TaskExecution = 1
	shouldRun = shouldRunSetupGroup(nextTask, tc)
	s.Equal(true, shouldRun)

	tc.taskConfig.Task.Execution = 1
	shouldRun = shouldRunSetupGroup(nextTask, tc)
	s.Equal(false, shouldRun)

	tc.taskConfig.Task.Execution = 2
	shouldRun = shouldRunSetupGroup(nextTask, tc)
	s.Equal(false, shouldRun)
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
	for _, line := range mc.GetTaskLogs(taskID) {
		for log := range expectedLog {
			if strings.Contains(line.Data, log) {
				expectedLog[log] = true
			}
		}
		for log := range unexpectedLog {
			if strings.Contains(line.Data, log) {
				unexpectedLog[log] = true
			}
		}
		allLogs = append(allLogs, line.Data)
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
