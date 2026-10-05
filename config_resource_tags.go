package evergreen

import (
	"net/mail"
	"strings"

	"github.com/pkg/errors"
)

const (
	MongoDBEnvironmentProd    = "prod"
	MongoDBEnvironmentStaging = "staging"
	MongoDBEnvironmentDev     = "dev"
	MongoDBEnvironmentQA      = "qa"
	MongoDBEnvironmentTest    = "test"
	MongoDBEnvironmentLocal   = "local"
	MongoDBEnvironmentPOC     = "poc"
	MongoDBEnvironmentDemo    = "demo"
	MongoDBEnvironmentUAT     = "uat"
	MongoDBEnvironmentSandbox = "sandbox"
)

var validMongoDBEnvironments = map[string]struct{}{
	MongoDBEnvironmentProd:    {},
	MongoDBEnvironmentStaging: {},
	MongoDBEnvironmentDev:     {},
	MongoDBEnvironmentQA:      {},
	MongoDBEnvironmentTest:    {},
	MongoDBEnvironmentLocal:   {},
	MongoDBEnvironmentPOC:     {},
	MongoDBEnvironmentDemo:    {},
	MongoDBEnvironmentUAT:     {},
	MongoDBEnvironmentSandbox: {},
}

type ResourceTagsConfig struct {
	MongoDBEnv   string `yaml:"mongodb_env" bson:"mongodb_env" json:"mongodb_env"`
	MongoDBOwner string `yaml:"mongodb_owner" bson:"mongodb_owner" json:"mongodb_owner"`
}

func (c *ResourceTagsConfig) Validate() error {
	if c.MongoDBEnv != "" {
		if _, ok := validMongoDBEnvironments[c.MongoDBEnv]; !ok {
			return errors.Errorf("invalid MongoDB environment '%s'", c.MongoDBEnv)
		}
	}
	if c.MongoDBOwner != "" {
		if err := ValidateMongoDBEmail(c.MongoDBOwner); err != nil {
			return errors.Wrap(err, "validating MongoDB owner email")
		}
	}
	return nil
}

// ValidateMongoDBEmail checks that an email is a bare address on an approved MongoDB domain.
func ValidateMongoDBEmail(email string) error {
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || (!strings.HasSuffix(email, "@mongodb.com") && !strings.HasSuffix(email, "@10gen.com")) {
		return errors.New("email must be a valid address ending in @mongodb.com or @10gen.com")
	}
	return nil
}
