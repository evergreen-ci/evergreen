package evergreen

import (
	"fmt"
	"text/template"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type JIRANotificationsConfig struct {
	CustomFields []JIRANotificationsProject `bson:"custom_fields"`
}

type JIRANotificationsProject struct {
	Project    string                         `bson:"project"`
	Fields     []JIRANotificationsCustomField `bson:"fields"`
	Components []string                       `bson:"components"`
	Labels     []string                       `bson:"labels"`
}

type JIRANotificationsCustomField struct {
	Field    string `bson:"field"`
	Template string `bson:"template"`
}

func (c *JIRANotificationsConfig) SectionId() string { return "jira_notifications" }

func (c *JIRANotificationsConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = JIRANotificationsConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}
	return nil
}

func (c *JIRANotificationsConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.ReplaceOne(ctx, byId(c.SectionId()), c, options.Replace().SetUpsert(true))
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *JIRANotificationsConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	for _, project := range c.CustomFields {
		for _, field := range project.Fields {
			_, err := template.New(fmt.Sprintf("%s-%s", project.Project, field.Field)).Parse(field.Template)
			catcher.Add(err)
		}
	}

	return catcher.Resolve()
}
