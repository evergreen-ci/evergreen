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
	Url            string `bson:"url" json:"url" yaml:"url"`
	HelpUrl        string `bson:"help_url" json:"help_url" yaml:"helpurl"`
	HttpListenAddr string `bson:"http_listen_addr" json:"http_listen_addr" yaml:"httplistenaddr"`
	// Secret to encrypt session storage
	Secret string `bson:"secret" json:"secret" yaml:"secret"`
	// Default project to assume when none specified, e.g. when using
	// the /waterfall route use this project, while /waterfall/other-project
	// then use `other-project`
	DefaultProject string `bson:"default_project" json:"default_project" yaml:"defaultproject"`
	// Cache results of template compilation, so you don't have to re-read files
	// on every request. Note that if this is true, changes to HTML templates
	// won't take effect until server restart.
	CacheTemplates bool `bson:"cache_templates" json:"cache_templates" yaml:"cachetemplates"`
	// CsrfKey is a 32-byte key used to generate tokens that validate UI requests
	CsrfKey string `bson:"csrf_key" json:"csrf_key" yaml:"csrfkey"`
	// CORSOrigin is the allowed CORS Origin for some UI Routes
	CORSOrigin string `bson:"cors_origin" json:"cors_origin" yaml:"cors_origin"`
}

func (c *UIConfig) SectionId() string { return "ui" }

func (c *UIConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = UIConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
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
			"http_listen_addr": c.HttpListenAddr,
			"secret":           c.Secret,
			"default_project":  c.DefaultProject,
			"cache_templates":  c.CacheTemplates,
			"csrf_key":         c.CsrfKey,
			"cors_origin":      c.CORSOrigin,
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
	if _, err := url.Parse(c.CORSOrigin); err != nil {
		if c.CORSOrigin != "*" {
			catcher.Add(errors.Wrap(err, "CORS Origin must be a valid URL or '*'"))
		}
	}

	return catcher.Resolve()
}
