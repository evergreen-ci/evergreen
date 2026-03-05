package evergreen

import (
	"context"
	"net/url"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// BetaFeatures represents beta tests, which may be active or inactive.
type BetaFeatures struct {
	SpruceWaterfallEnabled bool `bson:"spruce_waterfall_enabled" json:"spruce_waterfall_enabled"`
	ParsleyAIEnabled       bool `bson:"parsley_ai_enabled" json:"parsley_ai_enabled"`
}

// UIConfig holds relevant settings for the UI server.
type UIConfig struct {
	Url                       string       `bson:"url" json:"url" yaml:"url"`
	UIv2Url                   string       `bson:"uiv2_url" json:"uiv2_url" yaml:"uiv2_url"`
	ParsleyUrl                string       `bson:"parsley_url" json:"parsley_url" yaml:"parsley_url"`
	HttpListenAddr            string       `bson:"http_listen_addr" json:"http_listen_addr" yaml:"httplistenaddr"`
	Secret                    string       `bson:"secret" json:"secret" yaml:"secret"`                                                                   // Secret to encrypt session storage
	DefaultProject            string       `bson:"default_project" json:"default_project" yaml:"defaultproject"`                                         // Default project to assume when none specified
	CacheTemplates            bool         `bson:"cache_templates" json:"cache_templates" yaml:"cachetemplates"`                                         // Cache results of template compilation
	CsrfKey                   string       `bson:"csrf_key" json:"csrf_key" yaml:"csrfkey"`                                                              // 32-byte key used to generate tokens that validate UI requests
	CORSOrigins               []string     `bson:"cors_origins" json:"cors_origins" yaml:"cors_origins"`                                                 // allowed request origins for some UI Routes
	FileStreamingContentTypes []string     `bson:"file_streaming_content_types" json:"file_streaming_content_types" yaml:"file_streaming_content_types"` // allowed content types for the file streaming route.
	LoginDomain               string       `bson:"login_domain" json:"login_domain" yaml:"login_domain"`                                                 // domain for the login cookie (defaults to domain of app)
	UserVoice                 string       `bson:"userVoice" json:"userVoice" yaml:"userVoice"`
	BetaFeatures              BetaFeatures `bson:"beta_features" json:"beta_features" yaml:"beta_features"`
	StagingEnvironment        string       `bson:"staging_environment,omitempty" json:"staging_environment" yaml:"staging_environment"`
}

func (c *UIConfig) SectionId() string { return "ui" }

func (c *UIConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *UIConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			"url":                          c.Url,
			"uiv2_url":                     c.UIv2Url,
			"parsley_url":                  c.ParsleyUrl,
			"http_listen_addr":             c.HttpListenAddr,
			"secret":                       c.Secret,
			"default_project":              c.DefaultProject,
			"cache_templates":              c.CacheTemplates,
			"csrf_key":                     c.CsrfKey,
			"cors_origins":                 c.CORSOrigins,
			"file_streaming_content_types": c.FileStreamingContentTypes,
			"login_domain":                 c.LoginDomain,
			"userVoice":                    c.UserVoice,
			"beta_features":                c.BetaFeatures,
			"staging_environment":          c.StagingEnvironment,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *UIConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	if c.Secret == "" {
		catcher.Add(errors.New("UI secret must not be empty"))
	}
	if c.DefaultProject == "" {
		catcher.Add(errors.New("must specify a default project in UI"))
	}
	if c.Url == "" {
		catcher.Add(errors.New("must specify a default UI url"))
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
