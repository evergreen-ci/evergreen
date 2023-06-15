package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

const (
	taskLogDirectory  = "evergreen-logs"
	agentLogFileName  = "agent.log"
	systemLogFileName = "system.log"
	taskLogFileName   = "task.log"
)

var (
	idSource chan int
)

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
func (a *Agent) GetSender(ctx context.Context, prefix string) (send.Sender, error) {
	var senders []send.Sender

	if a.opts.SetupData.SplunkClientToken != "" && a.opts.SetupData.SplunkServerURL != "" && a.opts.SetupData.SplunkChannel != "" {
		info := send.SplunkConnectionInfo{
			ServerURL: a.opts.SetupData.SplunkServerURL,
			Token:     a.opts.SetupData.SplunkClientToken,
			Channel:   a.opts.SetupData.SplunkChannel,
		}
		grip.Info("Configuring splunk sender.")
		sender, err := send.NewSplunkLogger("evergreen.agent", info, send.LevelInfo{Default: level.Alert, Threshold: level.Alert})
		if err != nil {
			return nil, errors.Wrap(err, "creating Splunk logger")
		}
		senders = append(senders, sender)
	}

	if prefix == "" {
		// pass
	} else if prefix == evergreen.LocalLoggingOverride || prefix == "--" || prefix == evergreen.StandardOutputLoggingOverride {
		sender, err := send.NewNativeLogger("evergreen.agent", send.LevelInfo{Default: level.Info, Threshold: level.Debug})
		if err != nil {
			return nil, errors.Wrap(err, "creating native console logger")
		}

		senders = append(senders, sender)
	} else {
		sender, err := send.NewFileLogger("evergreen.agent",
			fmt.Sprintf("%s-%d-%d.log", prefix, os.Getpid(), getInc()), send.LevelInfo{Default: level.Info, Threshold: level.Debug})
		if err != nil {
			return nil, errors.Wrap(err, "creating file logger")
		}

		senders = append(senders, sender)
	}

	return send.NewConfiguredMultiSender(senders...), nil
}

func (a *Agent) SetDefaultLogger(sender send.Sender) {
	a.defaultLogger = sender
}

func (a *Agent) makeLoggerProducer(ctx context.Context, tc *taskContext, c *model.LoggerConfig, commandName string) (client.LoggerProducer, error) {
	config := a.prepLogger(tc, c, commandName)

	logger, err := a.comm.GetLoggerProducer(ctx, tc.task, &config)
	if err != nil {
		return nil, err
	}
	loggerData := a.comm.GetLoggerMetadata()
	tc.logs = &apimodels.TaskLogs{}
	for _, agent := range loggerData.Agent {
		tc.logs.AgentLogURLs = append(tc.logs.AgentLogURLs, apimodels.LogInfo{
			Command: commandName,
			URL:     fmt.Sprintf("%s/build/%s/test/%s", a.opts.LogkeeperURL, agent.Build, agent.Test),
		})
	}
	for _, system := range loggerData.System {
		tc.logs.SystemLogURLs = append(tc.logs.SystemLogURLs, apimodels.LogInfo{
			Command: commandName,
			URL:     fmt.Sprintf("%s/build/%s/test/%s", a.opts.LogkeeperURL, system.Build, system.Test),
		})
	}
	for _, task := range loggerData.Task {
		tc.logs.TaskLogURLs = append(tc.logs.TaskLogURLs, apimodels.LogInfo{
			Command: commandName,
			URL:     fmt.Sprintf("%s/build/%s/test/%s", a.opts.LogkeeperURL, task.Build, task.Test),
		})
	}
	return logger, nil
}

func (a *Agent) prepLogger(tc *taskContext, c *model.LoggerConfig, commandName string) client.LoggerConfig {
	logDir := filepath.Join(a.opts.WorkingDirectory, taskLogDirectory)
	grip.Error(errors.Wrapf(os.MkdirAll(logDir, os.ModeDir|os.ModePerm), "making log directory '%s'", logDir))
	// if this is a command-specific logger, create a dir for the command's logs separate from the overall task
	if commandName != "" {
		logDir = filepath.Join(logDir, commandName)
		grip.Error(errors.Wrapf(os.MkdirAll(logDir, os.ModeDir|os.ModePerm), "making log directory '%s' for command '%s'", logDir, commandName))
	}
	config := client.LoggerConfig{}

	var defaultLogger string
	if tc.taskConfig != nil && tc.taskConfig.ProjectRef != nil {
		defaultLogger = tc.taskConfig.ProjectRef.DefaultLogger
	}
	if !model.IsValidDefaultLogger(defaultLogger) {
		grip.Warningf("Default logger '%s' is not valid, setting Evergreen logger as default.", defaultLogger)
		defaultLogger = model.EvergreenLogSender
	}
	if len(c.Agent) == 0 {
		c.Agent = []model.LogOpts{{Type: defaultLogger}}
	}
	if len(c.System) == 0 {
		c.System = []model.LogOpts{{Type: defaultLogger}}
	}
	if len(c.Task) == 0 {
		c.Task = []model.LogOpts{{Type: defaultLogger}}
	}

	for _, agentConfig := range c.Agent {
		config.Agent = append(config.Agent, a.prepSingleLogger(tc, agentConfig, logDir, agentLogFileName))
	}
	for _, systemConfig := range c.System {
		config.System = append(config.System, a.prepSingleLogger(tc, systemConfig, logDir, systemLogFileName))
	}
	for _, taskConfig := range c.Task {
		config.Task = append(config.Task, a.prepSingleLogger(tc, taskConfig, logDir, taskLogFileName))
	}

	return config
}

func (a *Agent) prepSingleLogger(tc *taskContext, in model.LogOpts, logDir, fileName string) client.LogOpts {
	splunkServer, err := tc.expansions.ExpandString(in.SplunkServer)
	if err != nil {
		grip.Error(errors.Wrap(err, "expanding Splunk server"))
	}
	splunkToken, err := tc.expansions.ExpandString(in.SplunkToken)
	if err != nil {
		grip.Error(errors.Wrap(err, "expanding Splunk token"))
	}
	if in.LogDirectory != "" {
		grip.Error(errors.Wrapf(os.MkdirAll(in.LogDirectory, os.ModeDir|os.ModePerm), "making log directory '%s'", in.LogDirectory))
		logDir = in.LogDirectory
	}
	return client.LogOpts{
		LogkeeperURL:      a.opts.LogkeeperURL,
		LogkeeperBuildNum: tc.taskModel.Execution,
		BuilderID:         tc.taskModel.Id,
		Sender:            in.Type,
		SplunkServerURL:   splunkServer,
		SplunkToken:       splunkToken,
		Filepath:          filepath.Join(logDir, fileName),
	}
}
