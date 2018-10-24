package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

type TriggerConfig struct {
	GenerateTaskDistro string `bson:"generate_distro" json:"generate_distro" yaml:"generate_distro"`
}

func (c *TriggerConfig) SectionId() string { return "triggers" }
func (c *TriggerConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = TriggerConfig{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}
func (c *TriggerConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"generate_distro": c.GenerateTaskDistro,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}
func (c *TriggerConfig) ValidateAndDefault() error {
	return nil
}
