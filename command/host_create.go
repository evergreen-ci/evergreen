package command

import (
	"context"
	"io/ioutil"
	"os"

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
	return errors.Wrapf(mapstructure.Decode(params, c.CreateHost), "error parsing '%s' params", c.Name())
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

	return comm.CreateHost(ctx, taskData, *c.CreateHost)
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
