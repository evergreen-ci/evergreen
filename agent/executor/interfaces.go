package executor

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"go.opentelemetry.io/otel/trace"
)

// BlockExecutor defines the interface for executing command blocks
type BlockExecutor interface {
	RunCommandsInBlock(ctx context.Context, execCtx ExecutionContext, cmdBlock CommandBlock) error
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

// ExecutionContext provides the context needed for command execution
type ExecutionContext interface {
	GetTaskConfig() *internal.TaskConfig
	GetTaskData() TaskData

	SetCurrentCommand(cmd command.Command)
	GetCurrentCommand() command.Command
	SetFailingCommand(cmd command.Command)
	GetFailingCommand() command.Command
	AddOtherFailingCommand(cmd command.Command)

	GetTaskLogger() grip.Journaler
	GetExecutionLogger() grip.Journaler

	SetHeartbeatTimeout(opts HeartbeatTimeoutOptions)
	SetIdleTimeout(timeout time.Duration)
	HadTimedOut() bool
	GetTimeoutType() globals.TimeoutType
	GetTimeoutDuration() time.Duration

	AddTaskCommandCleanups(cleanups []internal.CommandCleanup)

	GetExpansions() *util.Expansions
	GetNewExpansions() *util.Expansions
	GetDynamicExpansions() *util.Expansions
}

// TaskData represents basic task information
type TaskData interface {
	GetID() string
	GetSecret() string
}

// HeartbeatTimeoutOptions represents heartbeat timeout configuration
type HeartbeatTimeoutOptions struct {
	StartAt    time.Time
	GetTimeout func() time.Duration
	Kind       globals.TimeoutType
}

// CommandExecutor provides command execution capabilities
type CommandExecutor interface {
	GetJasperManager() jasper.Manager

	GetTracer() trace.Tracer

	MarkFailedTaskToRestart(ctx context.Context, taskData TaskData) error
	UpdateLastMessageTime()

	StartTimeoutWatcher(ctx context.Context, cancel context.CancelFunc, opts TimeoutWatcherOptions)

	RunCommand(ctx context.Context, execCtx ExecutionContext, commandInfo model.PluginCommandConf, cmd command.Command, options RunCommandsOptions) error
	RunCommandOrFunc(ctx context.Context, execCtx ExecutionContext, commandInfo model.PluginCommandConf, cmds []command.Command, options RunCommandsOptions) error
}

// TimeoutWatcherOptions configures timeout watching
type TimeoutWatcherOptions struct {
	ExecutionContext      ExecutionContext
	Kind                  globals.TimeoutType
	GetTimeout            func() time.Duration
	CanMarkTimeoutFailure bool
}

// RunCommandsOptions provides options for command execution
type RunCommandsOptions struct {
	Block       command.BlockType
	CanFailTask bool
}
