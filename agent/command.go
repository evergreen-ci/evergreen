package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

type runCommandsOptions struct {
	isTaskCommands   bool
	shouldSetupFail  bool
	setupTimeoutSecs int
}

func (a *Agent) runCommands(ctx context.Context, tc *taskContext, commands []model.PluginCommandConf,
	options runCommandsOptions) (err error) {
	var cmds []command.Command
	defer func() { err = recovery.HandlePanicWithError(recover(), err, "run commands") }()

	for i, commandInfo := range commands {
		if ctx.Err() != nil {
			grip.Error("runCommands canceled")
			return errors.New("runCommands canceled")
		}

		cmds, err = command.Render(commandInfo, tc.taskConfig.Project.Functions)
		if err != nil {
			tc.logger.Task().Errorf("Couldn't parse plugin command '%v': %v", commandInfo.Command, err)
			return err
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
			return errors.Wrap(err, "error making logger")
		}
		defer func() {
			grip.Error(logger.Close())
		}()
	}
	for idx, cmd := range cmds {
		if ctx.Err() != nil {
			grip.Error("runCommands canceled")
			return errors.New("runCommands canceled")
		}

		// SetType implementations only modify the
		// command's type *if* the command's type is
		// not otherwise set.
		cmd.SetType(tc.taskConfig.Project.CommandType)
		cmd.SetJasperManager(a.jasper)

		fullCommandName := getCommandName(commandInfo, cmd)

		if !commandInfo.RunOnVariant(tc.taskConfig.BuildVariant.Name) {
			tc.logger.Task().Infof("Skipping command %s on variant %s (step %d of %d)",
				fullCommandName, tc.taskConfig.BuildVariant.Name, index, total)
			continue
		}

		if len(cmds) == 1 {
			tc.logger.Task().Infof("Running command %s (step %d of %d)", fullCommandName, index, total)
		} else {
			// for functions with more than one command
			tc.logger.Task().Infof("Running command %v (step %d.%d of %d)", fullCommandName, index, idx+1, total)
		}

		for key, val := range commandInfo.Vars {
			var newVal string
			newVal, err = tc.taskConfig.Expansions.ExpandString(val)
			if err != nil {
				return errors.Wrapf(err, "Can't expand '%v'", val)
			}
			tc.taskConfig.Expansions.Put(key, newVal)
		}

		if options.isTaskCommands {
			tc.setCurrentCommand(cmd)
			tc.setCurrentTimeout(cmd)
			a.comm.UpdateLastMessageTime()
		} else {
			tc.setCurrentTimeout(nil)
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
					fmt.Sprintf("problem running command '%s'", cmd.Name()))
			}()
			cmdChan <- cmd.Execute(ctx, a.comm, logger, tc.taskConfig)
		}()
		select {
		case err = <-cmdChan:
			if err != nil {
				tc.logger.Task().Errorf("Command failed: %v", err)
				if options.isTaskCommands || options.shouldSetupFail {
					return errors.Wrap(err, "command failed")
				}
			}
		case <-ctx.Done():
			tc.logger.Task().Errorf("Command stopped early: %s", ctx.Err())
			return errors.Wrap(ctx.Err(), "Agent stopped early")
		}
		tc.logger.Execution().Infof("Finished %s in %s", fullCommandName, time.Since(start).String())
	}
	return nil
}

// runTaskCommands runs all commands for the task currently assigned to the agent and
// returns the task status
func (a *Agent) runTaskCommands(ctx context.Context, tc *taskContext) error {
	conf := tc.taskConfig
	task := conf.Project.FindProjectTask(conf.Task.DisplayName)

	if task == nil {
		tc.logger.Execution().Errorf("Can't find task: %v", conf.Task.DisplayName)
		return errors.New("unable to find task")
	}

	if ctx.Err() != nil {
		grip.Error("task canceled")
		return errors.New("task canceled")
	}
	tc.logger.Execution().Info("Running task commands.")
	start := time.Now()
	opts := runCommandsOptions{isTaskCommands: true}
	err := a.runCommands(ctx, tc, task.Commands, opts)
	tc.logger.Execution().Infof("Finished running task commands in %v.", time.Since(start).String())
	if err != nil {
		tc.logger.Execution().Errorf("Task failed: %v", err)
		return errors.New("task failed")
	}
	return nil
}

func getCommandName(commandInfo model.PluginCommandConf, cmd command.Command) string {
	commandName := cmd.Name()
	if commandInfo.Function != "" {
		commandName = fmt.Sprintf(`'%s' in "%s"`, commandName, commandInfo.Function)
	} else if commandInfo.DisplayName != "" {
		commandName = fmt.Sprintf(`("%s") %s`, commandInfo.DisplayName, commandName)
	} else {
		commandName = fmt.Sprintf("'%s'", commandName)
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
