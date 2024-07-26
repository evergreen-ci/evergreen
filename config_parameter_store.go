package evergreen

import (
	"context"

	"github.com/evergreen-ci/evergreen/parameterstore"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ParameterStoreConfig configures Parameter Store. If not SSMBackend data will be stored in the database
// and not Parameter Store.
type ParameterStoreConfig struct {
	SSMBackend bool   `yaml:"ssm_backend" bson:"ssm_backend" json:"ssm_backend"`
	Prefix     string `yaml:"prefix" bson:"prefix" json:"prefix"`
}

// SectionId returns the ID of this config section.
func (c *ParameterStoreConfig) SectionId() string { return parameterStoreConfigID }

// Get populates the config from the database.
func (c *ParameterStoreConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = ParameterStoreConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(&c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

// Set sets the document in the database to match the in-memory config struct.
func (c *ParameterStoreConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			parameterStoreSSMBackendKey: c.SSMBackend,
			parameterStorePrefixKey:     c.Prefix,
		},
	}, options.Update().SetUpsert(true))
	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

// ValidateAndDefault validates the tracer configuration.
func (c *ParameterStoreConfig) ValidateAndDefault() error {
	return nil
}

func GetParameterStoreOpts(ctx context.Context) (parameterstore.ParameterStoreOptions, error) {
	var config = ParameterStoreConfig{}
	if err := config.Get(ctx); err != nil {
		return parameterstore.ParameterStoreOptions{}, errors.Wrap(err, "getting Parameter Store options from the database")
	}
	return parameterstore.ParameterStoreOptions{
		Database:   GetEnvironment().DB(),
		SSMBackend: config.SSMBackend,
		Prefix:     config.Prefix,
	}, nil
}
