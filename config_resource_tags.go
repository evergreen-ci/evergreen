package evergreen

import (
	"net/mail"
	"strings"

	"github.com/pkg/errors"
)

// TODO (DEVPROD-44540): Move the allowed MongoDB owner email domains to admin settings.
const (
	mongoDBOwnerEmailSuffix       = "@mongodb.com"
	legacyMongoDBOwnerEmailSuffix = "@10gen.com"
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
	if err != nil || parsed.Address != email || (!strings.HasSuffix(email, mongoDBOwnerEmailSuffix) && !strings.HasSuffix(email, legacyMongoDBOwnerEmailSuffix)) {
		return errors.Errorf("email must be a valid address ending in %s or %s", mongoDBOwnerEmailSuffix, legacyMongoDBOwnerEmailSuffix)
	}
	return nil
}
