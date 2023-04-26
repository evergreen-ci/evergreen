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
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	commandsAttribute = "evergreen.command"
)

var (
	commandNameAttribute  = fmt.Sprintf("%s.command_name", commandsAttribute)
	functionNameAttribute = fmt.Sprintf("%s.function_name", commandsAttribute)
)

type runCommandsOptions struct {
	isTaskCommands bool
	failPreAndPost bool
}

func (a *Agent) runCommands(ctx context.Context, tc *taskContext, commands []model.PluginCommandConf,
	options runCommandsOptions) (err error) {
	var cmds []command.Command
	defer func() { err = recovery.HandlePanicWithError(recover(), err, "run commands") }()

	for i, commandInfo := range commands {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "canceled while running commands")
		}
		cmds, err = command.Render(commandInfo, tc.taskConfig.Project)
		if err != nil {
			return errors.Wrapf(err, "rendering command '%s'", commandInfo.Command)
		}
		if err = a.runCommandSet(ctx, tc, commandInfo, cmds, options, i+1, len(commands)); err != nil {
			return errors.WithStack(err)
		}
	}

	return errors.WithStack(err)
}

func (a *Agent) runCommandSet(ctx context.Context, tc *taskContext, commandInfo model.PluginCommandConf,
	cmds []command.Command, options runCommandsOptions, index, total int) error {

	var err error
	var logger client.LoggerProducer
	// if there is a command-specific logger, make it here otherwise use the task-level logger
	if commandInfo.Loggers == nil {
		logger = tc.logger
	} else {
		logger, err = a.makeLoggerProducer(ctx, tc, commandInfo.Loggers, getFunctionName(commandInfo))
		if err != nil {
			return errors.Wrap(err, "making command logger")
		}
		defer func() {
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
		cmd.SetJasperManager(a.jasper)

		fullCommandName := getCommandName(commandInfo, cmd)
		if !commandInfo.RunOnVariant(tc.taskConfig.BuildVariant.Name) {
			tc.logger.Task().Infof("Skipping command %s on variant %s (step %d of %d).",
				fullCommandName, tc.taskConfig.BuildVariant.Name, index, total)
			continue
		}
		if len(cmds) == 1 {
			tc.logger.Task().Infof("Running command %s (step %d of %d).", fullCommandName, index, total)
		} else {
			// for functions with more than one command
			tc.logger.Task().Infof("Running command %s (step %d.%d of %d).", fullCommandName, index, idx+1, total)
		}

		ctx, commandSpan := a.tracer.Start(ctx, cmd.Name(), trace.WithAttributes(
			attribute.String(commandNameAttribute, cmd.Name()),
		))
		if err := a.runCommand(ctx, tc, logger, commandInfo, cmd, fullCommandName, options); err != nil {
			commandSpan.SetStatus(codes.Error, "running command")
			commandSpan.RecordError(err, trace.WithAttributes(tc.taskConfig.TaskAttributes()...))
			commandSpan.End()
			return errors.Wrap(err, "running command")
		}
		commandSpan.End()
	}
	return nil
}

func (a *Agent) runCommand(ctx context.Context, tc *taskContext, logger client.LoggerProducer, commandInfo model.PluginCommandConf,
	cmd command.Command, fullCommandName string, options runCommandsOptions) error {

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
				fmt.Sprintf("running command %s", fullCommandName))
		}()
		cmdChan <- cmd.Execute(ctx, a.comm, logger, tc.taskConfig)
	}()
	select {
	case err := <-cmdChan:
		if err != nil {
			tc.logger.Task().Errorf("Command %s failed: %s.", fullCommandName, err)
			if options.isTaskCommands || options.failPreAndPost ||
				(cmd.Name() == "git.get_project" && tc.taskModel.Requester == evergreen.MergeTestRequester) {
				// any git.get_project in the commit queue should fail
				return errors.Wrap(err, "command failed")
			}
		}
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			tc.logger.Task().Errorf("Command %s stopped early because idle timeout duration of %d seconds has been reached.", fullCommandName, int(tc.timeout.idleTimeoutDuration.Seconds()))
		} else {
			tc.logger.Task().Errorf("Command %s stopped early: %s.", fullCommandName, ctx.Err())
		}
		return errors.Wrap(ctx.Err(), "agent stopped early")
	}
	tc.logger.Task().Infof("Finished command %s in %s.", fullCommandName, time.Since(start).String())
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
	err := a.runCommands(ctx, tc, task.Commands, opts)
	tc.logger.Execution().Infof("Finished running task commands in %s.", time.Since(start).String())
	if err != nil {
		return err
	}
	return nil
}

func getCommandName(commandInfo model.PluginCommandConf, cmd command.Command) string {
	commandName := fmt.Sprintf(`'%s'`, cmd.Name())
	if commandInfo.DisplayName != "" {
		commandName = fmt.Sprintf(`'%s' (%s)`, commandInfo.DisplayName, commandName)
	}
	if commandInfo.Function != "" {
		commandName = fmt.Sprintf(`%s in function '%s'`, commandName, commandInfo.Function)
	}

	return commandName
}

func getFunctionName(commandInfo model.PluginCommandConf) string {
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
