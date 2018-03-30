package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// HostInitConfig holds logging settings for the hostinit process.
type HostInitConfig struct {
	SSHTimeoutSeconds int64 `bson:"ssh_timeout_secs" json:"ssh_timeout_secs" yaml:"sshtimeoutseconds"`
}

func (c *HostInitConfig) SectionId() string { return "hostinit" }

func (c *HostInitConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = HostInitConfig{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *HostInitConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"ssh_timeout_secs": c.SSHTimeoutSeconds,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *HostInitConfig) ValidateAndDefault() error { return nil }
