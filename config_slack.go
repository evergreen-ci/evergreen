package evergreen

import (
	"context"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type SlackConfig struct {
	Options *send.SlackOptions `bson:"options" json:"options" yaml:"options"`
	Token   string             `bson:"token" json:"token" yaml:"token" secret:"true"`
	Level   string             `bson:"level" json:"level" yaml:"level"`
	Name    string             `bson:"name" json:"name" yaml:"name"`
}

func (c *SlackConfig) SectionId() string { return "slack" }

func (c *SlackConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *SlackConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			"options": c.Options,
			"token":   c.Token,
			"level":   c.Level,
			"name":    c.Name,
		}}), "updating config section '%s'", c.SectionId(),
	)
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
			return errors.Wrap(err, "with a non-empty token, you must specify a valid Slack configuration")
		}

		if c.Level != "" && !level.FromString(c.Level).IsValid() {
			return errors.Errorf("%s is not a valid priority", c.Level)
		}
	}

	return nil
}
