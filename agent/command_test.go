package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
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
	ctx              context.Context
	cancel           context.CancelFunc
}

func TestCommandSuite(t *testing.T) {
	suite.Run(t, new(CommandSuite))
}

func (s *CommandSuite) TearDownTest() {
	s.cancel()
}

func (s *CommandSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	var err error
	s.tmpDirName = s.T().TempDir()

	s.a = &Agent{
		opts: Options{
			HostID:           "host",
			HostSecret:       "secret",
			StatusPort:       2286,
			LogOutput:        LogOutputStdout,
			LogPrefix:        "agent",
			WorkingDirectory: s.tmpDirName,
		},
		comm:   client.NewMock("url"),
		tracer: otel.GetTracerProvider().Tracer("noop_tracer"),
	}
	s.mockCommunicator = s.a.comm.(*client.Mock)

	s.a.jasper, err = jasper.NewSynchronizedManager(false)
	s.Require().NoError(err)

	s.tc = &taskContext{
		task: client.TaskData{
			ID:     "mock_task_id",
			Secret: "mock_task_secret",
		},
		oomTracker: &mock.OOMTracker{},
	}
}

func (s *CommandSuite) TestPreErrorFailsWithSetup() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// This task ID signifies that the mock communicator should load the
	// <task_id>.yaml file as the project YAML.
	taskID := "pre_error"
	s.tc.task.ID = taskID
	s.tc.ranSetupGroup = false

	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
	}

	_, _, err := s.a.runTask(ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tmpDirName)
	s.NoError(err)
	s.NoError(s.tc.logger.Close())
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

	// This task ID signifies that the mock communicator should load the
	// <task_id>.yaml file as the project YAML.
	taskID := "shellexec"
	s.tc.task.ID = taskID
	s.tc.ranSetupGroup = false

	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
	}

	_, _, err = s.a.runTask(ctx, s.tc, nextTask, !s.tc.ranSetupGroup, s.tmpDirName)
	s.NoError(err)

	s.Require().NoError(s.tc.logger.Close())

	checkMockLogs(s.T(), s.mockCommunicator, taskID, []string{
		"Task completed - SUCCESS",
		"Finished command 'shell.exec' in function 'foo' (step 1 of 1)",
	}, nil)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal(evergreen.TaskSucceeded, detail.Status)
	s.Zero(detail.Type, "should not include failure command type for successful task")
	s.Zero(detail.Description, "should not include failure description for successful task")
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
			tc.taskConfig.Task.SyncAtEndOpts.Enabled = false
			assert.Nil(t, endTaskSyncCommands(tc, detail))
		},
		"ReturnsCommandsIfMatchesTaskStatus": func(t *testing.T, tc *taskContext, detail *apimodels.TaskEndDetail) {
			detail.Status = evergreen.TaskSucceeded
			tc.taskConfig.Task.SyncAtEndOpts.Statuses = []string{evergreen.TaskSucceeded}
			cmds := endTaskSyncCommands(tc, detail)
			require.NotNil(t, cmds)
			assert.True(t, s3PushFound(cmds))
		},
		"ReturnsNoCommandsIfDoesNotMatchTaskStatus": func(t *testing.T, tc *taskContext, detail *apimodels.TaskEndDetail) {
			detail.Status = evergreen.TaskSucceeded
			tc.taskConfig.Task.SyncAtEndOpts.Statuses = []string{evergreen.TaskFailed}
			cmds := endTaskSyncCommands(tc, detail)
			assert.Nil(t, cmds)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			const taskID = "task_id"
			comm := client.NewMock("url")
			tsk := task.Task{
				Id:            taskID,
				SyncAtEndOpts: task.SyncAtEndOptions{Enabled: true},
			}
			td := client.TaskData{ID: taskID, Secret: "secret"}
			logger, err := comm.GetLoggerProducer(ctx, td, nil)
			require.NoError(t, err)
			tc := &taskContext{
				taskConfig: &internal.TaskConfig{
					Task: tsk,
				},
				logger: logger,
			}
			detail := &apimodels.TaskEndDetail{}
			testCase(t, tc, detail)
		})
	}
}

func (s *CommandSuite) setUpConfigAndProject(projYml string) {
	config := &internal.TaskConfig{
		Expansions: util.Expansions{"key1": "expansionVar", "key2": "expansionVar2", "key3": "expansionVar3"},
		BuildVariant: model.BuildVariant{
			Name: "some_build_variant",
		},
		Task: task.Task{
			Id:           "task_id",
			DisplayName:  "some task",
			BuildVariant: "some_build_variant",
			Version:      "v1",
		},
	}
	s.tc.taskConfig = config
	p := model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", &p)
	s.NoError(err)
	s.tc.taskConfig.Project = p

	s.tc.logger, err = s.mockCommunicator.GetLoggerProducer(s.ctx, s.tc.task, nil)
	s.NoError(err)
	s.tc.taskConfig.Project = p
}

func (s *CommandSuite) TestVarsAreUnsetAfterRunning() {
	projYml := `
functions:
  yes:
    vars: 
      key1: "functionVar"
    command: shell.exec
    params:
        shell: bash
        script: |
          echo "hi"
`

	s.setUpConfigAndProject(projYml)

	func1 := model.PluginCommandConf{
		Function:    "yes",
		DisplayName: "function",
		Vars:        map[string]string{"key1": "functionVar"},
	}

	cmdBlock := commandBlock{
		commands: &model.YAMLCommandSet{SingleCommand: &func1},
	}
	err := s.a.runCommandsInBlock(s.ctx, s.tc, cmdBlock)
	s.NoError(err)

	key1Value := s.tc.taskConfig.Expansions.Get("key1")
	s.Equal("expansionVar", key1Value, "globalVar should be set back to what it was before the function ran")
}

func (s *CommandSuite) TestVarsUnsetPreserveExpansionUpdates() {
	projYml := `
functions:
  yes:
    command: expansions.update
    params:
      updates: 
      - key: key1
        value: ${key1}
      - key: key2
        value: ${key2}
`
	s.setUpConfigAndProject(projYml)

	func1 := model.PluginCommandConf{
		Function:    "yes",
		DisplayName: "function",
		Vars:        map[string]string{"key1": "functionVar1", "key2": "functionVar2", "key3": "functionVar3"},
	}

	cmdBlock := commandBlock{
		commands: &model.YAMLCommandSet{SingleCommand: &func1},
	}
	err := s.a.runCommandsInBlock(s.ctx, s.tc, cmdBlock)
	s.NoError(err)

	key1Value := s.tc.taskConfig.Expansions.Get("key1")
	s.Equal("functionVar1", key1Value, "key1 should be set to what it was updated to with expansions.update")

	key2Value := s.tc.taskConfig.Expansions.Get("key2")
	s.Equal("functionVar2", key2Value, "key2 should be set to what it was updated to with expansions.update")

	key3Value := s.tc.taskConfig.Expansions.Get("key3")
	s.Equal("expansionVar3", key3Value, "key3 should be the original expansion value")

	s.Empty(s.tc.taskConfig.DynamicExpansions)
}

func (s *CommandSuite) TestVarsUnsetPreserveExpansionUpdatesFromFile() {
	projYml := `
functions:
  yes:
    command: expansions.update
    params:
      file: command/testdata/git/test_expansions.yml
`
	s.setUpConfigAndProject(projYml)

	func1 := model.PluginCommandConf{
		Function:    "yes",
		DisplayName: "function",
		Vars:        map[string]string{"key1": "newValue1", "key2": "newValue2", "key3": "newValue3"},
	}

	cmdBlock := commandBlock{
		commands: &model.YAMLCommandSet{SingleCommand: &func1},
	}
	err := s.a.runCommandsInBlock(s.ctx, s.tc, cmdBlock)
	s.NoError(err)

	key1Value := s.tc.taskConfig.Expansions.Get("key1")
	s.Equal("newValue1", key1Value, "key1 should be set to what it was updated to with expansions.update")

	key2Value := s.tc.taskConfig.Expansions.Get("key2")
	s.Equal("newValue2", key2Value, "key2 should be set to what it was updated to with expansions.update")

	key3Value := s.tc.taskConfig.Expansions.Get("key3")
	s.Equal("expansionVar3", key3Value, "key3 should be the original expansion value")
	s.Empty(s.tc.taskConfig.DynamicExpansions)
}
