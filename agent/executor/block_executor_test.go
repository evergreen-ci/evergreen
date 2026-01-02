package executor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCommandsInBlock(t *testing.T) {
	ctx := context.Background()

	t.Run("NilCommands", func(t *testing.T) {
		deps := BlockExecutorDeps{
			TaskLogger: grip.NewJournaler("test"),
			ExecLogger: grip.NewJournaler("test"),
		}
		cmdBlock := CommandBlock{
			Commands: nil,
		}
		err := RunCommandsInBlock(ctx, deps, cmdBlock)
		assert.NoError(t, err)
	})

	t.Run("EmptyCommands", func(t *testing.T) {
		deps := BlockExecutorDeps{
			TaskLogger: grip.NewJournaler("test"),
			ExecLogger: grip.NewJournaler("test"),
			TaskConfig: &internal.TaskConfig{
				Project: model.Project{},
			},
		}
		cmdBlock := CommandBlock{
			Block:    command.MainTaskBlock,
			Commands: &model.YAMLCommandSet{},
		}
		err := RunCommandsInBlock(ctx, deps, cmdBlock)
		assert.NoError(t, err)
	})

	t.Run("RunsCommands", func(t *testing.T) {
		commandRun := false
		deps := BlockExecutorDeps{
			TaskLogger: grip.NewJournaler("test"),
			ExecLogger: grip.NewJournaler("test"),
			TaskConfig: &internal.TaskConfig{
				Project: model.Project{},
			},
			RunCommandOrFunc: func(ctx context.Context, commandInfo model.PluginCommandConf, cmds []command.Command, block command.BlockType, canFailTask bool) error {
				commandRun = true
				assert.Equal(t, command.MainTaskBlock, block)
				assert.True(t, canFailTask)
				return nil
			},
		}

		cmdBlock := CommandBlock{
			Block: command.MainTaskBlock,
			Commands: &model.YAMLCommandSet{
				SingleCommand: &model.PluginCommandConf{
					Command: "shell.exec",
					Params: map[string]any{
						"script": "echo test",
					},
				},
			},
			CanFailTask: true,
		}

		err := RunCommandsInBlock(ctx, deps, cmdBlock)
		assert.NoError(t, err)
		assert.True(t, commandRun)
	})

	t.Run("TimeoutWatcher", func(t *testing.T) {
		timeoutWatcherCalled := false
		deps := BlockExecutorDeps{
			TaskLogger: grip.NewJournaler("test"),
			ExecLogger: grip.NewJournaler("test"),
			TaskConfig: &internal.TaskConfig{
				Project: model.Project{},
			},
			StartTimeoutWatcher: func(ctx context.Context, cancel context.CancelFunc, kind globals.TimeoutType, getTimeout func() time.Duration, canMarkFailure bool) {
				timeoutWatcherCalled = true
				assert.Equal(t, globals.ExecTimeout, kind)
				assert.Equal(t, 5*time.Second, getTimeout())
				assert.True(t, canMarkFailure)
			},
			RunCommandOrFunc: func(ctx context.Context, commandInfo model.PluginCommandConf, cmds []command.Command, block command.BlockType, canFailTask bool) error {
				return nil
			},
		}

		cmdBlock := CommandBlock{
			Block: command.MainTaskBlock,
			Commands: &model.YAMLCommandSet{
				SingleCommand: &model.PluginCommandConf{
					Command: "shell.exec",
					Params: map[string]any{
						"script": "echo test",
					},
				},
			},
			TimeoutKind: globals.ExecTimeout,
			GetTimeout: func() time.Duration {
				return 5 * time.Second
			},
			CanFailTask: true,
		}

		err := RunCommandsInBlock(ctx, deps, cmdBlock)
		assert.NoError(t, err)
		assert.True(t, timeoutWatcherCalled)
	})

	t.Run("HeartbeatTimeout", func(t *testing.T) {
		setHeartbeatCalled := false
		resetHeartbeatCalled := false

		deps := BlockExecutorDeps{
			TaskLogger: grip.NewJournaler("test"),
			ExecLogger: grip.NewJournaler("test"),
			TaskConfig: &internal.TaskConfig{
				Project: model.Project{},
			},
			SetHeartbeatTimeout: func(startAt time.Time, getTimeout func() time.Duration, kind globals.TimeoutType) {
				setHeartbeatCalled = true
				assert.Equal(t, globals.ExecTimeout, kind)
				assert.Equal(t, 10*time.Second, getTimeout())
			},
			ResetHeartbeatTimeout: func() {
				resetHeartbeatCalled = true
			},
			StartTimeoutWatcher: func(ctx context.Context, cancel context.CancelFunc, kind globals.TimeoutType, getTimeout func() time.Duration, canMarkFailure bool) {
			},
			RunCommandOrFunc: func(ctx context.Context, commandInfo model.PluginCommandConf, cmds []command.Command, block command.BlockType, canFailTask bool) error {
				return nil
			},
		}

		cmdBlock := CommandBlock{
			Block: command.MainTaskBlock,
			Commands: &model.YAMLCommandSet{
				SingleCommand: &model.PluginCommandConf{
					Command: "shell.exec",
					Params: map[string]any{
						"script": "echo test",
					},
				},
			},
			TimeoutKind: globals.ExecTimeout,
			GetTimeout: func() time.Duration {
				return 10 * time.Second
			},
			CanTimeOutHeartbeat: true,
		}

		err := RunCommandsInBlock(ctx, deps, cmdBlock)
		assert.NoError(t, err)
		assert.True(t, setHeartbeatCalled)
		assert.True(t, resetHeartbeatCalled)
	})

	t.Run("PanicRecovery", func(t *testing.T) {
		panicHandlerCalled := false

		sender := send.MakeInternalLogger()
		logger := grip.NewJournaler("test")
		require.NoError(t, logger.SetSender(sender))

		deps := BlockExecutorDeps{
			TaskLogger: logger,
			ExecLogger: logger,
			TaskConfig: &internal.TaskConfig{
				Project: model.Project{},
			},
			HandlePanic: func(panicErr error, originalErr error, op string) error {
				panicHandlerCalled = true
				assert.NotNil(t, panicErr)
				assert.Contains(t, panicErr.Error(), "test panic")
				assert.Contains(t, op, "running commands for block")
				return panicErr
			},
			RunCommandOrFunc: func(ctx context.Context, commandInfo model.PluginCommandConf, cmds []command.Command, block command.BlockType, canFailTask bool) error {
				panic("test panic")
			},
		}

		cmdBlock := CommandBlock{
			Block: command.MainTaskBlock,
			Commands: &model.YAMLCommandSet{
				SingleCommand: &model.PluginCommandConf{
					Command: "shell.exec",
					Params: map[string]any{
						"script": "echo test",
					},
				},
			},
		}

		err := RunCommandsInBlock(ctx, deps, cmdBlock)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "test panic")
		assert.True(t, panicHandlerCalled)
	})

	t.Run("PanicRecoveryWithoutHandler", func(t *testing.T) {
		sender := send.MakeInternalLogger()
		logger := grip.NewJournaler("test")
		require.NoError(t, logger.SetSender(sender))

		deps := BlockExecutorDeps{
			TaskLogger: logger,
			ExecLogger: logger,
			TaskConfig: &internal.TaskConfig{
				Project: model.Project{},
			},
			HandlePanic: nil,
			RunCommandOrFunc: func(ctx context.Context, commandInfo model.PluginCommandConf, cmds []command.Command, block command.BlockType, canFailTask bool) error {
				panic("test panic without handler")
			},
		}

		cmdBlock := CommandBlock{
			Block: command.MainTaskBlock,
			Commands: &model.YAMLCommandSet{
				SingleCommand: &model.PluginCommandConf{
					Command: "shell.exec",
					Params: map[string]any{
						"script": "echo test",
					},
				},
			},
		}

		err := RunCommandsInBlock(ctx, deps, cmdBlock)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "test panic without handler")

		msg := sender.GetMessage()
		assert.NotNil(t, msg)
		assert.Equal(t, level.Error, msg.Priority)
	})

	t.Run("CommandError", func(t *testing.T) {
		expectedErr := errors.New("command failed")

		deps := BlockExecutorDeps{
			TaskLogger: grip.NewJournaler("test"),
			ExecLogger: grip.NewJournaler("test"),
			TaskConfig: &internal.TaskConfig{
				Project: model.Project{},
			},
			RunCommandOrFunc: func(ctx context.Context, commandInfo model.PluginCommandConf, cmds []command.Command, block command.BlockType, canFailTask bool) error {
				return expectedErr
			},
		}

		cmdBlock := CommandBlock{
			Block: command.MainTaskBlock,
			Commands: &model.YAMLCommandSet{
				SingleCommand: &model.PluginCommandConf{
					Command: "shell.exec",
					Params: map[string]any{
						"script": "echo test",
					},
				},
			},
		}

		err := RunCommandsInBlock(ctx, deps, cmdBlock)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "command failed")
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		deps := BlockExecutorDeps{
			TaskLogger: grip.NewJournaler("test"),
			ExecLogger: grip.NewJournaler("test"),
			TaskConfig: &internal.TaskConfig{
				Project: model.Project{},
			},
			RunCommandOrFunc: func(ctx context.Context, commandInfo model.PluginCommandConf, cmds []command.Command, block command.BlockType, canFailTask bool) error {
				t.Fatal("Should not reach here")
				return nil
			},
		}

		cmdBlock := CommandBlock{
			Block: command.MainTaskBlock,
			Commands: &model.YAMLCommandSet{
				SingleCommand: &model.PluginCommandConf{
					Command: "shell.exec",
					Params: map[string]any{
						"script": "echo test",
					},
				},
			},
		}

		err := RunCommandsInBlock(ctx, deps, cmdBlock)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "canceled while running commands")
	})
}

func TestBlockToLegacyName(t *testing.T) {
	tests := []struct {
		block    command.BlockType
		expected string
	}{
		{command.PreBlock, "pre-task"},
		{command.MainTaskBlock, "task"},
		{command.PostBlock, "post-task"},
		{command.TaskTimeoutBlock, "task-timeout"},
		{command.SetupGroupBlock, "setup-group"},
		{command.SetupTaskBlock, "setup-task"},
		{command.TeardownTaskBlock, "teardown-task"},
		{command.TeardownGroupBlock, "teardown-group"},
		{command.BlockType("custom"), "custom"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Block_%s", test.block), func(t *testing.T) {
			result := blockToLegacyName(test.block)
			assert.Equal(t, test.expected, result)
		})
	}
}
