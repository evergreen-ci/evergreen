package evergreen

import (
	"context"
	"encoding/json"

	"github.com/evergreen-ci/evergreen/parameterstore"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

type SplunkConfig struct {
	SplunkConnectionInfo send.SplunkConnectionInfo `json:"splunk_connection_info" yaml:"splunk_connection_info"`
}

func (c *SplunkConfig) SectionId() string { return "splunk" }

func (c *SplunkConfig) Get(ctx context.Context) error {
	parameterStoreOpts, err := GetParameterStoreOpts(ctx)
	if err != nil {
		return errors.Wrap(err, "getting Parameter Store options")
	}
	parameterStore, err := parameterstore.NewParameterStore(ctx, parameterStoreOpts)
	if err != nil {
		return errors.Wrap(err, "getting Parameter Store client")
	}

	data, err := parameterStore.GetParameter(ctx, adminParameterName(c.SectionId()))
	if err != nil {
		return errors.Wrapf(err, "getting config section '%s' from Parameter Store", c.SectionId())
	}
	return errors.Wrapf(json.Unmarshal([]byte(data), c), "decoding config section '%s'", c.SectionId())
}

func (c *SplunkConfig) Set(ctx context.Context) error {
	data, err := json.Marshal(c)
	if err != nil {
		return errors.Wrapf(err, "marshalling config section '%s' as json", c.SectionId())
	}

	parameterStoreOpts, err := GetParameterStoreOpts(ctx)
	if err != nil {
		return errors.Wrap(err, "getting Parameter Store options")
	}
	parameterStore, err := parameterstore.NewParameterStore(ctx, parameterStoreOpts)
	if err != nil {
		return errors.Wrap(err, "getting Parameter Store client")
	}
	return errors.Wrapf(parameterStore.SetParameter(ctx, adminParameterName(c.SectionId()), string(data)), "setting config section '%s' in Parameter Store", c.SectionId())
}

func (c *SplunkConfig) ValidateAndDefault() error { return nil }
