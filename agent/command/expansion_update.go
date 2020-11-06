package command

import (
	"context"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/rest/client"
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
		return err
	}

	for _, item := range c.Updates {
		if item.Key == "" {
			return errors.Errorf("error parsing '%v' params: key must not be "+
				"a blank string", c.Name())
		}
	}

	return nil
}

func (c *update) ExecuteUpdates(ctx context.Context, conf *internal.TaskConfig) error {
	for _, update := range c.Updates {
		if ctx.Err() != nil {
			return errors.New("operation aborted")
		}

		if update.Concat == "" {
			newValue, err := conf.Expansions.ExpandString(update.Value)

			if err != nil {
				return errors.WithStack(err)
			}
			conf.Expansions.Put(update.Key, newValue)
		} else {
			newValue, err := conf.Expansions.ExpandString(update.Concat)
			if err != nil {
				return errors.WithStack(err)
			}

			oldValue := conf.Expansions.Get(update.Key)
			conf.Expansions.Put(update.Key, oldValue+newValue)
		}
	}

	return nil
}

// Execute updates the expansions. Fulfills Command interface.
func (c *update) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	err := c.ExecuteUpdates(ctx, conf)

	if err != nil {
		return errors.WithStack(err)
	}

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

		logger.Task().Infof("Updating expansions with keys from file: %s", filename)
		err := conf.Expansions.UpdateFromYaml(filename)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil

}
