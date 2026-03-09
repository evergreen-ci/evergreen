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
	ClientID     string `bson:"client_id" json:"client_id" yaml:"client_id"`
	ClientSecret string `bson:"client_secret" json:"client_secret" yaml:"client_secret" secret:"true"`
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
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *OktaServiceConfig) ValidateAndDefault() error {
	return nil
}
