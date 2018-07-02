package evergreen

import (
	"text/template"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type JIRANotificationsConfig struct {
	CustomFields map[string]JIRAProjectFields `bson:"custom_fields"`
}

// JIRAProjectFields is a map of JIRA field names to Golang template strings
// If the expanded template resolves to a slice, the slice will be handed to
// JIRA without any manipulation
type JIRAProjectFields map[string]string

func (c *JIRANotificationsConfig) SectionId() string { return "jira_notifications" }

func (c *JIRANotificationsConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = JIRANotificationsConfig{}
		return nil
	}

	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *JIRANotificationsConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), c)
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *JIRANotificationsConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	for _, project := range c.CustomFields {
		for _, tmpl := range project {
			_, err := template.New("jira_notification").Parse(tmpl)
			catcher.Add(err)
		}
	}
	return catcher.Resolve()
}
