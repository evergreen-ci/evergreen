package evergreen

import (
	"context"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

type SplunkConfig struct {
	SplunkConnectionInfo send.SplunkConnectionInfo `bson:",inline" json:"splunk_connection_info" yaml:"splunk_connection_info"`
}

func (c *SplunkConfig) SectionId() string { return "splunk" }

func (c *SplunkConfig) Get(ctx context.Context) error {
	// SSM parameters may not be available such as when running locally.
	grip.Error(message.WrapError(decodeParameter(ctx, c), message.Fields{
		"section_id": c.SectionId(),
		"message":    "getting config section from SSM",
	}))
	return errors.Wrapf(decodeDBConfig(ctx, c), "getting config section '%s' from the database", c.SectionId())
}

func (c *SplunkConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setParameter(ctx, c), "setting config section '%s'", c.SectionId())
}

func (c *SplunkConfig) ValidateAndDefault() error { return nil }
