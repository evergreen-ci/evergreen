package agent

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSenderRemote(t *testing.T) {
	assert := assert.New(t)
	_ = os.Setenv("GRIP_SUMO_ENDPOINT", "http://www.example.com/")
	_ = os.Setenv("GRIP_SPLUNK_SERVER_URL", "http://www.example.com/")
	_ = os.Setenv("GRIP_SPLUNK_CLIENT_TOKEN", "token")
	_ = os.Setenv("GRIP_SPLUNK_CHANNEL", "channel")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := GetSender(ctx, evergreen.LocalLoggingOverride, "task_id")
	assert.NoError(err)
}

func TestGetSenderLocal(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := GetSender(ctx, evergreen.LocalLoggingOverride, "task_id")
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
		comm: client.NewCommunicator("www.example.com"),
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
		runGroupSetup: true,
		taskConfig: &model.TaskConfig{
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
			Timeout: &model.Timeout{IdleTimeoutSecs: 15, ExecTimeoutSecs: 15},
			WorkDir: tmpDirName,
		},
		taskModel: task,
	}
	assert.NoError(agt.resetLogging(ctx, tc))
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

func TestResetLogging(t *testing.T) {
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
		taskConfig: &model.TaskConfig{
			Task: &task.Task{},
		},
	}

	ctx := context.Background()
	assert.NoError(agt.fetchProjectConfig(ctx, tc))
	assert.EqualValues(model.EvergreenLogSender, tc.project.Loggers.Agent[0].Type)
	assert.EqualValues(model.SplunkLogSender, tc.project.Loggers.System[0].Type)
	assert.EqualValues(model.FileLogSender, tc.project.Loggers.Task[0].Type)

	assert.NoError(agt.resetLogging(ctx, tc))
	tc.logger.Execution().Info("foo")
	assert.NoError(tc.logger.Close())
	msgs := agt.comm.(*client.Mock).GetMockMessages()
	assert.Equal("foo", msgs[tc.task.ID][0].Message)

	config, err := agt.makeTaskConfig(ctx, tc)
	assert.NoError(err)
	assert.NotNil(config.Project)
	assert.NotNil(config.Version)

	// check that expansions are correctly populated
	logConfig := agt.prepLogger(tc, tc.project.Loggers, "")
	assert.Equal("bar", logConfig.System[0].SplunkToken)
}

func TestResetLoggingErrors(t *testing.T) {
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
		comm: client.NewCommunicator("www.foo.com"),
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
		runGroupSetup: true,
		taskConfig:    &model.TaskConfig{},
		project:       project,
		taskModel:     &task.Task{},
	}

	ctx := context.Background()
	assert.Error(agt.resetLogging(ctx, tc))
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
		taskConfig: &model.TaskConfig{
			Task:         task,
			BuildVariant: &model.BuildVariant{Name: "bv"},
			Timeout:      &model.Timeout{IdleTimeoutSecs: 15, ExecTimeoutSecs: 15},
		},
		taskModel: task,
	}
	assert.NoError(agt.resetLogging(ctx, tc))
	assert.Equal("logkeeper/build/build1/test/test1", tc.logs.AgentLogURLs[0].URL)
	assert.Equal("logkeeper/build/build1/test/test2", tc.logs.SystemLogURLs[0].URL)
	assert.Equal("logkeeper/build/build1/test/test3", tc.logs.TaskLogURLs[0].URL)
	assert.Equal("", tc.logs.TaskLogURLs[0].Command)
}

func TestTimberSender(t *testing.T) {
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
				Agent:  []model.LogOpts{{Type: model.BuildloggerLogSender}},
				System: []model.LogOpts{{Type: model.BuildloggerLogSender}},
				Task:   []model.LogOpts{{Type: model.BuildloggerLogSender}},
			},
		},
		taskConfig: &model.TaskConfig{
			Task:         task,
			BuildVariant: &model.BuildVariant{Name: "bv"},
			Timeout:      &model.Timeout{IdleTimeoutSecs: 15, ExecTimeoutSecs: 15},
		},
		taskModel: task,
	}
	assert.NoError(agt.resetLogging(ctx, tc))
}
