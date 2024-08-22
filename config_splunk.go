package evergreen

import (
	"context"

	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type SplunkConfig struct {
	SplunkConnectionInfo send.SplunkConnectionInfo `bson:",inline" json:"splunk_connection_info" yaml:"splunk_connection_info"`
}

func (c *SplunkConfig) SectionId() string { return "splunk" }

func (c *SplunkConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *SplunkConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			"url":     c.SplunkConnectionInfo.ServerURL,
			"token":   c.SplunkConnectionInfo.Token,
			"channel": c.SplunkConnectionInfo.Channel,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *SplunkConfig) ValidateAndDefault() error { return nil }
