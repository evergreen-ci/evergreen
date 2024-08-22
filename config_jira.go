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
	Host            string              `yaml:"host" bson:"host" json:"host"`
	BasicAuthConfig JiraBasicAuthConfig `yaml:"basic_auth" bson:"basic_auth" json:"basic_auth"`
	OAuth1Config    JiraOAuth1Config    `yaml:"oauth1" bson:"oauth1" json:"oauth1"`
	DefaultProject  string              `yaml:"default_project" bson:"default_project" json:"default_project"`
	Email           string              `yaml:"email" bson:"email" json:"email"`
}

type JiraBasicAuthConfig struct {
	Username string `yaml:"username" bson:"username" json:"username"`
	Password string `yaml:"password" bson:"password" json:"password"`
}

type JiraOAuth1Config struct {
	PrivateKey  string `yaml:"private_key" bson:"private_key" json:"private_key"`
	AccessToken string `yaml:"access_token" bson:"access_token" json:"access_token"`
	TokenSecret string `yaml:"token_secret" bson:"token_secret" json:"token_secret"`
	ConsumerKey string `yaml:"consumer_key" bson:"consumer_key" json:"consumer_key"`
}

func (c *JiraConfig) SectionId() string { return "jira" }

func (c *JiraConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *JiraConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			"host":            c.Host,
			"basic_auth":      c.BasicAuthConfig,
			"oauth1":          c.OAuth1Config,
			"default_project": c.DefaultProject,
			"email":           c.Email,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *JiraConfig) ValidateAndDefault() error {
	if (c.Host != "") && (c.BasicAuthConfig.Username != "") == (c.OAuth1Config.AccessToken != "") {
		return errors.New("must specify exactly 1 Jira auth method")
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
		BasicAuthOpts: send.JiraBasicAuth{
			Username:     c.BasicAuthConfig.Username,
			Password:     c.BasicAuthConfig.Password,
			UseBasicAuth: true,
		},
		Oauth1Opts: send.JiraOauth1{
			AccessToken: c.OAuth1Config.AccessToken,
			TokenSecret: c.OAuth1Config.TokenSecret,
			PrivateKey:  []byte(c.OAuth1Config.PrivateKey),
			ConsumerKey: c.OAuth1Config.ConsumerKey,
		},
	}
}
