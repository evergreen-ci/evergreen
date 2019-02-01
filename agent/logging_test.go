package agent

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
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

func TestCommandLoggerOverride(t *testing.T) {
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "logging"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		taskDirectory: tmpDirName,
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
		runGroupSetup: true,
		taskConfig: &model.TaskConfig{
			Task: &task.Task{
				DisplayName: "task1",
			},
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
								System: []model.LogOpts{{Type: model.FileLogSender}},
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
	}
	err = agt.resetLogging(ctx, tc)
	assert.NoError(err)
	defer agt.removeTaskDirectory(tc)
	err = agt.runTaskCommands(ctx, tc)
	assert.NoError(err)

	f, err := os.Open(fmt.Sprintf("%s/%s/task.log", tmpDirName, taskLogDirectory))
	assert.NoError(err)
	bytes, err := ioutil.ReadAll(f)
	assert.NoError(err)
	assert.Contains(string(bytes), "[p=info]: hello world")
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
	}

	ctx := context.Background()
	assert.NoError(agt.fetchProjectConfig(ctx, tc))
	assert.EqualValues(model.EvergreenLogSender, tc.project.Loggers.Agent[0].Type)
	assert.EqualValues(model.FileLogSender, tc.project.Loggers.System[0].Type)
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
}
