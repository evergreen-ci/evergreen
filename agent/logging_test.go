package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestGetSenderLocal(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := (&Agent{}).GetSender(ctx, LogOutputStdout, "agent")
	assert.NoError(err)
}

func TestAgentFileLogging(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	tmpDirName := t.TempDir()
	require.NoError(os.Mkdir(fmt.Sprintf("%s/tmp", tmpDirName), 0666))

	agt := &Agent{
		opts: Options{
			HostID:           "host",
			HostSecret:       "secret",
			StatusPort:       2286,
			LogOutput:        LogOutputStdout,
			LogPrefix:        "agent",
			WorkingDirectory: tmpDirName,
		},
		comm:   client.NewHostCommunicator("www.example.com", "host", "secret"),
		tracer: otel.GetTracerProvider().Tracer("noop_tracer"),
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
					{Name: "bv", Tasks: []model.BuildVariantTaskUnit{{Name: "task1", Variant: "bv"}}},
				},
			},
			Timeout:    &internal.Timeout{IdleTimeoutSecs: 15, ExecTimeoutSecs: 15},
			WorkDir:    tmpDirName,
			Expansions: util.NewExpansions(nil),
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
	bytes, err := io.ReadAll(f)
	assert.NoError(f.Close())
	require.NoError(err)
	assert.Contains(string(bytes), "hello world")
}

func TestStartLogging(t *testing.T) {
	assert := assert.New(t)
	tmpDirName := t.TempDir()
	agt := &Agent{
		opts: Options{
			HostID:           "host",
			HostSecret:       "secret",
			StatusPort:       2286,
			LogOutput:        LogOutputStdout,
			LogPrefix:        "agent",
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
	require.NotNil(t, tc.project)
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

// kim: TODO: delete because there's no way to get this to fail.
// func TestStartLoggingErrors(t *testing.T) {
//     assert := assert.New(t)
//     // tmpDirName := t.TempDir()
//     agt := &Agent{
//         opts: Options{
//             // HostID:           "host",
//             // HostSecret:       "secret",
//             // StatusPort:       2286,
//             // LogOutput:        LogOutputStdout,
//             // LogPrefix:        "agent",
//             // WorkingDirectory: tmpDirName,
//         },
//         comm: client.NewHostCommunicator("www.foo.com", "host", "secret"),
//     }
//     // project := &model.Project{
//     //     Loggers: &model.LoggerConfig{
//     //         // kim: TODO: delete
//     //         // Agent: []model.LogOpts{{Type: model.LogkeeperLogSender}},
//     //         // Agent: []model.LogOpts{{
//     //         //     Type:         model.FileLogSender,
//     //         //     LogDirectory: filepath.Join("🍪", "is", "not", "a", "valid", "directory"),
//     //         // }},
//     //     },
//     //     Tasks: []model.ProjectTask{},
//     // }
//     tc := &taskContext{
//         // taskDirectory: tmpDirName,
//         // task: client.TaskData{
//         //     ID:     "logging_error",
//         //     Secret: "secret",
//         // },
//         // ranSetupGroup: false,
//         // taskConfig:    &internal.TaskConfig{},
//         // project:       project,
//         // taskModel:     &task.Task{},
//     }
//
//     ctx := context.Background()
//     assert.Error(agt.startLogging(ctx, tc))
// }

// kim: TODO: delete
// func TestLogkeeperMetadataPopulated(t *testing.T) {
//     assert := assert.New(t)
//
//     agt := &Agent{
//         opts: Options{
//             HostID:       "host",
//             HostSecret:   "secret",
//             StatusPort:   2286,
//             LogOutput:    LogOutputStdout,
//             LogPrefix:    "agent",
//             LogkeeperURL: "logkeeper",
//         },
//         comm: client.NewMock("mock"),
//     }
//     ctx, cancel := context.WithCancel(context.Background())
//     defer cancel()
//
//     taskID := "logging"
//     taskSecret := "mock_task_secret"
//     task := &task.Task{
//         DisplayName: "task1",
//     }
//     tc := &taskContext{
//         task: client.TaskData{
//             ID:     taskID,
//             Secret: taskSecret,
//         },
//         project: &model.Project{
//             Loggers: &model.LoggerConfig{
//                 Agent:  []model.LogOpts{{Type: model.LogkeeperLogSender}},
//                 System: []model.LogOpts{{Type: model.LogkeeperLogSender}},
//                 Task:   []model.LogOpts{{Type: model.LogkeeperLogSender}},
//             },
//         },
//         taskConfig: &internal.TaskConfig{
//             Task:         task,
//             BuildVariant: &model.BuildVariant{Name: "bv"},
//             Timeout:      &internal.Timeout{IdleTimeoutSecs: 15, ExecTimeoutSecs: 15},
//         },
//         taskModel: task,
//     }
//     assert.NoError(agt.startLogging(ctx, tc))
//     assert.Equal("logkeeper/build/build1/test/test1", tc.logs.AgentLogURLs[0].URL)
//     assert.Equal("logkeeper/build/build1/test/test2", tc.logs.SystemLogURLs[0].URL)
//     assert.Equal("logkeeper/build/build1/test/test3", tc.logs.TaskLogURLs[0].URL)
//     assert.Equal("", tc.logs.TaskLogURLs[0].Command)
// }

func TestDefaultSender(t *testing.T) {
	agt := &Agent{
		opts: Options{
			HostID:     "host",
			HostSecret: "secret",
			StatusPort: 2286,
			LogOutput:  LogOutputStdout,
			LogPrefix:  "agent",
			// kim: TODO: remove
			// LogkeeperURL: "logkeeper",
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
			HostID:     "host",
			HostSecret: "secret",
			StatusPort: 2286,
			LogOutput:  LogOutputStdout,
			LogPrefix:  "agent",
			// kim: TODO: remove
			// LogkeeperURL: "logkeeper",
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
