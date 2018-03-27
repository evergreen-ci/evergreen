package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	newrelic "github.com/newrelic/go-agent"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

type NewRelicConfig struct {
	ApplicationName string `bson:"application_name" json:"application_name" yaml:"application_name"`
	LicenseKey      string `bson:"license_key" json:"license_key" yaml:"license_key"`
}

func (c *NewRelicConfig) SectionId() string { return "new_relic" }

func (c *NewRelicConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = NewRelicConfig{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *NewRelicConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"application_name": c.ApplicationName,
			"license_key":      c.LicenseKey,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *NewRelicConfig) ValidateAndDefault() error { return nil }

func (c *NewRelicConfig) SetUp() (newrelic.Application, error) {
	if c.ApplicationName == "" || c.LicenseKey == "" {
		return nil, nil
	}
	config := newrelic.NewConfig(c.ApplicationName, c.LicenseKey)
	app, err := newrelic.NewApplication(config)
	if err != nil || app == nil {
		return nil, errors.Wrap(err, "error creating New Relic application")
	}
	return app, nil
}
