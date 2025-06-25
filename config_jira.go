package evergreen

import (
	"context"
	"strings"

	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// JiraConfig stores auth info for interacting with Atlassian Jira.
type JiraConfig struct {
	Host                string `yaml:"host" bson:"host" json:"host"`
	Email               string `yaml:"email" bson:"email" json:"email"`
	PersonalAccessToken string `yaml:"personal_access_token" bson:"personal_access_token" json:"personal_access_token" secret:"true"`
}

func (c *JiraConfig) SectionId() string { return "jira" }

func (c *JiraConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *JiraConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			"host":                  c.Host,
			"personal_access_token": c.PersonalAccessToken,
			"email":                 c.Email,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *JiraConfig) ValidateAndDefault() error {
	hostPopulated := c.Host != ""
	patPopulated := c.PersonalAccessToken != ""
	if hostPopulated && !patPopulated {
		return errors.New("must specify at least one Jira auth method")
	}
	return nil
}

func (c JiraConfig) GetHostURL() string {
	if strings.HasPrefix("http", c.Host) {
		return c.Host
	}

	return "https://" + c.Host
}

func (c JiraConfig) Export() *send.JiraOptions {
	return &send.JiraOptions{
		Name:    "evergreen",
		BaseURL: c.GetHostURL(),
		PersonalAccessTokenOpts: send.JiraPersonalAccessTokenAuth{
			Token: c.PersonalAccessToken,
		},
	}
}
