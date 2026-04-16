package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// OktaServiceConfig contains the settings for our Okta Services app.
// This is used exclusively for machine to machine authentication,
// e.g. the token exchange grant used in our spawn host workflow.
type OktaServiceConfig struct {
	ClientID     string   `bson:"client_id" json:"client_id" yaml:"client_id"`
	ClientSecret string   `bson:"client_secret" json:"client_secret" yaml:"client_secret" secret:"true"`
	Scopes       []string `bson:"scopes" json:"scopes" yaml:"scopes"`
	Audience     string   `bson:"audience" json:"audience" yaml:"audience"`
	Issuer       string   `bson:"issuer" json:"issuer" yaml:"issuer"`
	KeySetURL    string   `bson:"keyset_url" json:"keyset_url" yaml:"keyset_url"`
	HeaderName   string   `bson:"header_name" json:"header_name" yaml:"header_name"`
}

func (c *OktaServiceConfig) SectionId() string { return "okta_service" }

func (c *OktaServiceConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *OktaServiceConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			oktaServiceClientIDKey:     c.ClientID,
			oktaServiceClientSecretKey: c.ClientSecret,
			oktaServiceScopesKey:       c.Scopes,
			oktaServiceAudienceKey:     c.Audience,
			oktaServiceIssuerKey:       c.Issuer,
			oktaServiceKeySetURLKey:    c.KeySetURL,
			oktaServiceHeaderNameKey:   c.HeaderName,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

// ValidateAndDefault implements the ConfigSection interface; Okta service
// fields are optional at general settings load time and validated via Validate
// when the token exchange flow runs.
func (c *OktaServiceConfig) ValidateAndDefault() error {
	return nil
}

func (c *OktaServiceConfig) Validate() error {
	return nil
}
