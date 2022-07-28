package evergreen

import (
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SplunkConfig struct {
	SplunkConnectionInfo send.SplunkConnectionInfo `bson:",inline" json:"splunk_connection_info" yaml:"splunk_connection_info"`
}

func (c *SplunkConfig) SectionId() string { return "splunk" }

func (c *SplunkConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err != mongo.ErrNoDocuments {
			return errors.Wrapf(err, "retrieving section '%s'", c.SectionId())
		}
		// TODO EVG-17353: remove retrieving settings from global
		globalConfig := coll.FindOne(ctx, byId(ConfigDocID))
		if err := globalConfig.Err(); err != nil {
			if err != mongo.ErrNoDocuments {
				return errors.Wrap(err, "retrieving global settings")
			}
			c = &SplunkConfig{}
			return nil
		}
		s := Settings{}
		if err := globalConfig.Decode(&s); err != nil {
			return errors.Wrap(err, "decoding global config")
		}
		c.SplunkConnectionInfo = s.Splunk.SplunkConnectionInfo
		return nil
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "decoding result")
	}

	return nil
}

func (c *SplunkConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"url":     c.SplunkConnectionInfo.ServerURL,
			"token":   c.SplunkConnectionInfo.Token,
			"channel": c.SplunkConnectionInfo.Channel,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating section '%s'", c.SectionId())
}

func (c *SplunkConfig) ValidateAndDefault() error { return nil }
