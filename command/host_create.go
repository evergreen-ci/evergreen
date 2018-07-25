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

type CreateHost struct {
	CreateHost *apimodels.CreateHost
	base
}

func createHostFactory() Command { return &CreateHost{} }

func (c *CreateHost) Name() string { return "host.create" }

func (c *CreateHost) ParseParams(params map[string]interface{}) error {
	c.CreateHost = &apimodels.CreateHost{}
	if err := mapstructure.Decode(params, c.CreateHost); err != nil {
		return errors.Wrapf(err, "error parsing '%s' params", c.Name())
	}

	return c.CreateHost.Validate()
}

func (c *CreateHost) Execute(ctx context.Context, comm client.Communicator,
	logger client.LoggerProducer, conf *model.TaskConfig) error {

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

func (c *CreateHost) populateUserdata() error {
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
