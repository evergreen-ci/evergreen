package evergreen

import (
	"context"

	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SplunkConfig struct {
	SplunkConnectionInfo send.SplunkConnectionInfo `bson:",inline" json:"splunk_connection_info" yaml:"splunk_connection_info"`
}

func (c *SplunkConfig) SectionId() string { return "splunk" }

func (c *SplunkConfig) Get(ctx context.Context) error {
	if err := decodeParameter(ctx, c); err != nil && !errors.Is(err, ssmDisabledErr) {
		return errors.Wrapf(err, "getting config section '%s' from SSM", c.SectionId())
	}
	return errors.Wrapf(decodeDBConfig(ctx, c), "getting config section '%s' from the database", c.SectionId())
}

func (c *SplunkConfig) Set(ctx context.Context) error {
	if err := setParameter(ctx, c); err == nil || !errors.Is(err, ssmDisabledErr) {
		return errors.Wrapf(err, "setting config section '%s' in SSM", c.SectionId())
	}

	// When SSM is disabled set the value in the database.
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"url":     c.SplunkConnectionInfo.ServerURL,
			"token":   c.SplunkConnectionInfo.Token,
			"channel": c.SplunkConnectionInfo.Channel,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *SplunkConfig) ValidateAndDefault() error { return nil }
