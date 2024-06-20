package command

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// update reads in a set of new expansions and updates the
// task's expansions at runtime. update can take a list
// of update expansion pairs and/or a file of expansion pairs
type update struct {
	// Key-value pairs for updating the task's parameters with
	Updates []updateParams `mapstructure:"updates"`

	// Filename for a yaml file containing expansion updates
	// in the form of
	//   "expansion_key: expansions_value"
	YamlFile string `mapstructure:"file"`

	IgnoreMissingFile bool `mapstructure:"ignore_missing_file"`

	base
}

// updateParams are pairings of expansion names
// and the value they expand to
type updateParams struct {
	// The name of the expansion
	Key string

	// The expanded value
	Value string

	// If the expansion should be redacted
	Redact bool

	// Can optionally concat a string to the end of the current value
	Concat string
}

func updateExpansionsFactory() Command { return &update{} }
func (c *update) Name() string         { return "expansions.update" }

// ParseParams validates the input to the update, returning and error
// if something is incorrect. Fulfills Command interface.
func (c *update) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	for i, item := range c.Updates {
		if item.Key == "" {
			return errors.Errorf("expansion key at index %d must not be a blank string", i)
		}
		if item.Value != "" && item.Concat != "" {
			return errors.Errorf("expansion key at index %d has both a value and a concat string", i)
		}
	}

	return nil
}

func (c *update) executeUpdates(ctx context.Context, conf *internal.TaskConfig) error {
	if conf.DynamicExpansions == nil {
		conf.DynamicExpansions = util.Expansions{}
	}
	for _, update := range c.Updates {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "operation aborted")
		}

		value := update.Value
		if update.Concat != "" {
			value = update.Concat
		}

		expanded, err := conf.NewExpansions.ExpandString(value)
		if err != nil {
			return errors.WithStack(err)
		}

		// If we are concating, the expanded value is not the whole new replacement-
		// it is the existing value plus the expanded value.
		if update.Concat != "" {
			existingValue := conf.NewExpansions.Get(update.Key)
			expanded = existingValue + expanded
		}

		conf.DynamicExpansions.Put(update.Key, expanded)
		if update.Redact {
			conf.NewExpansions.PutAndRedact(update.Key, expanded)
		} else {
			conf.NewExpansions.Put(update.Key, expanded)
		}
	}

	return nil
}

// Execute updates the expansions. Fulfills Command interface.
func (c *update) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	err := c.executeUpdates(ctx, conf)

	if err != nil {
		return errors.WithStack(err)
	}

	if c.YamlFile != "" {
		c.YamlFile, err = conf.NewExpansions.ExpandString(c.YamlFile)
		if err != nil {
			return errors.WithStack(err)
		}

		filename := GetWorkingDirectory(conf, c.YamlFile)

		_, err = os.Stat(filename)
		if os.IsNotExist(err) {
			if c.IgnoreMissingFile {
				return nil
			}
			return errors.Errorf("file '%s' does not exist", filename)
		}

		logger.Task().Infof("Updating expansions with keys from file '%s'.", filename)
		err := conf.NewExpansions.UpdateFromYaml(filename)
		if err != nil {
			return errors.WithStack(err)
		}
		if err = conf.DynamicExpansions.UpdateFromYaml(filename); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil

}
