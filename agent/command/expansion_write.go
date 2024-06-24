package command

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
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
	for expansionKey, expansionValue := range conf.Expansions.Map() {
		if !c.redactExpansion(expansionKey, conf) {
			expansions[expansionKey] = expansionValue
		}
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

func (c *expansionsWriter) redactExpansion(key string, conf *internal.TaskConfig) bool {
	// Always redact the global GitHub and AWS expansions.
	if utility.StringSliceContains(globals.ExpansionsToRedact, key) {
		return true
	}

	// Redact a private variable if the option to include Redacted variables
	// isn't set to true.
	if utility.StringSliceContains(conf.Redacted, key) && !c.Redacted {
		return true
	}

	// If the expansion is listed to redact when it was added, redact it.
	if utility.StringSliceContains(conf.NewExpansions.GetRedacted(), key) {
		return true
	}

	return false
}
