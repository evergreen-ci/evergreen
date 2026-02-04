package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// DebugSpawnHostsConfig holds configuration for debugging spawn hosts.
type DebugSpawnHostsConfig struct {
	SetupScript string `yaml:"setup_script" bson:"setup_script" json:"setup_script"`
}

func (c *DebugSpawnHostsConfig) SectionId() string { return "debug_spawn_hosts" }

func (c *DebugSpawnHostsConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *DebugSpawnHostsConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			setupScriptKey: c.SetupScript,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *DebugSpawnHostsConfig) ValidateAndDefault() error {
	return nil
}
