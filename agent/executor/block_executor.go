package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

// BlockExecutorDeps contains all dependencies needed for command block execution.
// This uses composition instead of complex interfaces to share logic between
// Agent and LocalTaskExecutor.
type BlockExecutorDeps struct {
	JasperManager jasper.Manager
	Tracer        trace.Tracer
	TaskLogger    grip.Journaler
	ExecLogger    grip.Journaler
	TaskConfig    *internal.TaskConfig

	StartTimeoutWatcher   func(ctx context.Context, cancel context.CancelFunc, kind globals.TimeoutType, getTimeout func() time.Duration, canMarkFailure bool)
	SetHeartbeatTimeout   func(startAt time.Time, getTimeout func() time.Duration, kind globals.TimeoutType)
	ResetHeartbeatTimeout func()
	HandlePanic           func(panicErr error, originalErr error, op string) error
	RunCommandOrFunc      func(ctx context.Context, commandInfo model.PluginCommandConf, cmds []command.Command, block command.BlockType, canFailTask bool) error
}

// RunCommandsInBlock executes all commands in a block and can be integrated into
// agent task execution and local task execution.
func RunCommandsInBlock(ctx context.Context, deps BlockExecutorDeps, cmdBlock CommandBlock) (err error) {
	if cmdBlock.Commands == nil {
		return nil
	}

	blockCtx, blockCancel := context.WithCancel(ctx)
	defer blockCancel()

	if cmdBlock.TimeoutKind != "" && cmdBlock.GetTimeout != nil {
		if deps.StartTimeoutWatcher != nil {
			go deps.StartTimeoutWatcher(blockCtx, blockCancel, cmdBlock.TimeoutKind, cmdBlock.GetTimeout, cmdBlock.CanFailTask)
		}

		if cmdBlock.CanTimeOutHeartbeat && deps.SetHeartbeatTimeout != nil {
			deps.ExecLogger.Infof("Setting heartbeat timeout to type '%s'.", cmdBlock.TimeoutKind)
			deps.SetHeartbeatTimeout(time.Now(), cmdBlock.GetTimeout, cmdBlock.TimeoutKind)
			defer func() {
				if deps.ResetHeartbeatTimeout != nil {
					deps.ExecLogger.Infof("Resetting heartbeat timeout from type '%s' back to default.", cmdBlock.TimeoutKind)
					deps.ResetHeartbeatTimeout()
				}
			}()
		}
	}

	defer func() {
		op := fmt.Sprintf("running commands for block '%s'", cmdBlock.Block)
		pErr := recovery.HandlePanicWithError(recover(), nil, op)
		if pErr == nil {
			return
		}

		// Use the provided panic handler if available (for agent),
		// otherwise fall back to basic logging (for local execution)
		if deps.HandlePanic != nil {
			err = deps.HandlePanic(pErr, err, op)
		} else {
			deps.TaskLogger.Error(message.Fields{
				"message":   "programmatic error: Evergreen agent hit panic",
				"operation": op,
				"error":     pErr.Error(),
			})
			err = errors.Wrap(pErr, op)
		}
	}()

	legacyBlockName := blockToLegacyName(cmdBlock.Block)
	deps.TaskLogger.Infof("Running %s commands.", legacyBlockName)
	start := time.Now()
	defer func() {
		if err != nil {
			deps.TaskLogger.Error(errors.Wrapf(err, "Running %s commands failed", legacyBlockName))
		}
		deps.TaskLogger.Infof("Finished running %s commands in %s.", legacyBlockName, time.Since(start).String())
	}()

	commands := cmdBlock.Commands.List()
	for i, commandInfo := range commands {
		if err := blockCtx.Err(); err != nil {
			return errors.Wrap(err, "canceled while running commands")
		}

		blockInfo := command.BlockInfo{
			Block:     cmdBlock.Block,
			CmdNum:    i + 1,
			TotalCmds: len(commands),
		}

		cmds, err := command.Render(commandInfo, &deps.TaskConfig.Project, blockInfo)
		if err != nil {
			return errors.Wrapf(err, "rendering command '%s'", commandInfo.Command)
		}

		if deps.RunCommandOrFunc != nil {
			if err = deps.RunCommandOrFunc(blockCtx, commandInfo, cmds, cmdBlock.Block, cmdBlock.CanFailTask); err != nil {
				return errors.WithStack(err)
			}
		}
	}

	return errors.WithStack(err)
}

// CommandBlock contains information for a block of commands
type CommandBlock struct {
	Block               command.BlockType
	Commands            *model.YAMLCommandSet
	TimeoutKind         globals.TimeoutType
	GetTimeout          func() time.Duration
	CanTimeOutHeartbeat bool
	CanFailTask         bool
}

// blockToLegacyName converts the name of a command block to the name it has
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
