package agent

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSenderLocal(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := (&Agent{}).GetSender(ctx, evergreen.LocalLoggingOverride)
	assert.NoError(err)
}

func TestCommandFileLogging(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	tmpDirName, err := ioutil.TempDir("", "agent-logging-")
	require.NoError(err)
	require.NoError(os.Mkdir(fmt.Sprintf("%s/tmp", tmpDirName), 0666))
	defer os.RemoveAll(tmpDirName)

	agt := &Agent{
		opts: Options{
			HostID:           "host",
			HostSecret:       "secret",
			StatusPort:       2286,
			LogPrefix:        evergreen.LocalLoggingOverride,
			WorkingDirectory: tmpDirName,
		},
		comm: client.NewHostCommunicator("www.example.com", "host", "secret"),
	}
	jpm, err := jasper.NewSynchronizedManager(false)
	require.NoError(err)
	agt.jasper = jpm
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// run a task with a command logger specified as a file
	taskID := "logging"
	taskSecret := "mock_task_secret"
	task := &task.Task{
		Id:          "t1",
		Execution:   0,
		DisplayName: "task1",
	}
	tc := &taskContext{
		taskDirectory: tmpDirName,
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
		ranSetupGroup: false,
		taskConfig: &internal.TaskConfig{
			Task:         task,
			BuildVariant: &model.BuildVariant{Name: "bv"},
			Project: &model.Project{
				Tasks: []model.ProjectTask{
					{Name: "task1", Commands: []model.PluginCommandConf{
						{
							Command: "shell.exec",
							Params: map[string]interface{}{
								"shell":  "bash",
								"script": "echo 'hello world'",
							},
							Loggers: &model.LoggerConfig{
								Agent:  []model.LogOpts{{Type: model.FileLogSender}},
								System: []model.LogOpts{{Type: model.SplunkLogSender}},
								Task:   []model.LogOpts{{Type: model.FileLogSender}},
							},
						},
					}},
				},
				BuildVariants: model.BuildVariants{
					{Name: "bv", Tasks: []model.BuildVariantTaskUnit{{Name: "task1"}}},
				},
			},
			Timeout: &internal.Timeout{IdleTimeoutSecs: 15, ExecTimeoutSecs: 15},
			WorkDir: tmpDirName,
		},
		taskModel: task,
	}
	assert.NoError(agt.startLogging(ctx, tc))
	defer agt.removeTaskDirectory(tc)
	err = agt.runTaskCommands(ctx, tc)
	require.NoError(err)

	// verify log contents
	f, err := os.Open(fmt.Sprintf("%s/%s/%s/task.log", tmpDirName, taskLogDirectory, "shell.exec"))
	require.NoError(err)
	bytes, err := ioutil.ReadAll(f)
	require.NoError(err)
	assert.Contains(string(bytes), "hello world")

	// mock upload the logs
	bucket, err := pail.NewLocalBucket(pail.LocalOptions{
		Path: tmpDirName,
	})
	require.NoError(err)
	require.NoError(agt.uploadLogDir(ctx, tc, bucket, filepath.Join(tmpDirName, taskLogDirectory), ""))

	// verify uploaded log contents
	f, err = os.Open(fmt.Sprintf("%s/logs/%s/%d/%s/task.log", tmpDirName, tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution, "shell.exec"))
	require.NoError(err)
	bytes, err = ioutil.ReadAll(f)
	require.NoError(err)
	assert.Contains(string(bytes), "hello world")

	// verify populated URLs
	assert.Equal("shell.exec", tc.logs.TaskLogURLs[0].Command)
	assert.Contains(tc.logs.TaskLogURLs[0].URL, "/logs/t1/0/shell.exec/task.log")
}

func TestStartLogging(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	tmpDirName, err := ioutil.TempDir("", "reset-logging-")
	require.NoError(err)
	defer os.RemoveAll(tmpDirName)
	agt := &Agent{
		opts: Options{
			HostID:           "host",
			HostSecret:       "secret",
			StatusPort:       2286,
			LogPrefix:        evergreen.LocalLoggingOverride,
			WorkingDirectory: tmpDirName,
		},
		comm: client.NewMock("url"),
	}
	tc := &taskContext{
		task: client.TaskData{
			ID:     "logging",
			Secret: "task_secret",
		},
		taskConfig: &internal.TaskConfig{
			Task: &task.Task{},
		},
	}

	ctx := context.Background()
	assert.NoError(agt.fetchProjectConfig(ctx, tc))
	assert.EqualValues(model.EvergreenLogSender, tc.project.Loggers.Agent[0].Type)
	assert.EqualValues(model.SplunkLogSender, tc.project.Loggers.System[0].Type)
	assert.EqualValues(model.FileLogSender, tc.project.Loggers.Task[0].Type)

	assert.NoError(agt.startLogging(ctx, tc))
	tc.logger.Execution().Info("foo")
	assert.NoError(tc.logger.Close())
	msgs := agt.comm.(*client.Mock).GetMockMessages()
	assert.Equal("foo", msgs[tc.task.ID][0].Message)

	config, err := agt.makeTaskConfig(ctx, tc)
	assert.NoError(err)
	assert.NotNil(config.Project)

	// check that expansions are correctly populated
	logConfig := agt.prepLogger(tc, tc.project.Loggers, "")
	assert.Equal("bar", logConfig.System[0].SplunkToken)
}

func TestStartLoggingErrors(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	tmpDirName, err := ioutil.TempDir("", "logging-error-")
	require.NoError(err)
	defer os.RemoveAll(tmpDirName)
	agt := &Agent{
		opts: Options{
			HostID:           "host",
			HostSecret:       "secret",
			StatusPort:       2286,
			LogPrefix:        evergreen.LocalLoggingOverride,
			WorkingDirectory: tmpDirName,
		},
		comm: client.NewHostCommunicator("www.foo.com", "host", "secret"),
	}
	project := &model.Project{
		Loggers: &model.LoggerConfig{
			Agent: []model.LogOpts{{Type: model.LogkeeperLogSender}},
		},
		Tasks: []model.ProjectTask{},
	}
	tc := &taskContext{
		taskDirectory: tmpDirName,
		task: client.TaskData{
			ID:     "logging_error",
			Secret: "secret",
		},
		ranSetupGroup: false,
		taskConfig:    &internal.TaskConfig{},
		project:       project,
		taskModel:     &task.Task{},
	}

	ctx := context.Background()
	assert.Error(agt.startLogging(ctx, tc))
}

func TestLogkeeperMetadataPopulated(t *testing.T) {
	assert := assert.New(t)

	agt := &Agent{
		opts: Options{
			HostID:       "host",
			HostSecret:   "secret",
			StatusPort:   2286,
			LogPrefix:    evergreen.LocalLoggingOverride,
			LogkeeperURL: "logkeeper",
		},
		comm: client.NewMock("mock"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "logging"
	taskSecret := "mock_task_secret"
	task := &task.Task{
		DisplayName: "task1",
	}
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
		project: &model.Project{
			Loggers: &model.LoggerConfig{
				Agent:  []model.LogOpts{{Type: model.LogkeeperLogSender}},
				System: []model.LogOpts{{Type: model.LogkeeperLogSender}},
				Task:   []model.LogOpts{{Type: model.LogkeeperLogSender}},
			},
		},
		taskConfig: &internal.TaskConfig{
			Task:         task,
			BuildVariant: &model.BuildVariant{Name: "bv"},
			Timeout:      &internal.Timeout{IdleTimeoutSecs: 15, ExecTimeoutSecs: 15},
		},
		taskModel: task,
	}
	assert.NoError(agt.startLogging(ctx, tc))
	assert.Equal("logkeeper/build/build1/test/test1", tc.logs.AgentLogURLs[0].URL)
	assert.Equal("logkeeper/build/build1/test/test2", tc.logs.SystemLogURLs[0].URL)
	assert.Equal("logkeeper/build/build1/test/test3", tc.logs.TaskLogURLs[0].URL)
	assert.Equal("", tc.logs.TaskLogURLs[0].Command)
}

func TestDefaultSender(t *testing.T) {
	agt := &Agent{
		opts: Options{
			HostID:       "host",
			HostSecret:   "secret",
			StatusPort:   2286,
			LogPrefix:    evergreen.LocalLoggingOverride,
			LogkeeperURL: "logkeeper",
		},
		comm: client.NewMock("mock"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "logging"
	taskSecret := "mock_task_secret"
	task := &task.Task{
		DisplayName: "task1",
	}
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
		project: &model.Project{},
		taskConfig: &internal.TaskConfig{
			Task:         task,
			BuildVariant: &model.BuildVariant{Name: "bv"},
			Timeout:      &internal.Timeout{IdleTimeoutSecs: 15, ExecTimeoutSecs: 15},
		},
		taskModel: task,
	}

	t.Run("Valid", func(t *testing.T) {
		tc.project.Loggers = &model.LoggerConfig{}
		tc.taskConfig.ProjectRef = &model.ProjectRef{DefaultLogger: model.BuildloggerLogSender}

		assert.NoError(t, agt.startLogging(ctx, tc))
		expectedLogOpts := []model.LogOpts{{Type: model.BuildloggerLogSender}}
		assert.Equal(t, expectedLogOpts, tc.project.Loggers.Agent)
		assert.Equal(t, expectedLogOpts, tc.project.Loggers.System)
		assert.Equal(t, expectedLogOpts, tc.project.Loggers.Task)
	})
	t.Run("Invalid", func(t *testing.T) {
		tc.project.Loggers = &model.LoggerConfig{}
		tc.taskConfig.ProjectRef = &model.ProjectRef{DefaultLogger: model.SplunkLogSender}

		assert.NoError(t, agt.startLogging(ctx, tc))
		expectedLogOpts := []model.LogOpts{{Type: model.EvergreenLogSender}}
		assert.Equal(t, expectedLogOpts, tc.project.Loggers.Agent)
		assert.Equal(t, expectedLogOpts, tc.project.Loggers.System)
		assert.Equal(t, expectedLogOpts, tc.project.Loggers.Task)
	})
}

func TestTimberSender(t *testing.T) {
	agt := &Agent{
		opts: Options{
			HostID:       "host",
			HostSecret:   "secret",
			StatusPort:   2286,
			LogPrefix:    evergreen.LocalLoggingOverride,
			LogkeeperURL: "logkeeper",
		},
		comm: client.NewMock("mock"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "logging"
	taskSecret := "mock_task_secret"
	task := &task.Task{
		DisplayName: "task1",
	}
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
		project: &model.Project{
			Loggers: &model.LoggerConfig{
				Agent:  []model.LogOpts{{Type: model.BuildloggerLogSender}},
				System: []model.LogOpts{{Type: model.BuildloggerLogSender}},
				Task:   []model.LogOpts{{Type: model.BuildloggerLogSender}},
			},
		},
		taskConfig: &internal.TaskConfig{
			Task:         task,
			BuildVariant: &model.BuildVariant{Name: "bv"},
			Timeout:      &internal.Timeout{IdleTimeoutSecs: 15, ExecTimeoutSecs: 15},
		},
		taskModel: task,
	}
	assert.NoError(t, agt.startLogging(ctx, tc))
}
