package command

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type createHost struct {
	CreateHost *apimodels.CreateHost
	base
}

func createHostFactory() Command { return &createHost{} }

func (c *createHost) Name() string { return "host.create" }

func (c *createHost) ParseParams(params map[string]interface{}) error {
	c.CreateHost = &apimodels.CreateHost{}

	// background default is true
	if _, ok := params["background"]; !ok {
		params["background"] = true
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

func (c *createHost) expandAndValidate(conf *model.TaskConfig) error {
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
	ids, err := comm.CreateHost(ctx, taskData, *c.CreateHost)
	if err != nil {
		return errors.Wrap(err, "error creating host")
	} else if c.CreateHost.CloudProvider == apimodels.ProviderDocker {
		if err = c.getLogsFromNewDockerHost(ctx, logger, comm, ids, startTime); err != nil {
			return errors.Wrap(err, "problem getting logs from created host")
		}
	}

	return nil
}

func (c *createHost) getLogsFromNewDockerHost(ctx context.Context, logger client.LoggerProducer, comm client.Communicator,
	ids []string, startTime time.Time) error {
	var err error
	if len(ids) == 0 {
		return errors.New("Programmer error: no intent host ID received")
	}
	if c.CreateHost.StderrFile == "" {
		c.CreateHost.StderrFile = fmt.Sprintf("%s.err.log", ids[0])
	}
	if c.CreateHost.StdoutFile == "" {
		c.CreateHost.StdoutFile = fmt.Sprintf("%s.out.log", ids[0])
	}

	if !c.CreateHost.Background {
		return errors.Wrap(c.waitForLogs(ctx, comm, logger, startTime, ids[0]), "error waiting for logs")
	}

	go func() {
		if err = c.waitForLogs(ctx, comm, logger, startTime, ids[0]); err != nil {
			logger.Task().Errorf("error waiting for logs in background: %v", err)
		}
	}()

	return nil
}

func (c *createHost) waitForLogs(ctx context.Context, comm client.Communicator, logger client.LoggerProducer,
	startTime time.Time, hostID string) error {
	pollTicker := time.NewTicker(time.Duration(c.CreateHost.PollFrequency) * time.Second)
	defer pollTicker.Stop()

	timeoutTimer := time.NewTimer(time.Duration(c.CreateHost.ContainerWaitTimeoutSecs) * time.Second)
	defer timeoutTimer.Stop()

	batchStart := startTime
	// get logs in batches until container exits or we timeout
	for {
		select {
		case <-ctx.Done():
			logger.Task().Infof("context finished waiting for host %s to exit", hostID)
			return nil
		case <-pollTicker.C:
			batchEnd := time.Now()
			status, err := comm.GetDockerStatus(ctx, hostID)
			if err != nil {
				return errors.Wrapf(err, "error getting docker status")
			}
			if status.HasStarted {
				if err = c.getAndWriteLogBatch(ctx, comm, hostID, batchStart, batchEnd); err != nil {
					return errors.Wrapf(err, "error getting and writing logs on started container")
				}
				batchStart = batchEnd

				if !status.IsRunning { // container exited
					logger.Task().Infof("Logs retrieved for container _id %s in %d seconds",
						hostID, int(time.Since(startTime).Seconds()))
					return nil
				}
			}
		case <-timeoutTimer.C:
			return errors.New("reached timeout waiting for host to exit")
		}
	}
}

func (c *createHost) getAndWriteLogBatch(ctx context.Context, comm client.Communicator, hostID string,
	startTime, endTime time.Time) error {

	outLogs, err := comm.GetDockerLogs(ctx, hostID, startTime, endTime, false)
	if err != nil {
		return errors.Wrap(err, "error retrieving docker output logs")
	}
	errLogs, err := comm.GetDockerLogs(ctx, hostID, startTime, endTime, true)
	if err != nil {
		return errors.Wrap(err, "error retrieving docker error logs")
	}

	if err = ioutil.WriteFile(c.CreateHost.StdoutFile, outLogs, 0644); err != nil {
		return errors.Wrap(err, "error writing stdout to file")
	}
	if err = ioutil.WriteFile(c.CreateHost.StderrFile, errLogs, 0644); err != nil {
		return errors.Wrap(err, "error writing stderr to file")
	}
	return nil
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
