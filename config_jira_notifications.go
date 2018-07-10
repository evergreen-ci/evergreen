package evergreen

import (
	"fmt"
	"text/template"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type JIRANotificationsConfig struct {
	// CustomFields is a map[string]map[string]string. The key of the first
	// map is the JIRA project (ex: EVG), the key of the second map is
	// the custom field name, and the inner most value is the template
	// for the custom field
	CustomFields util.KeyValuePairSlice `bson:"custom_fields"`
}

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
	m, err := c.CustomFields.NestedMap()
	if err != nil {
		return errors.Wrap(err, "failed to build jira notifications custom field")
	}

	for _, project := range m {
		for field, tmpl := range project {
			_, err := template.New(fmt.Sprintf("%s-%s", project, field)).Parse(tmpl)
			catcher.Add(err)
		}
	}
	return catcher.Resolve()
}
