package command

import (
	"context"
	"io/ioutil"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	yaml "gopkg.in/20210107192922/yaml.v3"
)

var (
	expansionsToRedact = []string{
		evergreen.GlobalGitHubTokenExpansion,
		AWSAccessKeyId,
		AWSSecretAccessKey,
		AWSSessionToken,
	}
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
	_ client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	expansions := map[string]string{}
	for k, v := range conf.Expansions.Map() {
		_, ok := conf.Redacted[k]
		// Users should not be able to use the global github token expansion
		// as it can result in the breaching of Evergreen's GitHub API limit.
		// Likewise with AWS expansions.
		if (ok && !c.Redacted) || utility.StringSliceContains(expansionsToRedact, k) {
			continue
		}
		expansions[k] = v
	}
	out, err := yaml.Marshal(expansions)
	if err != nil {
		return errors.Wrap(err, "error marshaling expansions")
	}
	fn := getJoinedWithWorkDir(conf, c.File)
	if err := ioutil.WriteFile(fn, out, 0600); err != nil {
		return errors.Wrapf(err, "error writing expansions to file (%s)", fn)
	}
	logger.Task().Infof("expansions written to file (%s)", fn)
	return nil
}
