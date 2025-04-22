package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

func (c *createHost) ParseParams(params map[string]any) error {
	c.CreateHost = &apimodels.CreateHost{}

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
		fn = GetWorkingDirectory(conf, fn)
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

	c.logAMI(ctx, comm, logger, taskData)
	if _, err := comm.CreateHost(ctx, taskData, *c.CreateHost); err != nil {
		return errors.Wrap(err, "creating host")
	}

	return nil
}

func (c *createHost) logAMI(ctx context.Context, comm client.Communicator, logger client.LoggerProducer,
	taskData client.TaskData) {
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
