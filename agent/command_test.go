package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
)

type CommandSuite struct {
	suite.Suite
	a                *Agent
	mockCommunicator *client.Mock
	tmpDirName       string
	tc               *taskContext
}

func TestCommandSuite(t *testing.T) {
	suite.Run(t, new(CommandSuite))
}

func (s *CommandSuite) SetupTest() {
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

	var err error
	s.tmpDirName = s.T().TempDir()
	s.a.jasper, err = jasper.NewSynchronizedManager(false)
	s.Require().NoError(err)

	s.tc = &taskContext{
		task: client.TaskData{
			Secret: "mock_task_secret",
		},
		taskModel:  &task.Task{},
		oomTracker: &mock.OOMTracker{},
	}
}

func (s *CommandSuite) TestPreErrorFailsWithSetup() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "pre_error"
	s.tc.task.ID = taskID
	s.tc.ranSetupGroup = false

	defer s.a.removeTaskDirectory(s.tc)
	_, err := s.a.runTask(ctx, s.tc)
	s.NoError(err)
	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal(evergreen.CommandTypeSetup, detail.Type)
	s.Contains(detail.Description, "shell.exec")
	s.False(detail.TimedOut)

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(s.tc.task.Secret, taskData.Secret)
}

func (s *CommandSuite) TestShellExec() {
	f, err := os.CreateTemp(s.tmpDirName, "shell-exec-")
	s.Require().NoError(err)
	defer os.Remove(f.Name())

	tmpFile := f.Name()
	s.mockCommunicator.ShellExecFilename = tmpFile
	s.Require().NoError(f.Close())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "shellexec"
	s.tc.task.ID = taskID
	s.tc.ranSetupGroup = false

	s.NoError(s.a.startLogging(ctx, s.tc))
	defer s.a.removeTaskDirectory(s.tc)
	_, err = s.a.runTask(ctx, s.tc)
	s.NoError(err)

	s.Require().NoError(s.tc.logger.Close())

	checkMockLogs(s.T(), s.mockCommunicator, taskID,
		"Task completed - SUCCESS",
		"Finished command 'shell.exec' in function 'foo' (step 1 of 1)",
	)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal("success", detail.Status)
	s.Equal("test", detail.Type)
	s.Contains(detail.Description, "shell.exec")
	s.False(detail.TimedOut)

	data, err := os.ReadFile(tmpFile)
	s.Require().NoError(err)
	s.Equal("shell.exec test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(s.tc.task.Secret, taskData.Secret)
}

func TestEndTaskSyncCommands(t *testing.T) {
	s3PushFound := func(cmds *model.YAMLCommandSet) bool {
		for _, cmd := range cmds.List() {
			if cmd.Command == evergreen.S3PushCommandName {
				return true
			}
		}
		return false
	}
	for testName, testCase := range map[string]func(t *testing.T, tc *taskContext, detail *apimodels.TaskEndDetail){
		"ReturnsTaskSyncCommands": func(t *testing.T, tc *taskContext, detail *apimodels.TaskEndDetail) {
			cmds := endTaskSyncCommands(tc, detail)
			require.NotNil(t, cmds)
			assert.True(t, s3PushFound(cmds))
		},
		"ReturnsNoCommandsForNoSync": func(t *testing.T, tc *taskContext, detail *apimodels.TaskEndDetail) {
			tc.taskModel.SyncAtEndOpts.Enabled = false
			assert.Nil(t, endTaskSyncCommands(tc, detail))
		},
		"ReturnsCommandsIfMatchesTaskStatus": func(t *testing.T, tc *taskContext, detail *apimodels.TaskEndDetail) {
			detail.Status = evergreen.TaskSucceeded
			tc.taskModel.SyncAtEndOpts.Statuses = []string{evergreen.TaskSucceeded}
			cmds := endTaskSyncCommands(tc, detail)
			require.NotNil(t, cmds)
			assert.True(t, s3PushFound(cmds))
		},
		"ReturnsNoCommandsIfDoesNotMatchTaskStatus": func(t *testing.T, tc *taskContext, detail *apimodels.TaskEndDetail) {
			detail.Status = evergreen.TaskSucceeded
			tc.taskModel.SyncAtEndOpts.Statuses = []string{evergreen.TaskFailed}
			cmds := endTaskSyncCommands(tc, detail)
			assert.Nil(t, cmds)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tc := &taskContext{
				taskModel: &task.Task{
					SyncAtEndOpts: task.SyncAtEndOptions{Enabled: true},
				},
			}
			detail := &apimodels.TaskEndDetail{}
			testCase(t, tc, detail)
		})
	}
}
