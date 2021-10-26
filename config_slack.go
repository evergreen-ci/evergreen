package evergreen

import (
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SlackConfig struct {
	Options *send.SlackOptions `bson:"options" json:"options" yaml:"options"`
	Token   string             `bson:"token" json:"token" yaml:"token"`
	Level   string             `bson:"level" json:"level" yaml:"level"`
}

func (c *SlackConfig) SectionId() string { return "slack" }

func (c *SlackConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = SlackConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	// Clear the struct because Decode will not set fields that are omitempty to
	// the zero value if they're zero in the database.
	*c = SlackConfig{}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *SlackConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"options": c.Options,
			"token":   c.Token,
			"level":   c.Level,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *SlackConfig) ValidateAndDefault() error {
	if c.Options == nil {
		c.Options = &send.SlackOptions{}
	}

	if c.Token != "" {
		if c.Options.Channel == "" {
			c.Options.Channel = "#evergreen-ops-alerts"
		}

		if c.Options.Name == "" {
			c.Options.Name = "evergreen"
		}

		if err := c.Options.Validate(); err != nil {
			return errors.Wrap(err, "with a non-empty token, you must specify a valid slack configuration")
		}

		if !level.FromString(c.Level).IsValid() {
			return errors.Errorf("%s is not a valid priority", c.Level)
		}
	}

	return nil
}
