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
	} else if c.CreateHost.CloudProvider != apimodels.ProviderDocker {
		return nil // don't want to wait for logs
	}

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
	pollTicker := time.NewTimer(time.Duration(c.CreateHost.PollFrequency) * time.Second)
	defer pollTicker.Stop()

	timeoutTimer := time.NewTimer(time.Duration(c.CreateHost.ContainerWaitTimeoutSecs) * time.Second)
	defer timeoutTimer.Stop()

	// get logs in batches until container exits or we timeout
	for {
		select {
		case <-ctx.Done():
			return errors.New("context finished waiting for host to exit")
		case <-pollTicker.C:
			curTime := time.Now()
			logInfo, err := comm.GetDockerLogs(ctx, hostID, startTime, curTime)
			if err != nil {
				return errors.Wrap(err, "error retrieving docker logs")
			}
			startTime = curTime
			if logInfo.HasStarted {
				if err = ioutil.WriteFile(c.CreateHost.StdoutFile, []byte(logInfo.OutStr), 0644); err != nil {
					return errors.Wrap(err, "error writing stdout to file")
				}
				if err = ioutil.WriteFile(c.CreateHost.StderrFile, []byte(logInfo.ErrStr), 0644); err != nil {
					return errors.Wrap(err, "error writing stderr to file")
				}
				if !logInfo.IsRunning { // container exited
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
