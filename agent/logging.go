package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/pail"
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
func (a *Agent) GetSender(ctx context.Context, output globals.LogOutputType, prefix string, taskID string, taskExecution int) (send.Sender, error) {
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
	case globals.LogOutputStdout:
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

func (a *Agent) makeLoggerProducer(ctx context.Context, tc *taskContext, commandName string) (client.LoggerProducer, error) {
	config := a.prepLogger(tc, commandName)

	logger, err := a.comm.GetLoggerProducer(ctx, &tc.taskConfig.Task, &config)
	if err != nil {
		return nil, err
	}
	return logger, nil
}

func (a *Agent) prepLogger(tc *taskContext, commandName string) client.LoggerConfig {
	logDir := filepath.Join(a.opts.WorkingDirectory, taskLogDirectory)
	grip.Error(errors.Wrapf(os.MkdirAll(logDir, os.ModeDir|os.ModePerm), "making log directory '%s'", logDir))
	// if this is a command-specific logger, create a dir for the command's logs separate from the overall task
	if commandName != "" {
		logDir = filepath.Join(logDir, commandName)
		grip.Error(errors.Wrapf(os.MkdirAll(logDir, os.ModeDir|os.ModePerm), "making log directory '%s' for command '%s'", logDir, commandName))
	}
	redactorExpansions := tc.taskConfig.NewExpansions
	// Add the host's secret to the internal agent expansions, so it can be redacted by our redacting logger later.
	redactorExpansions.Put(globals.HostSecret, a.opts.HostSecret)
	config := client.LoggerConfig{
		SendToGlobalSender: a.opts.SendTaskLogsToGlobalSender,
		AWSCredentials:     pail.CreateAWSCredentials(tc.taskConfig.TaskSync.Key, tc.taskConfig.TaskSync.Secret, ""),
		RedactorOpts: redactor.RedactionOptions{
			Expansions: redactorExpansions,
			Redacted:   tc.taskConfig.Redacted,
		},
	}
	agentLog := client.LogOpts{
		Sender:   model.EvergreenLogSender,
		Filepath: filepath.Join(logDir, agentLogFileName),
	}
	systemLog := client.LogOpts{
		Sender:   model.EvergreenLogSender,
		Filepath: filepath.Join(logDir, systemLogFileName),
	}
	taskLog := client.LogOpts{
		Sender:   model.EvergreenLogSender,
		Filepath: filepath.Join(logDir, taskLogFileName),
	}
	config.Agent = append(config.Agent, agentLog)
	config.System = append(config.System, systemLog)
	config.Task = append(config.Task, taskLog)
	return config
}
