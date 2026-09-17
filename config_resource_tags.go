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
