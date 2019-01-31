package agent

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws/endpoints"

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

const taskLogDirectory = "evergreen-logs"

var (
	idSource chan int

	agentLogs  = fmt.Sprintf("%s/%s", taskLogDirectory, "agent.log")
	systemLogs = fmt.Sprintf("%s/%s", taskLogDirectory, "system.log")
	taskLogs   = fmt.Sprintf("%s/%s", taskLogDirectory, "task.log")
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
	path := fmt.Sprintf("%s/%s", a.opts.WorkingDirectory, taskLogDirectory)
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
			Filepath:          fmt.Sprintf("%s/%s", a.opts.WorkingDirectory, agentLogs),
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
			Filepath:          fmt.Sprintf("%s/%s", a.opts.WorkingDirectory, systemLogs),
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
			Filepath:          fmt.Sprintf("%s/%s", a.opts.WorkingDirectory, taskLogs),
		})
	}
	return a.comm.GetLoggerProducer(ctx, td, &config)
}

type s3UploadOpts struct {
	bucket    string
	awsKey    string
	awsSecret string
}

func (a *Agent) uploadToS3(ctx context.Context, tc *taskContext, opts s3UploadOpts) error {
	s3Opts := pail.S3Options{
		Credentials: pail.CreateAWSCredentials(opts.awsKey, opts.awsSecret, ""),
		Region:      endpoints.UsEast1RegionID,
		Name:        opts.bucket,
		Permission:  "public-read",
		ContentType: "text/plain",
	}
	bucket, err := pail.NewS3Bucket(s3Opts)
	if err != nil {
		return errors.Wrap(err, "error creating pail")
	}

	return a.uploadLogFiles(ctx, tc, bucket, opts.bucket)
}

func (a *Agent) uploadLogFiles(ctx context.Context, tc *taskContext, bucket pail.Bucket, name string) error {
	agentLogFile, err := os.Open(fmt.Sprintf("%s/%s", a.opts.WorkingDirectory, agentLogs))
	if err != nil {
		if !os.IsNotExist(err) {
			grip.Error(errors.Wrap(err, "error opening agent log file"))
		}
	} else {
		path := fmt.Sprintf("logs/%s/%d/agent.log", tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution)
		err = bucket.Put(ctx, path, agentLogFile)
		if err != nil {
			return errors.Wrap(err, "error uploading agent log")
		}
		grip.Infof("agent logs uploaded to %s/%s", name, path)
	}
	systemLogFile, err := os.Open(fmt.Sprintf("%s/%s", a.opts.WorkingDirectory, systemLogs))
	if err != nil {
		if !os.IsNotExist(err) {
			grip.Error(errors.Wrap(err, "error opening system log file"))
		}
	} else {
		path := fmt.Sprintf("logs/%s/%d/system.log", tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution)
		err = bucket.Put(ctx, path, systemLogFile)
		if err != nil {
			return errors.Wrap(err, "error uploading system log")
		}
		grip.Infof("system logs uploaded to %s/%s", name, path)
	}
	taskLogFile, err := os.Open(fmt.Sprintf("%s/%s", a.opts.WorkingDirectory, taskLogs))
	if err != nil {
		if !os.IsNotExist(err) {
			grip.Error(errors.Wrap(err, "error opening agent task file"))
		}
	} else {
		path := fmt.Sprintf("logs/%s/%d/task.log", tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution)
		err = bucket.Put(ctx, path, taskLogFile)
		if err != nil {
			return errors.Wrap(err, "error uploading task log")
		}
		grip.Infof("task logs uploaded to %s/%s", name, path)
	}

	return nil
}
