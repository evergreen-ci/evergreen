package command

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

var (
	ExpansionsToRedact = []string{
		evergreen.GlobalGitHubTokenExpansion,
		evergreen.GithubAppToken,
		// HostServicePasswordExpansion exists to redact the host's ServicePassword in the logs,
		// which is used for some jasper commands for Windows hosts. It is populated as a default
		// expansion only for tasks running on Windows hosts.
		evergreen.HostServicePasswordExpansion,
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
		return errors.Wrap(err, "decoding mapstructure params")
	}

	return nil
}

func (c *expansionsWriter) Execute(ctx context.Context,
	_ client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	expansions := map[string]string{}
	for k, v := range conf.Expansions.Map() {
		_, ok := conf.Redacted[k]
		// Redact private variables unless redacted set to true. Always
		// redact the global GitHub and AWS expansions.
		if (ok && !c.Redacted) || utility.StringSliceContains(ExpansionsToRedact, k) {
			continue
		}

		expansions[k] = v
	}
	out, err := yaml.Marshal(expansions)
	if err != nil {
		return errors.Wrap(err, "marshalling expansions")
	}
	fn := GetWorkingDirectory(conf, c.File)
	if err := os.WriteFile(fn, out, 0600); err != nil {
		return errors.Wrapf(err, "writing expansions to file '%s'", fn)
	}
	logger.Task().Infof("Expansions written to file '%s'.", fn)
	return nil
}
