package command

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v3"
)

// setExpansions takes a file of key value pairs and
// sets the downstream parameters for a patch
type setDownstream struct {
	// Filename for a yaml file containing key-value pairs
	YamlFile          string `mapstructure:"file"`
	IgnoreMissingFile bool   `mapstructure:"ignore_missing_file"`
	base

	// Key-value pairs for updating the task's parameters with
	downstreamParams []patch.Parameter
}

func setExpansionsFactory() Command   { return &setDownstream{} }
func (c *setDownstream) Name() string { return "downstream_expansions.set" }

// ParseParams validates the input to setDownstream, returning and error
// if something is incorrect. Fulfills Command interface.
func (c *setDownstream) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrapf(err, "error parsing '%v' params", c.Name())
	}

	if c.YamlFile == "" {
		return errors.New("file cannot be blank")
	}

	return nil
}

// Execute updates the expansions. Fulfills Command interface.
// params are not expanded as part of Execute, they will be expanded as part of parameterized builds.
func (c *setDownstream) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	var err error

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
	logger.Task().Infof("Saving downstream parameters to patch with keys from file: %s", c.YamlFile)

	if len(c.downstreamParams) != 0 {
		err = comm.SetDownstreamParams(ctx, c.downstreamParams, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})
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
		c.downstreamParams = append(c.downstreamParams, param)
	}

	return nil
}
