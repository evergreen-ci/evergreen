package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// SharedBlockExecutor provides the core command block execution logic
// that can be shared between Agent and LocalTaskExecutor
type SharedBlockExecutor struct {
	executor CommandExecutor
}

func NewSharedBlockExecutor(executor CommandExecutor) *SharedBlockExecutor {
	return &SharedBlockExecutor{
		executor: executor,
	}
}

// RunCommandsInBlock runs all the commands listed in a block (e.g. pre, post, main)
func (s *SharedBlockExecutor) RunCommandsInBlock(ctx context.Context, execCtx ExecutionContext, cmdBlock CommandBlock) (err error) {
	if cmdBlock.Commands == nil {
		return nil
	}

	taskLogger := execCtx.GetTaskLogger()
	execLogger := execCtx.GetExecutionLogger()

	blockCtx, blockCancel := context.WithCancel(ctx)
	defer blockCancel()

	if cmdBlock.TimeoutKind != "" && cmdBlock.GetTimeout != nil {
		timeoutOpts := TimeoutWatcherOptions{
			ExecutionContext:      execCtx,
			Kind:                  cmdBlock.TimeoutKind,
			GetTimeout:            cmdBlock.GetTimeout,
			CanMarkTimeoutFailure: cmdBlock.CanFailTask,
		}
		go s.executor.StartTimeoutWatcher(blockCtx, blockCancel, timeoutOpts)

		if cmdBlock.CanTimeOutHeartbeat {
			execLogger.Infof("Setting heartbeat timeout to type '%s'.", cmdBlock.TimeoutKind)
			execCtx.SetHeartbeatTimeout(HeartbeatTimeoutOptions{
				StartAt:    time.Now(),
				GetTimeout: cmdBlock.GetTimeout,
				Kind:       cmdBlock.TimeoutKind,
			})
			defer func() {
				execLogger.Infof("Resetting heartbeat timeout from type '%s' back to default.", cmdBlock.TimeoutKind)
				execCtx.SetHeartbeatTimeout(HeartbeatTimeoutOptions{})
			}()
		}
	}

	defer func() {
		op := fmt.Sprintf("running commands for block '%s'", cmdBlock.Block)
		pErr := recovery.HandlePanicWithError(recover(), nil, op)
		if pErr != nil {
			taskLogger.Error(message.Fields{
				"message":   "programmatic error: Evergreen agent hit panic",
				"operation": op,
				"error":     pErr.Error(),
			})
			err = errors.Wrap(pErr, op)
		}
	}()

	legacyBlockName := blockToLegacyName(cmdBlock.Block)
	taskLogger.Infof("Running %s commands.", legacyBlockName)
	start := time.Now()
	defer func() {
		if err != nil {
			taskLogger.Error(errors.Wrapf(err, "Running %s commands failed", legacyBlockName))
		}
		taskLogger.Infof("Finished running %s commands in %s.", legacyBlockName, time.Since(start).String())
	}()

	commands := cmdBlock.Commands.List()
	taskConfig := execCtx.GetTaskConfig()

	for i, commandInfo := range commands {
		if err := blockCtx.Err(); err != nil {
			return errors.Wrap(err, "canceled while running commands")
		}

		blockInfo := command.BlockInfo{
			Block:     cmdBlock.Block,
			CmdNum:    i + 1,
			TotalCmds: len(commands),
		}

		cmds, err := command.Render(commandInfo, &taskConfig.Project, blockInfo)
		if err != nil {
			return errors.Wrapf(err, "rendering command '%s'", commandInfo.Command)
		}

		runCmdOpts := RunCommandsOptions{
			Block:       cmdBlock.Block,
			CanFailTask: cmdBlock.CanFailTask,
		}

		if err = s.executor.RunCommandOrFunc(blockCtx, execCtx, commandInfo, cmds, runCmdOpts); err != nil {
			return errors.WithStack(err)
		}
	}

	return errors.WithStack(err)
}

// blockToLegacyName converts the name of a command block to the name it has
// historically been referred to as in the task logs.
func blockToLegacyName(block command.BlockType) string {
	switch block {
	case command.PreBlock:
		return "pre-task"
	case command.MainTaskBlock:
		return "task"
	case command.PostBlock:
		return "post-task"
	case command.TaskTimeoutBlock:
		return "task-timeout"
	case command.SetupGroupBlock:
		return "setup-group"
	case command.SetupTaskBlock:
		return "setup-task"
	case command.TeardownTaskBlock:
		return "teardown-task"
	case command.TeardownGroupBlock:
		return "teardown-group"
	default:
		return string(block)
	}
}
