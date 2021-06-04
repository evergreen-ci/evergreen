package evergreen

import (
	"net/url"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UIConfig holds relevant settings for the UI server.
type UIConfig struct {
	Url            string   `bson:"url" json:"url" yaml:"url"`
	HelpUrl        string   `bson:"help_url" json:"help_url" yaml:"helpurl"`
	UIv2Url        string   `bson:"uiv2_url" json:"uiv2_url" yaml:"uiv2_url"`
	HttpListenAddr string   `bson:"http_listen_addr" json:"http_listen_addr" yaml:"httplistenaddr"`
	Secret         string   `bson:"secret" json:"secret" yaml:"secret"`                           // Secret to encrypt session storage
	DefaultProject string   `bson:"default_project" json:"default_project" yaml:"defaultproject"` // Default project to assume when none specified
	CacheTemplates bool     `bson:"cache_templates" json:"cache_templates" yaml:"cachetemplates"` // Cache results of template compilation
	CsrfKey        string   `bson:"csrf_key" json:"csrf_key" yaml:"csrfkey"`                      // 32-byte key used to generate tokens that validate UI requests
	CORSOrigins    []string `bson:"cors_origins" json:"cors_origins" yaml:"cors_origins"`         // allowed request origins for some UI Routes
	LoginDomain    string   `bson:"login_domain" json:"login_domain" yaml:"login_domain"`         // domain for the login cookie (defaults to domain of app)
	UserVoice      string   `bson:"userVoice" json:"userVoice" yaml:"userVoice"`
}

func (c *UIConfig) SectionId() string { return "ui" }

func (c *UIConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = UIConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}
	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}
	return nil
}

func (c *UIConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"url":              c.Url,
			"help_url":         c.HelpUrl,
			"uiv2_url":         c.UIv2Url,
			"http_listen_addr": c.HttpListenAddr,
			"secret":           c.Secret,
			"default_project":  c.DefaultProject,
			"cache_templates":  c.CacheTemplates,
			"csrf_key":         c.CsrfKey,
			"cors_origins":     c.CORSOrigins,
			"login_domain":     c.LoginDomain,
			"userVoice":        c.UserVoice,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *UIConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	if c.Secret == "" {
		catcher.Add(errors.New("UI Secret must not be empty"))
	}
	if c.DefaultProject == "" {
		catcher.Add(errors.New("You must specify a default project in UI"))
	}
	if c.Url == "" {
		catcher.Add(errors.New("You must specify a default UI url"))
	}
	if c.CsrfKey != "" && len(c.CsrfKey) != 32 {
		catcher.Add(errors.New("CSRF key must be 32 characters long"))
	}
	for _, origin := range c.CORSOrigins {
		if _, err := url.Parse(origin); err != nil {
			if origin != "*" {
				catcher.Add(errors.Wrap(err, "CORS Origin must be a valid URL or '*'"))
			}
		}
	}

	return catcher.Resolve()
}
