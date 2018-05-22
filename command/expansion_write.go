package command

import (
	"context"
	"io/ioutil"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

type expansionsWriter struct {
	File string `mapstructure:"file"`

	base
}

func writeExpansionsFactory() Command    { return &expansionsWriter{} }
func (c *expansionsWriter) Name() string { return "expansions.write" }

func (c *expansionsWriter) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrap(err, "couldn't decode params")
	}

	return nil
}

func (c *expansionsWriter) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	out, err := yaml.Marshal(conf.Expansions)
	if err != nil {
		return errors.Wrap(err, "error marshaling expansions")
	}
	if err := ioutil.WriteFile(c.File, out, 0600); err != nil {
		return errors.Wrapf(err, "error writing expansions to file (%s)", c.File)
	}
	return nil

}
