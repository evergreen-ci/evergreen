package command

import (
	"context"
	"io/ioutil"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

type expansionsWriter struct {
	File     string `mapstructure:"file" plugin:"expand"`
	Redacted bool   `mapstructure:"redacted"`

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
	_ client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	expansions := map[string]string{}
	for k, v := range conf.Expansions.Map() {
		_, ok := conf.Redacted[k]
		if ok && !c.Redacted {
			continue
		}
		expansions[k] = v
	}
	out, err := yaml.Marshal(expansions)
	if err != nil {
		return errors.Wrap(err, "error marshaling expansions")
	}
	fn := filepath.Join(conf.WorkDir, c.File)
	if err := ioutil.WriteFile(fn, out, 0600); err != nil {
		return errors.Wrapf(err, "error writing expansions to file (%s)", fn)
	}
	logger.Task().Infof("expansions written to file (%s)", fn)
	return nil

}
