package evergreen

import (
	"strings"

	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// JiraConfig stores auth info for interacting with Atlassian Jira.
type JiraConfig struct {
	Host            string              `yaml:"host" bson:"host" json:"host"`
	BasicAuthConfig JiraBasicAuthConfig `yaml:"basic_auth" bson:"basic_auth" json:"basic_auth"`
	OAuth1Config    JiraOAuth1Config    `yaml:"oauth1" bson:"oauth1" json:"oauth1"`
	DefaultProject  string              `yaml:"default_project" bson:"default_project" json:"default_project"`
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

func (c *JiraConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = JiraConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	// Clear the struct because Decode will not set fields that are omitempty to
	// the zero value if they're zero in the database.
	*c = JiraConfig{}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *JiraConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"host":            c.Host,
			"basic_auth":      c.BasicAuthConfig,
			"oauth1":          c.OAuth1Config,
			"default_project": c.DefaultProject,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *JiraConfig) ValidateAndDefault() error {
	if (c.Host != "") && (c.BasicAuthConfig.Username != "") == (c.OAuth1Config.AccessToken != "") {
		return errors.New("must specify exactly 1 jira auth method")
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
