package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
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
	commandNameAttribute  = fmt.Sprintf("%s.command_name", commandsAttribute)
	functionNameAttribute = fmt.Sprintf("%s.function_name", commandsAttribute)
)

// TODO (EVG-20629): remember to move the block timeout watcher into a helper
// for runCommandsInBlock to reduce duplication.
type runCommandsOptions struct {
	isTaskCommands bool
	failPreAndPost bool
}

// runCommandsInBlock runs all the commands listed in a block (e.g. pre, post).
func (a *Agent) runCommandsInBlock(ctx context.Context, tc *taskContext, commands []model.PluginCommandConf,
	options runCommandsOptions, block string) (err error) {

	defer func() {
		op := fmt.Sprintf("running commands for block '%s'", block)
		pErr := recovery.HandlePanicWithError(recover(), nil, op)
		if pErr == nil {
			return
		}
		err = a.logPanic(tc.logger, pErr, err, op)
	}()

	for i, commandInfo := range commands {
		var cmds []command.Command
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "canceled while running commands")
		}
		blockInfo := command.BlockInfo{
			Block:     block,
			CmdNum:    i + 1,
			TotalCmds: len(commands),
		}
		cmds, err = command.Render(commandInfo, tc.taskConfig.Project, blockInfo)
		if err != nil {
			return errors.Wrapf(err, "rendering command '%s'", commandInfo.Command)
		}
		if err = a.runCommandOrFunc(ctx, tc, commandInfo, cmds, options, blockInfo); err != nil {
			return errors.WithStack(err)
		}
	}

	return errors.WithStack(err)
}

// runCommandOrFunc initializes and then executes a list of commands, which can
// either be a single standalone command or a list of sub-commands in a
// function.
func (a *Agent) runCommandOrFunc(ctx context.Context, tc *taskContext, commandInfo model.PluginCommandConf,
	cmds []command.Command, options runCommandsOptions, blockInfo command.BlockInfo) error {

	var err error
	var logger client.LoggerProducer
	// if there is a command-specific logger, make it here otherwise use the task-level logger
	if commandInfo.Loggers == nil {
		logger = tc.logger
	} else {
		logger, err = a.makeLoggerProducer(ctx, tc, commandInfo.Loggers, getCommandNameForFileLogger(commandInfo))
		if err != nil {
			return errors.Wrap(err, "making command logger")
		}
		defer func() {
			// If the logger is a command-specific logger, when the command
			// finishes, the loggers should have no more logs to send. Closing
			// it ensure that the command logger flushes all logs and cleans up.
			grip.Error(errors.Wrap(logger.Close(), "closing command logger"))
		}()
	}

	if commandInfo.Function != "" {
		var commandSetSpan trace.Span
		ctx, commandSetSpan = a.tracer.Start(ctx, fmt.Sprintf("function: '%s'", commandInfo.Function), trace.WithAttributes(
			attribute.String(functionNameAttribute, commandInfo.Function),
		))
		defer commandSetSpan.End()
	}

	for idx, cmd := range cmds {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "canceled while running command list")
		}

		funcInfo := command.FunctionInfo{
			Function:     commandInfo.Function,
			SubCmdNum:    idx + 1,
			TotalSubCmds: len(cmds),
		}
		// Avoid using the command's display name here if the user has
		// explicitly configured it, because the user-defined display name may
		// be ambiguous (e.g. if the command runs in multiple different blocks
		// or functions).
		displayName := command.GetDefaultDisplayName(cmd.Name(), blockInfo, funcInfo)

		if !commandInfo.RunOnVariant(tc.taskConfig.BuildVariant.Name) {
			tc.logger.Task().Infof("Skipping command %s on variant %s.", displayName, tc.taskConfig.BuildVariant.Name)
			continue
		}

		tc.logger.Task().Infof("Running command %s.", displayName)

		ctx, commandSpan := a.tracer.Start(ctx, cmd.Name(), trace.WithAttributes(
			attribute.String(commandNameAttribute, cmd.Name()),
		))
		tc.taskConfig.Expansions.Put(otelTraceIDExpansion, commandSpan.SpanContext().TraceID().String())
		tc.taskConfig.Expansions.Put(otelParentIDExpansion, commandSpan.SpanContext().SpanID().String())
		tc.taskConfig.Expansions.Put(otelCollectorEndpointExpansion, a.opts.TraceCollectorEndpoint)

		cmd.SetJasperManager(a.jasper)

		if err := a.runCommand(ctx, tc, logger, commandInfo, cmd, displayName, options); err != nil {
			commandSpan.SetStatus(codes.Error, "running command")
			commandSpan.RecordError(err, trace.WithAttributes(tc.taskConfig.TaskAttributes()...))
			commandSpan.End()
			return errors.Wrap(err, "running command")
		}
		commandSpan.End()
	}
	return nil
}

// runCommand runs a single command, which is either a standalone command or a
// single sub-command within a function.
func (a *Agent) runCommand(ctx context.Context, tc *taskContext, logger client.LoggerProducer, commandInfo model.PluginCommandConf,
	cmd command.Command, displayName string, options runCommandsOptions) error {
	prevExp := map[string]string{}
	for key, val := range commandInfo.Vars {
		prevVal := tc.taskConfig.Expansions.Get(key)
		prevExp[key] = prevVal

		var newVal string
		newVal, err := tc.taskConfig.Expansions.ExpandString(val)
		if err != nil {
			return errors.Wrapf(err, "expanding '%s'", val)
		}
		tc.taskConfig.Expansions.Put(key, newVal)
	}
	defer func() {
		if !tc.unsetFunctionVarsDisabled || tc.project.UnsetFunctionVars {
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
			tc.taskConfig.Expansions.Update(prevExp)
			tc.taskConfig.DynamicExpansions = *util.NewExpansions(map[string]string{})
		}
	}()

	tc.setCurrentCommand(cmd)
	tc.setCurrentIdleTimeout(cmd)
	a.comm.UpdateLastMessageTime()

	start := time.Now()
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
			op := fmt.Sprintf("running command %s", displayName)
			pErr := recovery.HandlePanicWithError(recover(), nil, op)
			if pErr == nil {
				return
			}
			_ = a.logPanic(tc.logger, pErr, nil, op)

			cmdChan <- pErr
		}()

		cmdChan <- cmd.Execute(ctx, a.comm, logger, tc.taskConfig)
	}()
	select {
	case err := <-cmdChan:
		if err != nil {
			tc.logger.Task().Errorf("Command %s failed: %s.", displayName, err)
			if options.isTaskCommands || options.failPreAndPost ||
				(cmd.Name() == "git.get_project" && tc.taskModel.Requester == evergreen.MergeTestRequester) {
				// any git.get_project in the commit queue should fail
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

		tc.logger.Task().Errorf("Command %s stopped early: %s.", displayName, ctx.Err())
		return errors.Wrap(ctx.Err(), "command stopped early")
	}
	tc.logger.Task().Infof("Finished command %s in %s.", displayName, time.Since(start).String())
	if (options.isTaskCommands || options.failPreAndPost) && a.endTaskResp != nil && !a.endTaskResp.ShouldContinue {
		// only error if we're running a command that should fail, and we don't want to continue to run other tasks
		return errors.Errorf("task status has been set to '%s'; triggering end task", a.endTaskResp.Status)
	}

	return nil
}

// runTaskCommands runs all commands for the task currently assigned to the agent.
func (a *Agent) runTaskCommands(ctx context.Context, tc *taskContext) error {
	ctx, span := a.tracer.Start(ctx, "task-commands")
	defer span.End()

	conf := tc.taskConfig
	task := conf.Project.FindProjectTask(conf.Task.DisplayName)

	if task == nil {
		return errors.Errorf("unable to find task '%s' in project '%s'", conf.Task.DisplayName, conf.Task.Project)
	}

	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "canceled while running task commands")
	}
	tc.logger.Execution().Info("Running task commands.")
	start := time.Now()
	opts := runCommandsOptions{isTaskCommands: true}
	err := a.runCommandsInBlock(ctx, tc, task.Commands, opts, "")
	tc.logger.Execution().Infof("Finished running task commands in %s.", time.Since(start).String())
	if err != nil {
		return err
	}
	return nil
}

// getCommandNameForFileLogger gets the name of the command that should be used
// when the file logger is being used.
func getCommandNameForFileLogger(commandInfo model.PluginCommandConf) string {
	if commandInfo.DisplayName != "" {
		return commandInfo.DisplayName
	}
	if commandInfo.Function != "" {
		return commandInfo.Function
	}
	if commandInfo.Command != "" {
		return commandInfo.Command
	}
	return "unknown function"
}

// endTaskSyncCommands returns the commands to sync the task to S3 if it was
// requested when the task completes.
func endTaskSyncCommands(tc *taskContext, detail *apimodels.TaskEndDetail) *model.YAMLCommandSet {
	if tc.taskModel == nil {
		tc.logger.Task().Error("Task model not found for running task sync.")
		return nil
	}
	if !tc.taskModel.SyncAtEndOpts.Enabled {
		return nil
	}
	if statusFilter := tc.taskModel.SyncAtEndOpts.Statuses; len(statusFilter) != 0 {
		if detail.Status == evergreen.TaskSucceeded {
			if !utility.StringSliceContains(statusFilter, evergreen.TaskSucceeded) {
				return nil
			}
		} else if !utility.StringSliceContains(statusFilter, evergreen.TaskFailed) {
			return nil
		}
	}
	return &model.YAMLCommandSet{
		SingleCommand: &model.PluginCommandConf{
			Type:    evergreen.CommandTypeSetup,
			Command: evergreen.S3PushCommandName,
		},
	}
}
