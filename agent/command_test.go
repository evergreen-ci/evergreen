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
	"github.com/evergreen-ci/evergreen/model/patch"
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

	const bvName = "some_build_variant"
	tsk := &task.Task{
		Id:           "task_id",
		DisplayName:  "some task",
		BuildVariant: bvName,
		Version:      "v1",
	}

	project := &model.Project{
		Tasks: []model.ProjectTask{
			{
				Name: tsk.DisplayName,
			},
		},
		BuildVariants: []model.BuildVariant{{Name: bvName}},
	}

	taskConfig, err := internal.NewTaskConfig(s.tmpDirName, &apimodels.DistroView{}, project, tsk, &model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
	}, &patch.Patch{}, util.Expansions{})
	s.Require().NoError(err)

	s.tc = &taskContext{
		taskConfig: taskConfig,
		task: client.TaskData{
			Secret: "mock_task_secret",
		},
		taskModel:                 &task.Task{},
		oomTracker:                &mock.OOMTracker{},
		unsetFunctionVarsDisabled: false,
	}
}

func (s *CommandSuite) TestPreErrorFailsWithSetup() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "pre_error"
	s.tc.task.ID = taskID
	s.tc.ranSetupGroup = false

	defer s.a.removeTaskDirectory(s.tc)
	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
		TaskGroup:  s.tc.taskGroup,
	}
	shouldSetupGroup := !s.tc.ranSetupGroup
	taskDirectory := s.tc.taskDirectory
	_, _, err := s.a.runTask(ctx, s.tc, nextTask, shouldSetupGroup, taskDirectory)
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
	nextTask := &apimodels.NextTaskResponse{
		TaskId:     s.tc.task.ID,
		TaskSecret: s.tc.task.Secret,
		TaskGroup:  s.tc.taskGroup,
	}
	shouldSetupGroup := !s.tc.ranSetupGroup
	taskDirectory := s.tc.taskDirectory
	_, _, err = s.a.runTask(ctx, s.tc, nextTask, shouldSetupGroup, taskDirectory)
	s.NoError(err)

	s.Require().NoError(s.tc.logger.Close())

	checkMockLogs(s.T(), s.mockCommunicator, taskID, []string{
		"Task completed - SUCCESS",
		"Finished command 'shell.exec' in function 'foo' (step 1 of 1)",
	}, nil)

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

func (s *CommandSuite) setUpConfigAndProject(projYml string) {
	config := &internal.TaskConfig{
		Expansions: &util.Expansions{"key1": "expansionVar", "key2": "expansionVar2", "key3": "expansionVar3"},
		BuildVariant: &model.BuildVariant{
			Name: "some_build_variant",
		},
		Task: &task.Task{
			Id:           "task_id",
			DisplayName:  "some task",
			BuildVariant: "some_build_variant",
			Version:      "v1",
		},
		Timeout: &internal.Timeout{},
	}
	s.tc.taskConfig = config
	p := &model.Project{}
	_, err := model.LoadProjectInto(s.ctx, []byte(projYml), nil, "", p)
	s.NoError(err)
	s.tc.taskConfig.Project = p

	s.tc.logger, err = s.mockCommunicator.GetLoggerProducer(s.ctx, s.tc.task, nil)
	s.NoError(err)
	s.tc.project = p
	s.tc.taskConfig.Project = p
}

func (s *CommandSuite) TestFunctionVarsUnsetWhenProjectFlagTrue() {
	s.tc.unsetFunctionVarsDisabled = true
	projYml := `
unset_function_vars: true 
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

	cmds := []model.PluginCommandConf{func1}
	err := s.a.runCommandsInBlock(s.ctx, s.tc, cmds, runCommandsOptions{}, "")
	s.NoError(err)

	key1Value := s.tc.taskConfig.Expansions.Get("key1")
	s.Equal("expansionVar", key1Value, "globalVar should be set back to what it was before the function ran")

}

func (s *CommandSuite) TestFunctionVarsDontUnsetWithoutFlag() {
	s.tc.unsetFunctionVarsDisabled = true

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
	func1 := model.PluginCommandConf{
		Function:    "yes",
		DisplayName: "function",
		Vars:        map[string]string{"key1": "functionVar"},
	}

	cmds := []model.PluginCommandConf{func1}
	s.setUpConfigAndProject(projYml)
	err := s.a.runCommandsInBlock(s.ctx, s.tc, cmds, runCommandsOptions{}, "")
	s.NoError(err)

	key1Value := s.tc.taskConfig.Expansions.Get("key1")
	s.Equal("functionVar", key1Value, "globalVar should not be set back to what it was before if it's disabled")
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

	cmds := []model.PluginCommandConf{func1}
	err := s.a.runCommandsInBlock(s.ctx, s.tc, cmds, runCommandsOptions{}, "")
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

	cmds := []model.PluginCommandConf{func1}
	err := s.a.runCommandsInBlock(s.ctx, s.tc, cmds, runCommandsOptions{}, "")
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

	cmds := []model.PluginCommandConf{func1}
	err := s.a.runCommandsInBlock(s.ctx, s.tc, cmds, runCommandsOptions{}, "")
	s.NoError(err)

	key1Value := s.tc.taskConfig.Expansions.Get("key1")
	s.Equal("newValue1", key1Value, "key1 should be set to what it was updated to with expansions.update")

	key2Value := s.tc.taskConfig.Expansions.Get("key2")
	s.Equal("newValue2", key2Value, "key2 should be set to what it was updated to with expansions.update")

	key3Value := s.tc.taskConfig.Expansions.Get("key3")
	s.Equal("expansionVar3", key3Value, "key3 should be the original expansion value")
	s.Empty(s.tc.taskConfig.DynamicExpansions)
}
