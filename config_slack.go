package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

type SlackConfig struct {
	Options *send.SlackOptions `bson:"options" json:"options" yaml:"options"`
	Token   string             `bson:"token" json:"token" yaml:"token"`
	Level   string             `bson:"level" json:"level" yaml:"level"`
}

func (c *SlackConfig) SectionId() string { return "slack" }

func (c *SlackConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = SlackConfig{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *SlackConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"options": c.Options,
			"token":   c.Token,
			"level":   c.Level,
		},
	})
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

		if !level.IsValidPriority(level.FromString(c.Level)) {
			return errors.Errorf("%s is not a valid priority", c.Level)
		}
	}

	return nil
}
