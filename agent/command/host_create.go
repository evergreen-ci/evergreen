package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type createHost struct {
	CreateHost *apimodels.CreateHost
	File       string
	base
}

func createHostFactory() Command { return &createHost{} }

func (c *createHost) Name() string { return "host.create" }

func (c *createHost) ParseParams(params map[string]interface{}) error {
	c.CreateHost = &apimodels.CreateHost{}

	// background default is true
	if _, ok := params["background"]; !ok {
		c.CreateHost.Background = true
	}

	if _, ok := params["file"]; ok {
		fileName := fmt.Sprintf("%v", params["file"])
		c.File = fileName
		if len(params) > 1 {
			return errors.New("when using a file to parse params, no additional params other than file name should be defined")
		}
		return nil
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           c.CreateHost,
	})
	if err != nil {
		return errors.Wrap(err, "constructing mapstructure decoder")
	}
	return errors.Wrap(decoder.Decode(params), "decoding mapstructure params")
}

func (c *createHost) parseParamsFromFile(fn string, conf *internal.TaskConfig) error {
	if !filepath.IsAbs(fn) {
		fn = getWorkingDirectory(conf, fn)
	}
	return errors.Wrapf(utility.ReadYAMLFile(fn, &c.CreateHost), "reading YAML from file '%s'", fn)
}

func (c *createHost) expandAndValidate(ctx context.Context, conf *internal.TaskConfig) error {
	// if a filename is defined, then parseParams has not parsed the parameters yet,
	// since the file was not yet available. it therefore needs to be parsed now.
	if c.File != "" {
		if err := c.parseParamsFromFile(c.File, conf); err != nil {
			return err
		}
	}

	if err := c.CreateHost.Expand(&conf.Expansions); err != nil {
		return err
	}

	if err := c.CreateHost.Validate(ctx); err != nil {
		return errors.Wrap(err, "invalid command options")
	}
	return nil
}

func (c *createHost) Execute(ctx context.Context, comm client.Communicator,
	logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := c.expandAndValidate(ctx, conf); err != nil {
		return err
	}

	taskData := client.TaskData{
		ID:     conf.Task.Id,
		Secret: conf.Task.Secret,
	}

	if c.CreateHost.UserdataFile != "" {
		if err := c.populateUserdata(); err != nil {
			return err
		}
	}
	if c.CreateHost.StdinFile != "" {
		c.CreateHost.StdinFile = getWorkingDirectory(conf, c.CreateHost.StdinFile)
		fileContent, err := os.ReadFile(c.CreateHost.StdinFile)
		if err != nil {
			return errors.Wrapf(err, "reading stdin file '%s'", c.CreateHost.StdinFile)
		}
		c.CreateHost.StdinFileContents = fileContent
	}
	startTime := time.Now()

	c.logAMI(ctx, comm, logger, taskData)
	ids, err := comm.CreateHost(ctx, taskData, *c.CreateHost)
	if err != nil {
		return errors.Wrap(err, "creating host")
	} else if c.CreateHost.CloudProvider == apimodels.ProviderDocker {
		if err = c.getLogsFromNewDockerHost(ctx, logger, comm, ids, startTime, conf); err != nil {
			return errors.Wrap(err, "getting logs from created host")
		}
	}

	return nil
}

func (c *createHost) logAMI(ctx context.Context, comm client.Communicator, logger client.LoggerProducer,
	taskData client.TaskData) {
	if c.CreateHost.CloudProvider != apimodels.ProviderEC2 {
		return
	}
	if c.CreateHost.AMI != "" {
		logger.Task().Infof("host.create: using given AMI '%s'.", c.CreateHost.AMI)
		return
	}

	ami, err := comm.GetDistroAMI(ctx, c.CreateHost.Distro, c.CreateHost.Region, taskData)
	if err != nil {
		logger.Task().Warning(errors.Wrapf(err, "host.create: unable to retrieve AMI from distro '%s'.",
			c.CreateHost.Distro))
		return
	}

	logger.Task().Infof("host.create: using AMI '%s' (for distro '%s').", ami, c.CreateHost.Distro)
}

type logBatchInfo struct {
	hostID     string
	outFile    *os.File
	errFile    *os.File
	batchStart time.Time
	batchEnd   time.Time
}

func (c *createHost) getLogsFromNewDockerHost(ctx context.Context, logger client.LoggerProducer, comm client.Communicator,
	ids []string, startTime time.Time, conf *internal.TaskConfig) error {
	var err error
	if len(ids) == 0 {
		return errors.New("programmer error: no intent host ID received")
	}

	info, err := c.initializeLogBatchInfo(ids[0], conf, startTime)
	if err != nil {
		return err
	}

	if !c.CreateHost.Background {
		defer info.outFile.Close()
		defer info.errFile.Close()
		return errors.Wrapf(c.waitForLogs(ctx, comm, logger, info), "waiting for logs")
	}

	go func() {
		defer info.outFile.Close()
		defer info.errFile.Close()
		if err = c.waitForLogs(ctx, comm, logger, info); err != nil {
			logger.Task().Error(errors.Wrap(err, "waiting for logs in the background"))
		}
	}()

	return nil
}

func (c *createHost) initializeLogBatchInfo(id string, conf *internal.TaskConfig, startTime time.Time) (*logBatchInfo, error) {
	const permissions = os.O_APPEND | os.O_CREATE | os.O_WRONLY
	info := &logBatchInfo{batchStart: startTime}

	// initialize file names
	if c.CreateHost.StderrFile == "" {
		c.CreateHost.StderrFile = fmt.Sprintf("%s.err.log", id)
	}
	if !filepath.IsAbs(c.CreateHost.StderrFile) {
		c.CreateHost.StderrFile = getWorkingDirectory(conf, c.CreateHost.StderrFile)
	}
	if c.CreateHost.StdoutFile == "" {
		c.CreateHost.StdoutFile = fmt.Sprintf("%s.out.log", id)
	}
	if !filepath.IsAbs(c.CreateHost.StdoutFile) {
		c.CreateHost.StdoutFile = getWorkingDirectory(conf, c.CreateHost.StdoutFile)
	}

	// initialize files
	var err error
	info.outFile, err = os.OpenFile(c.CreateHost.StdoutFile, permissions, 0644)
	if err != nil {
		return nil, errors.Wrapf(err, "creating file '%s'", c.CreateHost.StdoutFile)
	}
	info.errFile, err = os.OpenFile(c.CreateHost.StderrFile, permissions, 0644)
	if err != nil {
		// the outfile already exists, so close it
		if fileErr := info.outFile.Close(); fileErr != nil {
			return nil, errors.Wrapf(fileErr, "removing stdout file after failed stderr file creation")
		}
		return nil, errors.Wrapf(err, "creating file '%s'", c.CreateHost.StderrFile)
	}
	return info, nil
}

func (c *createHost) waitForLogs(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, info *logBatchInfo) error {

	pollTicker := time.NewTicker(time.Duration(c.CreateHost.PollFrequency) * time.Second)
	defer pollTicker.Stop()

	timeoutTimer := time.NewTimer(time.Duration(c.CreateHost.ContainerWaitTimeoutSecs) * time.Second)
	defer timeoutTimer.Stop()

	// get logs in batches until container exits or we timeout
	startedCollectingLogs := false
	for {
		select {
		case <-ctx.Done():
			logger.Task().Infof("Context canceled waiting for host '%s' to exit: %s.", info.hostID, ctx.Err())
			return nil
		case <-pollTicker.C:
			info.batchEnd = time.Now()
			status, err := comm.GetDockerStatus(ctx, info.hostID)
			if err != nil {
				logger.Task().Infof("Problem receiving Docker logs in host.create: '%s'.", err.Error())
				if startedCollectingLogs {
					return nil // container has likely exited
				}
				continue
			}
			if status.HasStarted {
				startedCollectingLogs = true
				info.getAndWriteLogBatch(ctx, comm, logger)
				info.batchStart = info.batchEnd.Add(time.Nanosecond) // to prevent repeat logs

				if !status.IsRunning { // container exited
					logger.Task().Infof("Logs retrieved for container '%s' in %d seconds.",
						info.hostID, int(time.Since(info.batchStart).Seconds()))
					return nil
				}
			}
		case <-timeoutTimer.C:
			return errors.New("reached timeout waiting for host to exit")
		}
	}
}

func (info *logBatchInfo) getAndWriteLogBatch(ctx context.Context, comm client.Communicator, logger client.LoggerProducer) {
	outLogs, err := comm.GetDockerLogs(ctx, info.hostID, info.batchStart, info.batchEnd, false)
	if err != nil {
		logger.Task().Errorf("Error retrieving Docker output logs: %s.", err)
	}
	errLogs, err := comm.GetDockerLogs(ctx, info.hostID, info.batchStart, info.batchEnd, true)
	if err != nil {
		logger.Task().Errorf("Error retrieving Docker error logs: %s.", err)
	}
	if _, err = info.outFile.Write(outLogs); err != nil {
		logger.Task().Errorf("Error writing stdout to file: %s.", outLogs)
	}
	if _, err = info.errFile.Write(errLogs); err != nil {
		logger.Task().Errorf("Error writing stderr to file: %s.", errLogs)
	}
}

func (c *createHost) populateUserdata() error {
	file, err := os.Open(c.CreateHost.UserdataFile)
	if err != nil {
		return errors.Wrap(err, "opening user data file")
	}
	defer file.Close()
	fileData, err := io.ReadAll(file)
	if err != nil {
		return errors.Wrap(err, "reading user data file")
	}
	c.CreateHost.UserdataCommand = string(fileData)

	return nil
}
