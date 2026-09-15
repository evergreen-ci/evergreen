package evergreen

import (
	"context"
	"net/mail"
	"strings"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

var validMongoDBEnvironments = map[string]struct{}{
	"prod":    {},
	"staging": {},
	"dev":     {},
	"qa":      {},
	"test":    {},
	"local":   {},
	"poc":     {},
	"demo":    {},
	"uat":     {},
	"sandbox": {},
}

type ResourceTagsConfig struct {
	MongoDBEnv   string `yaml:"mongodb_env" bson:"mongodb_env" json:"mongodb_env"`
	MongoDBOwner string `yaml:"mongodb_owner" bson:"mongodb_owner" json:"mongodb_owner"`
}

var (
	resourceTagsMongoDBEnvKey   = bsonutil.MustHaveTag(ResourceTagsConfig{}, "MongoDBEnv")
	resourceTagsMongoDBOwnerKey = bsonutil.MustHaveTag(ResourceTagsConfig{}, "MongoDBOwner")
)

func (*ResourceTagsConfig) SectionId() string { return "resource_tags" }

func (c *ResourceTagsConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *ResourceTagsConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			resourceTagsMongoDBEnvKey:   c.MongoDBEnv,
			resourceTagsMongoDBOwnerKey: c.MongoDBOwner,
		},
	}), "updating config section '%s'", c.SectionId())
}

func (c *ResourceTagsConfig) ValidateAndDefault() error {
	if c.MongoDBEnv != "" {
		if _, ok := validMongoDBEnvironments[c.MongoDBEnv]; !ok {
			return errors.Errorf("invalid MongoDB environment '%s'", c.MongoDBEnv)
		}
	}
	if c.MongoDBOwner != "" {
		parsed, err := mail.ParseAddress(c.MongoDBOwner)
		if err != nil || parsed.Address != c.MongoDBOwner {
			return errors.Errorf("invalid MongoDB owner email '%s'", c.MongoDBOwner)
		}
		domain := strings.TrimPrefix(parsed.Address, parsed.Address[:strings.LastIndex(parsed.Address, "@")+1])
		if !strings.Contains(domain, ".") {
			return errors.Errorf("invalid MongoDB owner email '%s'", c.MongoDBOwner)
		}
	}
	return nil
}
