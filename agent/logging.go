package agent

import (
	"context"
	"fmt"
	"os"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

var idSource chan int

func init() {
	idSource = make(chan int, 100)

	go func() {
		id := 0
		for {
			idSource <- id
			id++
		}
	}()
}

func getInc() int { return <-idSource }

// GetSender configures the agent's local logging to a file.
func GetSender(ctx context.Context, prefix, taskId string) (send.Sender, error) {
	var (
		err     error
		sender  send.Sender
		senders []send.Sender
	)

	if os.Getenv(subprocess.MarkerAgentPID) == "" { // this var is set if the agent is started via a command
		if splunk := send.GetSplunkConnectionInfo(); splunk.Populated() {
			grip.Info("configuring splunk sender")
			sender, err = send.NewSplunkLogger("evergreen.agent", splunk, send.LevelInfo{Default: level.Alert, Threshold: level.Alert})
			if err != nil {
				return nil, errors.Wrap(err, "problem creating the splunk logger")
			}
			senders = append(senders, sender)
		}
	} else {
		grip.Notice("agent started via command - not configuring external logger")
	}

	if prefix == "" {
		// pass
	} else if prefix == evergreen.LocalLoggingOverride || prefix == "--" || prefix == evergreen.StandardOutputLoggingOverride {
		sender, err = send.NewNativeLogger("evergreen.agent", send.LevelInfo{Default: level.Info, Threshold: level.Debug})
		if err != nil {
			return nil, errors.Wrap(err, "problem creating a native console logger")
		}

		senders = append(senders, sender)
	} else {
		sender, err = send.NewFileLogger("evergreen.agent",
			fmt.Sprintf("%s-%d-%d.log", prefix, os.Getpid(), getInc()), send.LevelInfo{Default: level.Info, Threshold: level.Debug})
		if err != nil {
			return nil, errors.Wrap(err, "problem creating a file logger")
		}

		senders = append(senders, sender)
	}

	return send.NewConfiguredMultiSender(senders...), nil
}

func (a *Agent) makeLoggerProducer(ctx context.Context, c *model.LoggerConfig, tc *taskContext, td client.TaskData) client.LoggerProducer {
	if c == nil {
		return tc.logger
	}
	config := convertLoggerConfig(*c, a.opts.WorkingDirectory)
	return a.comm.GetLoggerProducer(ctx, td, &config)
}

// TODO: configuration for logkeeper
func convertLoggerConfig(c model.LoggerConfig, workdir string) client.LoggerConfig {
	config := client.LoggerConfig{}
	for _, agentConfig := range c.Agent {
		config.Agent = append(config.Agent, client.LogOpts{
			Sender:          model.LogSender(agentConfig.Type),
			SplunkServerURL: agentConfig.SplunkServer,
			SplunkToken:     agentConfig.SplunkToken,
			Filepath:        fmt.Sprintf("%s/agent.log", workdir),
		})
	}
	for _, systemConfig := range c.System {
		config.System = append(config.System, client.LogOpts{
			Sender:          model.LogSender(systemConfig.Type),
			SplunkServerURL: systemConfig.SplunkServer,
			SplunkToken:     systemConfig.SplunkToken,
			Filepath:        fmt.Sprintf("%s/system.log", workdir),
		})
	}
	for _, taskConfig := range c.Task {
		config.Task = append(config.Task, client.LogOpts{
			Sender:          model.LogSender(taskConfig.Type),
			SplunkServerURL: taskConfig.SplunkServer,
			SplunkToken:     taskConfig.SplunkToken,
			Filepath:        fmt.Sprintf("%s/task.log", workdir),
		})
	}
	return config
}
