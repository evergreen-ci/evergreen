package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type TriggerConfig struct {
	GenerateTaskDistro string `bson:"generate_distro" json:"generate_distro" yaml:"generate_distro"`
}

func (c *TriggerConfig) SectionId() string { return "triggers" }

func (c *TriggerConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *TriggerConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			"generate_distro": c.GenerateTaskDistro,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *TriggerConfig) ValidateAndDefault() error {
	return nil
}
