package command

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// setExpansions takes a file of key value pairs and
// sets the downstream parameters for a patch
type setDownstream struct {
	// Key-value pairs for updating the task's parameters with
	DownstreamParams []patch.Parameter `mapstructure:"downstream_params"`

	// Filename for a yaml file containing key-value pairs
	YamlFile          string `mapstructure:"file"`
	IgnoreMissingFile bool   `mapstructure:"ignore_missing_file"`
	base
}

func setExpansionsFactory() Command   { return &setDownstream{} }
func (c *setDownstream) Name() string { return "downstream_expansions.set" }

// ParseParams validates the input to the update, returning and error
// if something is incorrect. Fulfills Command interface.
func (c *setDownstream) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return err
	}

	for _, item := range c.DownstreamParams {
		if item.Key == "" {
			return errors.Errorf("error parsing '%v' params: key must not be "+
				"a blank string", c.Name())
		}
	}

	return nil
}

// Execute updates the expansions. Fulfills Command interface.
// params are not expanded as part of Execute, they will be expanded as part of parameterized builds.
func (c *setDownstream) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	var err error
	if c.YamlFile != "" {
		c.YamlFile, err = conf.Expansions.ExpandString(c.YamlFile)
		if err != nil {
			return errors.WithStack(err)
		}

		filename := filepath.Join(conf.WorkDir, c.YamlFile)

		_, err = os.Stat(filename)
		if os.IsNotExist(err) {
			if c.IgnoreMissingFile {
				return nil
			}
			return errors.Errorf("file '%s' does not exist", filename)
		}
		err = c.ParseFromFile(filename)
		if err != nil {
			return err
		}

		logger.Task().Infof("Saving downstream parameters to patch with keys from file: %s", filename)

		var patch *patch.Patch
		patch, err = comm.GetTaskPatch(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})
		if err != nil {
			return err
		}

		patchData := apimodels.PatchData{
			PatchId:          patch.Id.Hex(),
			DownstreamParams: c.DownstreamParams,
		}
		err = comm.SetDownstreamParams(ctx, patchData, conf.Task.Id)

		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil

}

func (c *setDownstream) ParseFromFile(filename string) error {
	filedata, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	params_from_file := make(map[string]string)
	err = yaml.Unmarshal(filedata, params_from_file)
	if err != nil {
		return err
	}
	for k, v := range params_from_file {
		param := patch.Parameter{
			Key:   k,
			Value: v,
		}
		c.DownstreamParams = append(c.DownstreamParams, param)
	}

	return nil
}
