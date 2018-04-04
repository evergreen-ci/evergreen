package evergreen

import (
	"strings"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// JiraConfig stores auth info for interacting with Atlassian Jira.
type JiraConfig struct {
	Host           string `yaml:"host" bson:"host" json:"host"`
	Username       string `yaml:"username" bson:"username" json:"username"`
	Password       string `yaml:"password" bson:"password" json:"password"`
	DefaultProject string `yaml:"default_project" bson:"default_project" json:"default_project"`
}

func (c *JiraConfig) SectionId() string { return "jira" }

func (c *JiraConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = JiraConfig{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *JiraConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"host":            c.Host,
			"username":        c.Username,
			"password":        c.Password,
			"default_project": c.DefaultProject,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *JiraConfig) ValidateAndDefault() error { return nil }

func (c JiraConfig) GetHostURL() string {
	if strings.HasPrefix("http", c.Host) {
		return c.Host
	}

	return "https://" + c.Host
}
