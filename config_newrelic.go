package evergreen

import (
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type NewRelicConfig struct {
	AccountID     string `bson:"accountId" json:"accountId" yaml:"accountId"`
	TrustKey      string `bson:"trustKey" json:"trustKey" yaml:"trustKey"`
	AgentID       string `bson:"agentId" json:"agentId" yaml:"agentId"`
	LicenseKey    string `bson:"licenseKey" json:"licenseKey" yaml:"licenseKey"`
	ApplicationID string `bson:"applicationId" json:"applicationId" yaml:"applicationId"`
}

func (c *NewRelicConfig) SectionId() string { return "newrelic" }

func (c *NewRelicConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = NewRelicConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}
	return nil
}

func (c *NewRelicConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"accountId":     c.AccountID,
			"trustKey":      c.TrustKey,
			"agentId":       c.AgentID,
			"licenseKey":    c.LicenseKey,
			"applicationId": c.ApplicationID,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *NewRelicConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()

	allFieldsAreEmpty := c.AccountID == "" && c.TrustKey == "" && c.AgentID == "" && c.LicenseKey == "" && c.ApplicationID == ""
	allFieldsAreFilledOut := len(c.AccountID) > 0 && len(c.TrustKey) > 0 && len(c.AgentID) > 0 && len(c.LicenseKey) > 0 && len(c.ApplicationID) > 0

	if allFieldsAreEmpty == false && allFieldsAreFilledOut == false {
		catcher.Add(errors.New("Must provide all fields or no fields for New Relic settings"))
	}

	return catcher.Resolve()
}
