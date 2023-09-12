package command

import (
	"context"
	"strconv"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type keyValInc struct {
	Key         string `mapstructure:"key"`
	Destination string `mapstructure:"destination"`
	base
}

func keyValIncFactory() Command   { return &keyValInc{} }
func (c *keyValInc) Name() string { return "keyval.inc" }

// ParseParams validates the input to the keyValInc, returning an error
// if something is incorrect. Fulfills Command interface.
func (c *keyValInc) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if c.Key == "" || c.Destination == "" {
		return errors.New("both key and destination must be set")
	}

	return nil
}

// Execute sends the request to increment the value for the key and sets the
// destination expansion's value.
func (c *keyValInc) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	keyVal := model.KeyVal{Key: c.Key}
	err := comm.KeyValInc(ctx, td, &keyVal)
	if err != nil {
		return errors.Wrapf(err, "incrementing key '%s'", c.Key)
	}

	conf.Expansions.Put(c.Destination, strconv.FormatInt(keyVal.Value, 10))
	return nil
}
