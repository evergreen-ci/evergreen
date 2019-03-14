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

	if params["background"] == nil {
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
	if err != nil || c.CreateHost.CloudProvider != apimodels.ProviderDocker || c.CreateHost.Background == false {
		return err
	}
	if len(ids) == 0 {
		return errors.New("Programmer error: no intent host ID received")
	}
	if c.CreateHost.StderrFile == "" {
		c.CreateHost.StderrFile = fmt.Sprintf("container-err-%s.log", ids[0])
	}
	if c.CreateHost.StdoutFile == "" {
		c.CreateHost.StdoutFile = fmt.Sprintf("container-out-%s.log", ids[0])
	}

	errChan := make(chan error)

	timer := time.NewTimer(time.Duration(c.CreateHost.PollFrequency) * time.Second)
	defer timer.Stop()

	timeoutTimer := time.NewTimer(time.Duration(c.CreateHost.BackgroundTimeoutSecs) * time.Second)
	defer timeoutTimer.Stop()

	// get logs in batches until container exits or we timeout
waitForLogs:
	for {
		select {
		case <-ctx.Done():
			errChan <- errors.New("context finished waiting for host to exit")
			break waitForLogs
		case <-timer.C:
			curTime := time.Now()
			logInfo, err := comm.GetDockerLogs(ctx, ids[0], startTime, curTime)
			startTime = curTime
			if err != nil || logInfo == nil {
				errChan <- err
				break waitForLogs
			}
			if logInfo.HasStarted != false {
				err = ioutil.WriteFile(c.CreateHost.StdoutFile, []byte(logInfo.OutStr), 0644)
				err2 := ioutil.WriteFile(c.CreateHost.StderrFile, []byte(logInfo.ErrStr), 0644)
				if err != nil || err2 != nil {
					errChan <- err
					errChan <- err2
					break waitForLogs
				}
				if !logInfo.IsRunning {
					break waitForLogs
				}
			}

			timer.Reset(time.Duration(c.CreateHost.PollFrequency) * time.Second)
		case <-timeoutTimer.C:
			errChan <- errors.New("reached timeout waiting for host to exit")
			break waitForLogs
		}
	}

	select {
	case err := <-errChan:
		return errors.Wrap(err, "problem waiting for logs in background")
	default:
		return nil
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
