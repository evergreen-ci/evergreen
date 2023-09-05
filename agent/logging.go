package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal/client"
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

// GetSender configures the agent's local logging, which can go to Splunk, a
// file, or stdout.
func (a *Agent) GetSender(ctx context.Context, output LogOutputType, prefix string, taskID string, taskExecution int) (send.Sender, error) {
	var senders []send.Sender

	splunkInfo := send.SplunkConnectionInfo{
		ServerURL: a.opts.SetupData.SplunkServerURL,
		Token:     a.opts.SetupData.SplunkClientToken,
		Channel:   a.opts.SetupData.SplunkChannel,
	}
	if splunkInfo.Populated() {
		// Send alerts or higher from the agent to Splunk, since they are
		// generally critical problems (e.g. panics) in the agent runtime.
		grip.Info("Configuring Splunk sender.")
		alertThreshold := send.LevelInfo{Default: level.Alert, Threshold: level.Alert}
		sender, err := send.NewSplunkLogger("evergreen.agent", splunkInfo, alertThreshold)
		if err != nil {
			return nil, errors.Wrap(err, "creating Splunk logger")
		}
		senders = append(senders, sender)
	}

	switch output {
	case LogOutputStdout:
		sender, err := send.NewNativeLogger("evergreen.agent", send.LevelInfo{Default: level.Info, Threshold: level.Debug})
		if err != nil {
			return nil, errors.Wrap(err, "creating native console logger")
		}

		senders = append(senders, sender)
	default:
		var fileName string
		if taskID != "" && taskExecution >= 0 {
			fileName = fmt.Sprintf("%s-%d-%s-%d.log", prefix, os.Getpid(), taskID, taskExecution)
		} else {
			fileName = fmt.Sprintf("%s-%d-%d.log", prefix, os.Getpid(), getInc())
		}
		sender, err := send.NewFileLogger("evergreen.agent",
			fileName,
			send.LevelInfo{Default: level.Info, Threshold: level.Debug})
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
	config := client.LoggerConfig{
		SendToGlobalSender: a.opts.SendTaskLogsToGlobalSender,
	}

	defaultLogger := tc.taskConfig.ProjectRef.DefaultLogger

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
	splunkServer, err := tc.taskConfig.Expansions.ExpandString(in.SplunkServer)
	if err != nil {
		grip.Error(errors.Wrap(err, "expanding Splunk server"))
	}
	splunkToken, err := tc.taskConfig.Expansions.ExpandString(in.SplunkToken)
	if err != nil {
		grip.Error(errors.Wrap(err, "expanding Splunk token"))
	}
	if in.LogDirectory != "" {
		grip.Error(errors.Wrapf(os.MkdirAll(in.LogDirectory, os.ModeDir|os.ModePerm), "making log directory '%s'", in.LogDirectory))
		logDir = in.LogDirectory
	}
	return client.LogOpts{
		BuilderID:       tc.taskConfig.Task.Id,
		Sender:          in.Type,
		SplunkServerURL: splunkServer,
		SplunkToken:     splunkToken,
		Filepath:        filepath.Join(logDir, fileName),
	}
}
