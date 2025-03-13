package evergreen

import (
	"context"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const defaultHostThrottle = 32

// HostInitConfig holds logging settings for the hostinit process.
type HostInitConfig struct {
	HostThrottle         int `bson:"host_throttle" json:"host_throttle" yaml:"host_throttle"`
	ProvisioningThrottle int `bson:"provisioning_throttle" json:"provisioning_throttle" yaml:"provisioning_throttle"`
	CloudStatusBatchSize int `bson:"cloud_batch_size" json:"cloud_batch_size" yaml:"cloud_batch_size"`
	MaxTotalDynamicHosts int `bson:"max_total_dynamic_hosts" json:"max_total_dynamic_hosts" yaml:"max_total_dynamic_hosts"`
}

func (c *HostInitConfig) SectionId() string { return "hostinit" }

func (c *HostInitConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *HostInitConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			hostInitHostThrottleKey:         c.HostThrottle,
			hostInitProvisioningThrottleKey: c.ProvisioningThrottle,
			hostInitCloudStatusBatchSizeKey: c.CloudStatusBatchSize,
			hostInitMaxTotalDynamicHostsKey: c.MaxTotalDynamicHosts,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *HostInitConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	if c.HostThrottle == 0 {
		c.HostThrottle = defaultHostThrottle
	}
	catcher.NewWhen(c.HostThrottle < 0, "host throttle cannot be negative")

	if c.ProvisioningThrottle == 0 {
		c.ProvisioningThrottle = 200
	}
	catcher.NewWhen(c.ProvisioningThrottle < 0, "host provisioning throttle cannot be negative")

	if c.CloudStatusBatchSize == 0 {
		c.CloudStatusBatchSize = 500
	}
	catcher.NewWhen(c.CloudStatusBatchSize < 0, "cloud host status batch size cannot be negative")

	if c.MaxTotalDynamicHosts == 0 {
		c.MaxTotalDynamicHosts = 5000
	}
	catcher.NewWhen(c.MaxTotalDynamicHosts < 0, "max total dynamic hosts cannot be negative")

	return catcher.Resolve()
}
