package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
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

func (a *Agent) makeLoggerProducer(ctx context.Context, c *model.LoggerConfig, td client.TaskData, task *task.Task) client.LoggerProducer {
	path := filepath.Join(a.opts.WorkingDirectory, taskLogDirectory)
	grip.Error(errors.Wrap(os.Mkdir(path, os.ModeDir|os.ModePerm), "error making log directory"))

	config := client.LoggerConfig{}
	for _, agentConfig := range c.Agent {
		config.Agent = append(config.Agent, client.LogOpts{
			LogkeeperURL:      a.opts.LogkeeperURL,
			LogkeeperBuilder:  task.Id,
			LogkeeperBuildNum: task.Execution,
			Sender:            agentConfig.Type,
			SplunkServerURL:   agentConfig.SplunkServer,
			SplunkToken:       agentConfig.SplunkToken,
			Filepath:          filepath.Join(a.opts.WorkingDirectory, taskLogDirectory, agentLogFileName),
		})
	}
	for _, systemConfig := range c.System {
		config.System = append(config.System, client.LogOpts{
			LogkeeperURL:      a.opts.LogkeeperURL,
			LogkeeperBuilder:  task.Id,
			LogkeeperBuildNum: task.Execution,
			Sender:            systemConfig.Type,
			SplunkServerURL:   systemConfig.SplunkServer,
			SplunkToken:       systemConfig.SplunkToken,
			Filepath:          filepath.Join(a.opts.WorkingDirectory, taskLogDirectory, systemLogFileName),
		})
	}
	for _, taskConfig := range c.Task {
		config.Task = append(config.Task, client.LogOpts{
			LogkeeperURL:      a.opts.LogkeeperURL,
			LogkeeperBuilder:  task.Id,
			LogkeeperBuildNum: task.Execution,
			Sender:            taskConfig.Type,
			SplunkServerURL:   taskConfig.SplunkServer,
			SplunkToken:       taskConfig.SplunkToken,
			Filepath:          filepath.Join(a.opts.WorkingDirectory, taskLogDirectory, taskLogFileName),
		})
	}
	return a.comm.GetLoggerProducer(ctx, td, &config)
}

func (a *Agent) uploadToS3(ctx context.Context, tc *taskContext) error {
	bucket, err := pail.NewS3Bucket(a.opts.S3Opts)
	if err != nil {
		return errors.Wrap(err, "error creating pail")
	}

	return a.uploadLogFiles(ctx, tc, bucket)
}

func (a *Agent) uploadLogFiles(ctx context.Context, tc *taskContext, bucket pail.Bucket) error {
	if tc.taskConfig == nil || tc.taskConfig.Task == nil {
		return nil
	}
	catcher := grip.NewBasicCatcher()
	catcher.Add(a.uploadSingleFile(ctx, bucket, agentLogFileName, tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution))
	catcher.Add(a.uploadSingleFile(ctx, bucket, systemLogFileName, tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution))
	catcher.Add(a.uploadSingleFile(ctx, bucket, taskLogFileName, tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution))

	return catcher.Resolve()
}

func (a *Agent) uploadSingleFile(ctx context.Context, bucket pail.Bucket, file string, taskID string, execution int) error {
	localPath := filepath.Join(a.opts.WorkingDirectory, taskLogDirectory, file)
	_, err := os.Stat(localPath)
	if os.IsNotExist(err) {
		return nil
	}
	return bucket.Upload(ctx, filepath.Join("logs", taskID, strconv.Itoa(execution), file), localPath)
}
