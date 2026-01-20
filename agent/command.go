package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/executor"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	commandsAttribute = "evergreen.command"

	otelTraceIDExpansion           = "otel_trace_id"
	otelParentIDExpansion          = "otel_parent_id"
	otelCollectorEndpointExpansion = "otel_collector_endpoint"
)

var (
	commandNameAttribute        = fmt.Sprintf("%s.command_name", commandsAttribute)
	commandDisplayNameAttribute = fmt.Sprintf("%s.command_display_name", commandsAttribute)
	functionNameAttribute       = fmt.Sprintf("%s.function_name", commandsAttribute)
)

type runCommandsOptions struct {
	// block is the name of the block that the command runs in.
	block command.BlockType
	// canFailTask indicates whether the command can fail the task.
	canFailTask bool
}

// runCommandsInBlock runs all the commands listed in a block (e.g. pre, post).
func (a *Agent) runCommandsInBlock(ctx context.Context, tc *taskContext, cmdBlock commandBlock) error {
	var taskLogger, execLogger grip.Journaler
	if tc.logger != nil {
		taskLogger = tc.logger.Task()
		execLogger = tc.logger.Execution()
	} else {
		// In the case of teardown group, it's not guaranteed that the agent has
		// previously set up a valid logger, so use the fallback default logger.
		taskLogger = grip.GetDefaultJournaler()
		execLogger = grip.GetDefaultJournaler()
	}

	deps := executor.BlockExecutorDeps{
		JasperManager: a.jasper,
		Tracer:        a.tracer,
		TaskLogger:    taskLogger,
		ExecLogger:    execLogger,
		TaskConfig:    tc.taskConfig,

		StartTimeoutWatcher: func(ctx context.Context, cancel context.CancelFunc, kind globals.TimeoutType, getTimeout func() time.Duration, canMarkFailure bool) {
			opts := timeoutWatcherOptions{
				tc:                    tc,
				kind:                  kind,
				getTimeout:            getTimeout,
				canMarkTimeoutFailure: canMarkFailure,
			}
			a.startTimeoutWatcher(ctx, cancel, opts)
		},
		SetHeartbeatTimeout: func(startAt time.Time, getTimeout func() time.Duration, kind globals.TimeoutType) {
			tc.setHeartbeatTimeout(heartbeatTimeoutOptions{
				startAt:    startAt,
				getTimeout: getTimeout,
				kind:       kind,
			})
		},
		ResetHeartbeatTimeout: func() {
			tc.setHeartbeatTimeout(heartbeatTimeoutOptions{})
		},
		HandlePanic: func(panicErr error, originalErr error, op string) error {
			return a.logPanic(tc, panicErr, originalErr, op)
		},
		RunCommandOrFunc: func(ctx context.Context, commandInfo model.PluginCommandConf, cmds []command.Command, block command.BlockType, canFailTask bool) error {
			opts := runCommandsOptions{
				block:       block,
				canFailTask: canFailTask,
			}
			return a.runCommandOrFunc(ctx, tc, commandInfo, cmds, opts)
		},
	}

	execCmdBlock := executor.CommandBlock{
		Block:               cmdBlock.block,
		Commands:            cmdBlock.commands,
		TimeoutKind:         cmdBlock.timeoutKind,
		GetTimeout:          cmdBlock.getTimeout,
		CanTimeOutHeartbeat: cmdBlock.canTimeOutHeartbeat,
		CanFailTask:         cmdBlock.canFailTask,
	}

	return executor.RunCommandsInBlock(ctx, deps, execCmdBlock)
}

// runCommandOrFunc initializes and then executes a list of commands, which can
// either be a single standalone command or a list of sub-commands in a
// function.
func (a *Agent) runCommandOrFunc(ctx context.Context, tc *taskContext, commandInfo model.PluginCommandConf,
	cmds []command.Command, options runCommandsOptions) error {

	var functionSpan trace.Span
	if commandInfo.Function != "" {
		ctx, functionSpan = a.tracer.Start(ctx, resolveFunctionSpan(commandInfo), trace.WithAttributes(
			attribute.String(functionNameAttribute, commandInfo.Function),
		))
		defer functionSpan.End()
	}

	for _, cmd := range cmds {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "canceled while running command list")
		}

		if !commandInfo.RunOnVariant(tc.taskConfig.BuildVariant.Name) {
			tc.logger.Task().Infof("Skipping command %s on variant %s.", cmd.FullDisplayName(), tc.taskConfig.BuildVariant.Name)
			continue
		}

		tc.logger.Task().Infof("Running command %s.", cmd.FullDisplayName())

		ctx, commandSpan := a.tracer.Start(ctx, cmd.Name(), trace.WithAttributes(
			attribute.String(commandNameAttribute, cmd.Name()),
			attribute.String(commandDisplayNameAttribute, cmd.FullDisplayName()),
		))
		tc.taskConfig.NewExpansions.Put(otelTraceIDExpansion, commandSpan.SpanContext().TraceID().String())
		tc.taskConfig.NewExpansions.Put(otelParentIDExpansion, commandSpan.SpanContext().SpanID().String())
		tc.taskConfig.NewExpansions.Put(otelCollectorEndpointExpansion, a.opts.TraceCollectorEndpoint)

		cmd.SetJasperManager(a.jasper)

		if err := a.runCommand(ctx, tc, commandInfo, cmd, options); err != nil {
			commandSpan.SetStatus(codes.Error, "running command")
			commandSpan.RecordError(err, trace.WithAttributes(tc.taskConfig.TaskAttributes()...))
			if commandInfo.Function != "" {
				functionSpan.SetStatus(codes.Error, "running function")
				functionSpan.RecordError(err, trace.WithAttributes(tc.taskConfig.TaskAttributes()...))
			}
			commandSpan.End()
			if cmd.RetryOnFailure() {
				tc.logger.Task().Infof("Command is set to automatically restart on completion, this can be done %d total times per task.", evergreen.MaxAutomaticRestarts)
				if restartErr := a.comm.MarkFailedTaskToRestart(ctx, tc.task); restartErr != nil {
					tc.logger.Task().Errorf("Encountered error marking task to restart upon completion: %s", restartErr)
				}
			}
			return errors.Wrap(err, "running command")
		}
		commandSpan.End()
	}
	return nil
}

// resolveFunctionSpan returns the name of the function to use for the span, if available, otherwise resolves to the default.
func resolveFunctionSpan(commandInfo model.PluginCommandConf) string {
	const defaultFunctionSpan = "function"
	if commandInfo.Function != "" {
		return commandInfo.Function
	}
	return defaultFunctionSpan
}

// runCommand runs a single command, which is either a standalone command or a
// single sub-command within a function.
func (a *Agent) runCommand(ctx context.Context, tc *taskContext, commandInfo model.PluginCommandConf,
	cmd command.Command, options runCommandsOptions) error {
	prevExp := map[string]string{}
	for key, val := range commandInfo.Vars {
		prevVal := tc.taskConfig.Expansions.Get(key)
		prevExp[key] = prevVal

		newVal, err := tc.taskConfig.Expansions.ExpandString(val)
		if err != nil {
			return errors.Wrapf(err, "expanding '%s'", val)
		}
		tc.taskConfig.NewExpansions.Put(key, newVal)
	}
	defer func() {
		// This defer ensures that the function vars do not persist in the expansions after the function is over
		// unless they were updated using expansions.update
		if cmd.Name() == "expansions.update" {
			updatedExpansions := tc.taskConfig.DynamicExpansions.Map()
			for k := range updatedExpansions {
				if _, ok := commandInfo.Vars[k]; ok {
					// If expansions.update updated this key, don't reset it
					delete(prevExp, k)
				}
			}
		}
		tc.taskConfig.NewExpansions.Update(prevExp)
		tc.taskConfig.DynamicExpansions = *util.NewExpansions(map[string]string{})
	}()

	tc.setCurrentCommand(cmd)
	switch options.block {
	case command.PreBlock, command.SetupGroupBlock, command.SetupTaskBlock, command.MainTaskBlock:
		// Only set the idle timeout in cases where the idle timeout is actually
		// respected. In all other blocks, setting the idle timeout should have
		// no effect.
		tc.setCurrentIdleTimeout(cmd)
	}
	a.comm.UpdateLastMessageTime()
	defer func() {
		// After this block is done running, add the command cleanups to the
		// task context from the task config and then clear the task config's
		// command cleanups. Setup group commands are handled differently and
		// should only be cleaned up after the entire group has run while
		// any other block should clean up after the task is done running.
		if options.block == command.SetupGroupBlock {
			tc.addSetupGroupCommandCleanups(tc.taskConfig.GetAndClearCommandCleanups())
		} else {
			tc.addTaskCommandCleanups(tc.taskConfig.GetAndClearCommandCleanups())
		}
	}()

	start := time.Now()
	defer func() {
		tc.logger.Task().Infof("Finished command %s in %s.", cmd.FullDisplayName(), time.Since(start).String())
	}()

	// This method must return soon after the context errors (e.g. due to
	// aborting the task). Even though commands ought to respect the context and
	// finish up quickly when the context errors, we cannot guarantee that every
	// command implementation will respect the context or will finish in a
	// timely manner. Therefore, just in case the command hangs or is slow, run
	// the command in a goroutine so it not stop the task from making forward
	// progress.
	cmdChan := make(chan error, 1)
	go func() {
		defer func() {
			op := fmt.Sprintf("running command %s", cmd.FullDisplayName())
			pErr := recovery.HandlePanicWithError(recover(), nil, op)
			if pErr == nil {
				return
			}
			_ = a.logPanic(tc, pErr, nil, op)

			cmdChan <- pErr
		}()

		cmdChan <- cmd.Execute(ctx, a.comm, tc.logger, tc.taskConfig)
	}()

	select {
	case err := <-cmdChan:
		if err != nil {
			tc.logger.Task().Errorf("Command %s failed: %s.", cmd.FullDisplayName(), err)
			tc.addFailingCommand(cmd)
			if options.block == command.PostBlock {
				tc.setPostErrored(true)
			}
			if options.canFailTask {
				return errors.Wrap(err, "command failed")
			}
		}
	case <-ctx.Done():
		// Make a best-effort attempt to wait for the command to gracefully shut
		// down. Either the command will respect the context and return, or this
		// will time out waiting for the command.
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-cmdChan:
		}

		tc.addFailingCommand(cmd)
		if options.block == command.PostBlock {
			tc.setPostErrored(true)
		}

		tc.logger.Task().Errorf("Command %s stopped early: %s.", cmd.FullDisplayName(), ctx.Err())
		return errors.Wrap(ctx.Err(), "command stopped early")
	}

	userEndTaskResp := tc.getUserEndTaskResponse()
	if options.canFailTask && userEndTaskResp != nil && !userEndTaskResp.ShouldContinue {
		// only error if we're running a command that should fail, and we don't want to continue to run other tasks
		return errors.Errorf("task status has been set to '%s'; triggering end task", userEndTaskResp.Status)
	}

	return nil
}
