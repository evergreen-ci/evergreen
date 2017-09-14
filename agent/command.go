package agent

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (a *Agent) runCommands(ctx context.Context, tc *taskContext, commands []model.PluginCommandConf, returnOnError bool, idleTimeout chan<- time.Duration) error {
	for i, commandInfo := range commands {
		if ctx.Err() != nil {
			grip.Error("task canceled")
			return errors.New("runCommands canceled")
		}

		cmds, err := command.Render(commandInfo, tc.taskConfig.Project.Functions)
		if err != nil {
			tc.logger.Task().Errorf("Couldn't parse plugin command '%v': %v", commandInfo.Command, err)
			if returnOnError {
				return err
			}
			continue
		}

		for idx, cmd := range cmds {
			if ctx.Err() != nil {
				grip.Error("task canceled")
				return errors.New("runCommands canceled")
			}

			// SetType implementations only modify the
			// command's type *if* the command's type is
			// not otherwise set.
			cmd.SetType(tc.taskConfig.Project.CommandType)

			fullCommandName := a.getCommandName(commandInfo, cmd)

			if !commandInfo.RunOnVariant(tc.taskConfig.BuildVariant.Name) {
				tc.logger.Task().Infof("Skipping command %s on variant %s (step %d of %d)",
					fullCommandName, tc.taskConfig.BuildVariant.Name, i+1, len(commands))
				continue
			}

			if len(cmds) == 1 {
				tc.logger.Task().Infof("Running command %s (step %d of %d)", fullCommandName, i+1, len(commands))
			} else {
				// for functions with more than one command
				tc.logger.Task().Infof("Running command %v (step %d.%d of %d)", fullCommandName, i+1, idx+1, len(commands))
			}

			timeoutPeriod := a.getTimeout(&commandInfo)

			for key, val := range commandInfo.Vars {
				var newVal string
				newVal, err = tc.taskConfig.Expansions.ExpandString(val)
				if err != nil {
					return errors.Wrapf(err, "Can't expand '%v'", val)
				}
				tc.taskConfig.Expansions.Put(key, newVal)
			}

			if idleTimeout != nil {
				a.checkIn(ctx, tc, cmd, timeoutPeriod, idleTimeout)
			}

			start := time.Now()
			err = cmd.Execute(ctx, a.comm, tc.logger, tc.taskConfig)

			tc.logger.Execution().Infof("Finished %v in %v", fullCommandName, time.Since(start).String())

			if err != nil {
				tc.logger.Task().Errorf("Command failed: %v", err)
				if returnOnError {
					return errors.Wrap(err, "command failed")
				}
			}
		}
	}
	return nil
}

// runTaskCommands runs all commands for the task currently assigned to the agent and
// returns the task status
func (a *Agent) runTaskCommands(ctx context.Context, tc *taskContext, idleTimeout chan<- time.Duration) error {
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
	err := a.runCommands(ctx, tc, task.Commands, true, idleTimeout)
	tc.logger.Execution().Infof("Finished running task commands in %v.", time.Since(start).String())
	if err != nil {
		tc.logger.Execution().Errorf("Task failed: %v", err)
		return errors.New("task failed")
	}
	return nil
}

func (a *Agent) getTimeout(commandInfo *model.PluginCommandConf) time.Duration {
	var timeoutPeriod = defaultCmdTimeout
	if commandInfo.TimeoutSecs > 0 {
		timeoutPeriod = time.Duration(commandInfo.TimeoutSecs) * time.Second
	}
	return timeoutPeriod
}

func (a *Agent) getCommandName(commandInfo model.PluginCommandConf, cmd command.Command) string {
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
