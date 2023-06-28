package evergreen

import (
	"context"
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

func (c *JIRANotificationsConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = JIRANotificationsConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *JIRANotificationsConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).ReplaceOne(ctx, byId(c.SectionId()), c, options.Replace().SetUpsert(true))
	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *JIRANotificationsConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	projectSet := make(map[string]bool)
	for _, project := range c.CustomFields {
		if projectSet[project.Project] {
			catcher.Errorf("duplicate project key '%s'", project.Project)
			continue
		}
		projectSet[project.Project] = true

		fieldSet := make(map[string]bool)
		for _, field := range project.Fields {
			if fieldSet[field.Field] {
				catcher.Errorf("duplicate field key '%s' in project '%s'", field.Field, project.Project)
				continue
			}
			fieldSet[field.Field] = true

			_, err := template.New(fmt.Sprintf("%s-%s", project.Project, field.Field)).Parse(field.Template)
			catcher.Add(err)
		}
	}

	return catcher.Resolve()
}
