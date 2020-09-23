package command

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
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
			return errors.New("no params should be defined when using a file to parse params")
		}
		return nil
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           c.CreateHost,
	})
	if err != nil {
		return errors.Wrap(err, "problem constructing mapstructure decoder")
	}
	return errors.Wrapf(decoder.Decode(params), "error parsing '%s' params", c.Name())
}

func (c *createHost) parseParamsFromFile(fn string, conf *model.TaskConfig) error {
	if !filepath.IsAbs(fn) {
		fn = filepath.Join(conf.WorkDir, fn)
	}
	return errors.Wrapf(utility.ReadYAMLFile(fn, &c.CreateHost),
		"error reading from file '%s'", fn)
}

func (c *createHost) expandAndValidate(conf *model.TaskConfig) error {
	// if a filename is defined, then parseParams has not parsed the parameters yet,
	// since the file was not yet available. it therefore needs to be parsed now.
	if c.File != "" {
		if err := c.parseParamsFromFile(c.File, conf); err != nil {
			return err
		}
	}

	if err := c.CreateHost.Expand(conf.Expansions); err != nil {
		return err
	}

	if err := c.CreateHost.Validate(); err != nil {
		return errors.Wrap(err, "command is invalid")
	}
	return nil
}

func (c *createHost) Execute(ctx context.Context, comm client.Communicator,
	logger client.LoggerProducer, conf *model.TaskConfig) error {
	if err := c.expandAndValidate(conf); err != nil {
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
	startTime := time.Now()

	c.logAMI(ctx, comm, logger, taskData)
	ids, err := comm.CreateHost(ctx, taskData, *c.CreateHost)
	if err != nil {
		return errors.Wrap(err, "error creating host")
	} else if c.CreateHost.CloudProvider == apimodels.ProviderDocker {
		if err = c.getLogsFromNewDockerHost(ctx, logger, comm, ids, startTime, conf.WorkDir); err != nil {
			return errors.Wrap(err, "problem getting logs from created host")
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
		logger.Task().Infof("host.create: using given AMI '%s'", c.CreateHost.AMI)
		return
	}

	ami, err := comm.GetDistroAMI(ctx, c.CreateHost.Distro, c.CreateHost.Region, taskData)
	if err != nil {
		logger.Task().Warning(errors.Wrapf(err, "host.create: unable to retrieve AMI from distro '%s'",
			c.CreateHost.Distro))
		return
	}

	logger.Task().Infof("host.create: using AMI '%s' (for distro '%s')", ami, c.CreateHost.Distro)
}

type logBatchInfo struct {
	hostID     string
	outFile    *os.File
	errFile    *os.File
	batchStart time.Time
	batchEnd   time.Time
}

func (c *createHost) getLogsFromNewDockerHost(ctx context.Context, logger client.LoggerProducer, comm client.Communicator,
	ids []string, startTime time.Time, workDir string) error {
	var err error
	if len(ids) == 0 {
		return errors.New("Programmer error: no intent host ID received")
	}

	info, err := c.initializeLogBatchInfo(ids[0], workDir, startTime)
	if err != nil {
		return err
	}

	if !c.CreateHost.Background {
		defer info.outFile.Close()
		defer info.errFile.Close()
		return errors.Wrapf(c.waitForLogs(ctx, comm, logger, info), "error waiting for logs")
	}

	go func() {
		defer info.outFile.Close()
		defer info.errFile.Close()
		if err = c.waitForLogs(ctx, comm, logger, info); err != nil {
			logger.Task().Errorf("error waiting for logs in background: %v", err)
		}
	}()

	return nil
}

func (c *createHost) initializeLogBatchInfo(id, workDir string, startTime time.Time) (*logBatchInfo, error) {
	const permissions = os.O_APPEND | os.O_CREATE | os.O_WRONLY
	info := &logBatchInfo{batchStart: startTime}

	// initialize file names
	if c.CreateHost.StderrFile == "" {
		c.CreateHost.StderrFile = fmt.Sprintf("%s.err.log", id)
	}
	if !filepath.IsAbs(c.CreateHost.StderrFile) {
		c.CreateHost.StderrFile = filepath.Join(workDir, c.CreateHost.StderrFile)
	}
	if c.CreateHost.StdoutFile == "" {
		c.CreateHost.StdoutFile = fmt.Sprintf("%s.out.log", id)
	}
	if !filepath.IsAbs(c.CreateHost.StdoutFile) {
		c.CreateHost.StdoutFile = filepath.Join(workDir, c.CreateHost.StdoutFile)
	}

	// initialize files
	var err error
	info.outFile, err = os.OpenFile(c.CreateHost.StdoutFile, permissions, 0644)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating file %s", c.CreateHost.StdoutFile)
	}
	info.errFile, err = os.OpenFile(c.CreateHost.StderrFile, permissions, 0644)
	if err != nil {
		// the outfile already exists, so close it
		if fileErr := info.outFile.Close(); fileErr != nil {
			return nil, errors.Wrapf(fileErr, "error removing stdout file after failed stderr file creation")
		}
		return nil, errors.Wrapf(err, "error creating file %s", c.CreateHost.StderrFile)
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
			logger.Task().Infof("context cancelled waiting for host %s to exit", info.hostID)
			return nil
		case <-pollTicker.C:
			info.batchEnd = time.Now()
			status, err := comm.GetDockerStatus(ctx, info.hostID)
			if err != nil {
				logger.Task().Infof("problem receiving docker logs in host.create: '%s'", err.Error())
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
					logger.Task().Infof("Logs retrieved for container _id %s in %d seconds",
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
		logger.Task().Errorf("error retrieving docker out logs: %s", err.Error())
	}
	errLogs, err := comm.GetDockerLogs(ctx, info.hostID, info.batchStart, info.batchEnd, true)
	if err != nil {
		logger.Task().Errorf("error retrieving docker error logs: %s", err.Error())
	}
	if _, err = info.outFile.Write(outLogs); err != nil {
		logger.Task().Errorf("error writing stdout to file: %s", outLogs)
	}
	if _, err = info.errFile.Write(errLogs); err != nil {
		logger.Task().Errorf("error writing stderr to file: %s", errLogs)
	}
}

func (c *createHost) populateUserdata() error {
	file, err := os.Open(c.CreateHost.UserdataFile)
	if err != nil {
		return errors.Wrap(err, "error opening UserData file")
	}
	defer file.Close()
	fileData, err := ioutil.ReadAll(file)
	if err != nil {
		return errors.Wrap(err, "error reading UserData file")
	}
	c.CreateHost.UserdataCommand = string(fileData)

	return nil
}
