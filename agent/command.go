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
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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

type runCommandsOptions struct {
	isTaskCommands bool
	failPreAndPost bool
}

// runCommandsInBlock runs all the commands listed in a block (e.g. pre, post).
func (a *Agent) runCommandsInBlock(ctx context.Context, tc *taskContext, commands []model.PluginCommandConf,
	options runCommandsOptions, block string) (err error) {
	var cmds []command.Command
	defer func() {
		if pErr := recovery.HandlePanicWithError(recover(), nil, "running commands"); pErr != nil {
			msg := message.Fields{
				"message":   "programmatic error: task panicked while running commands",
				"operation": "running commands",
				"block":     block,
				"stack":     message.NewStack(2, "").Raw(),
			}
			grip.Alert(message.WrapError(pErr, msg))
			tc.logger.Execution().Error("programmatic error: Evergreen agent hit a runtime panic while running commands")
			catcher := grip.NewBasicCatcher()
			catcher.Add(pErr)
			catcher.Add(err)
			err = catcher.Resolve()
		}
	}()

	for i, commandInfo := range commands {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "canceled while running commands")
		}
		blockInfo := command.BlockInfo{
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

// runCommandOrFunc runs a list of commands, which can either be single
// standalone command or a list of sub-commands in a function.
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
			grip.Error(errors.Wrap(logger.Close(), "closing command logger"))
		}()
	}

	// Omit the block name from the display name, or else the display name
	// becomes too verbose.
	blockInfo.Block = ""

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

		// kim: TODO: test that command name still displays the same as before.
		funcInfo := command.FunctionInfo{
			Function:     commandInfo.Function,
			SubCmdNum:    idx + 1,
			TotalSubCmds: len(cmds),
		}
		// kim: TODO: determine if logging the block name is too noisy, in which
		// case it can be omitted.
		// Don't use the command's display name here because it can include the
		// block name, which makes it very verbose.
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

	for key, val := range commandInfo.Vars {
		var newVal string
		newVal, err := tc.taskConfig.Expansions.ExpandString(val)
		if err != nil {
			return errors.Wrapf(err, "expanding '%s'", val)
		}
		tc.taskConfig.Expansions.Put(key, newVal)
	}

	if options.isTaskCommands || options.failPreAndPost {
		tc.setCurrentCommand(cmd)
		tc.setCurrentIdleTimeout(cmd)
		a.comm.UpdateLastMessageTime()
	} else {
		tc.setCurrentIdleTimeout(nil)
	}

	start := time.Now()
	// We have seen cases where calling exec.*Cmd.Wait() waits for too long if
	// the process has called subprocesses. It will wait until a subprocess
	// finishes, instead of returning immediately when the context is canceled.
	// We therefore check both if the context is cancelled and if Wait() has finished.
	cmdChan := make(chan error, 1)
	go func() {
		defer func() {
			// this channel will get read from twice even though we only send once, hence why it's buffered
			cmdChan <- recovery.HandlePanicWithError(recover(), nil,
				fmt.Sprintf("running command %s", displayName))
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
		if ctx.Err() == context.DeadlineExceeded {
			tc.logger.Task().Errorf("Command %s stopped early because idle timeout duration of %d seconds has been reached.", displayName, int(tc.timeout.idleTimeoutDuration.Seconds()))
		} else {
			tc.logger.Task().Errorf("Command %s stopped early: %s.", displayName, ctx.Err())
		}
		return errors.Wrap(ctx.Err(), "agent stopped early")
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

// kim: TODO: delete
// getCommandAndFunctionName gets a display name for a command. If the command
// is run within a function, it includes the function context
//
//	func getCommandAndFunctionName(commandInfo model.PluginCommandConf, cmd command.Command) string {
//	    // kim: TODO: determine if this can use cmd.DisplayName, may need to
//	    // guarantee cmd.DisplayName gets set.
//	    commandName := fmt.Sprintf(`'%s'`, cmd.Name())
//	    if commandInfo.DisplayName != "" {
//	        commandName = fmt.Sprintf(`'%s' (%s)`, commandInfo.DisplayName, commandName)
//	    }
//	    if commandInfo.Function != "" {
//	        commandName = fmt.Sprintf(`%s in function '%s'`, commandName, commandInfo.Function)
//	    }
//
//	    return commandName
//	}
//

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
