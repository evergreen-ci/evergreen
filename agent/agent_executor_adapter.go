package agent

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/executor"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"go.opentelemetry.io/otel/trace"
)

// AgentCommandExecutor adapts the Agent to implement the CommandExecutor interface
type AgentCommandExecutor struct {
	agent *Agent
}

func NewAgentCommandExecutor(a *Agent) *AgentCommandExecutor {
	return &AgentCommandExecutor{agent: a}
}

func (e *AgentCommandExecutor) GetJasperManager() jasper.Manager {
	return e.agent.jasper
}

func (e *AgentCommandExecutor) GetTracer() trace.Tracer {
	return e.agent.tracer
}

func (e *AgentCommandExecutor) MarkFailedTaskToRestart(ctx context.Context, taskData executor.TaskData) error {
	return e.agent.comm.MarkFailedTaskToRestart(ctx, taskData.(*taskContextAdapter).tc.task)
}

func (e *AgentCommandExecutor) UpdateLastMessageTime() {
	e.agent.comm.UpdateLastMessageTime()
}

func (e *AgentCommandExecutor) StartTimeoutWatcher(ctx context.Context, cancel context.CancelFunc, opts executor.TimeoutWatcherOptions) {
	tcAdapter := opts.ExecutionContext.(*taskContextAdapter)
	timeoutOpts := timeoutWatcherOptions{
		tc:                    tcAdapter.tc,
		kind:                  opts.Kind,
		getTimeout:            opts.GetTimeout,
		canMarkTimeoutFailure: opts.CanMarkTimeoutFailure,
	}
	e.agent.startTimeoutWatcher(ctx, cancel, timeoutOpts)
}

func (e *AgentCommandExecutor) RunCommand(ctx context.Context, execCtx executor.ExecutionContext, commandInfo model.PluginCommandConf, cmd command.Command, options executor.RunCommandsOptions) error {
	tcAdapter := execCtx.(*taskContextAdapter)
	runOpts := runCommandsOptions{
		block:       options.Block,
		canFailTask: options.CanFailTask,
	}
	return e.agent.runCommand(ctx, tcAdapter.tc, commandInfo, cmd, runOpts)
}

// RunCommandOrFunc initializes and executes a list of commands
func (e *AgentCommandExecutor) RunCommandOrFunc(ctx context.Context, execCtx executor.ExecutionContext, commandInfo model.PluginCommandConf, cmds []command.Command, options executor.RunCommandsOptions) error {
	tcAdapter := execCtx.(*taskContextAdapter)
	runOpts := runCommandsOptions{
		block:       options.Block,
		canFailTask: options.CanFailTask,
	}
	return e.agent.runCommandOrFunc(ctx, tcAdapter.tc, commandInfo, cmds, runOpts)
}

// taskContextAdapter adapts taskContext to implement ExecutionContext
type taskContextAdapter struct {
	tc *taskContext
}

func NewTaskContextAdapter(tc *taskContext) executor.ExecutionContext {
	return &taskContextAdapter{tc: tc}
}

func (a *taskContextAdapter) GetTaskConfig() *internal.TaskConfig {
	return a.tc.taskConfig
}

func (a *taskContextAdapter) GetTaskData() executor.TaskData {
	return a
}

func (a *taskContextAdapter) GetID() string {
	return a.tc.task.ID
}

func (a *taskContextAdapter) GetSecret() string {
	return a.tc.task.Secret
}

func (a *taskContextAdapter) SetCurrentCommand(cmd command.Command) {
	a.tc.setCurrentCommand(cmd)
}

func (a *taskContextAdapter) GetCurrentCommand() command.Command {
	return a.tc.getCurrentCommand()
}

func (a *taskContextAdapter) SetFailingCommand(cmd command.Command) {
	a.tc.setFailingCommand(cmd)
}

func (a *taskContextAdapter) GetFailingCommand() command.Command {
	return a.tc.getFailingCommand()
}

func (a *taskContextAdapter) AddOtherFailingCommand(cmd command.Command) {
	a.tc.addFailingCommand(cmd)
}

func (a *taskContextAdapter) GetTaskLogger() grip.Journaler {
	if a.tc.logger != nil {
		return a.tc.logger.Task()
	}
	return grip.GetDefaultJournaler()
}

func (a *taskContextAdapter) GetExecutionLogger() grip.Journaler {
	if a.tc.logger != nil {
		return a.tc.logger.Execution()
	}
	return grip.GetDefaultJournaler()
}

func (a *taskContextAdapter) SetHeartbeatTimeout(opts executor.HeartbeatTimeoutOptions) {
	a.tc.setHeartbeatTimeout(heartbeatTimeoutOptions{
		startAt:    opts.StartAt,
		getTimeout: opts.GetTimeout,
		kind:       opts.Kind,
	})
}

func (a *taskContextAdapter) SetIdleTimeout(timeout time.Duration) {
	a.tc.setIdleTimeout(timeout)
}

func (a *taskContextAdapter) HadTimedOut() bool {
	return a.tc.hadTimedOut()
}

func (a *taskContextAdapter) GetTimeoutType() globals.TimeoutType {
	return a.tc.getTimeoutType()
}

func (a *taskContextAdapter) GetTimeoutDuration() time.Duration {
	return a.tc.getTimeoutDuration()
}

func (a *taskContextAdapter) AddTaskCommandCleanups(cleanups []internal.CommandCleanup) {
	a.tc.addTaskCommandCleanups(cleanups)
}

func (a *taskContextAdapter) GetExpansions() *util.Expansions {
	return &a.tc.taskConfig.Expansions
}

func (a *taskContextAdapter) GetNewExpansions() *util.Expansions {
	return &a.tc.taskConfig.NewExpansions.Expansions
}

func (a *taskContextAdapter) GetDynamicExpansions() *util.Expansions {
	return &a.tc.taskConfig.DynamicExpansions
}
