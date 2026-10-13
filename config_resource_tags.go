package evergreen

import (
	"net/mail"
	"regexp"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

const DefaultMongoDBOwner = "evergreen@mongodb.com"

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

var mongoDBOwnerEmailDomainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

type ResourceTagsConfig struct {
	MongoDBEnv               string   `yaml:"mongodb_env" bson:"mongodb_env" json:"mongodb_env"`
	MongoDBOwner             string   `yaml:"mongodb_owner" bson:"mongodb_owner" json:"mongodb_owner"`
	MongoDBOwnerEmailDomains []string `yaml:"mongodb_owner_email_domains" bson:"mongodb_owner_email_domains" json:"mongodb_owner_email_domains"`
}

// OwnerEmail returns the configured fallback owner, or Evergreen's address when unset.
func (c ResourceTagsConfig) OwnerEmail() string {
	if c.MongoDBOwner == "" {
		return DefaultMongoDBOwner
	}
	return c.MongoDBOwner
}

// OwnerEmailDomains returns the configured domains, or defaults for older settings.
func (c ResourceTagsConfig) OwnerEmailDomains() []string {
	if c.MongoDBOwnerEmailDomains == nil {
		return []string{"mongodb.com", "10gen.com"}
	}
	return slices.Clone(c.MongoDBOwnerEmailDomains)
}

func (c *ResourceTagsConfig) Validate() error {
	domains := c.OwnerEmailDomains()
	if len(domains) == 0 {
		return errors.New("MongoDB owner email domains must not be empty")
	}
	seen := map[string]bool{}
	for _, domain := range domains {
		if len(domain) > 253 || !mongoDBOwnerEmailDomainPattern.MatchString(domain) {
			return errors.Errorf("invalid MongoDB owner email domain '%s': use a lowercase domain without @ or wildcards", domain)
		}
		if seen[domain] {
			return errors.Errorf("duplicate MongoDB owner email domain '%s'", domain)
		}
		seen[domain] = true
	}

	if c.MongoDBEnv != "" {
		if _, ok := validMongoDBEnvironments[c.MongoDBEnv]; !ok {
			return errors.Errorf("invalid MongoDB environment '%s'", c.MongoDBEnv)
		}
	}
	if c.MongoDBOwner != "" {
		if err := c.ValidateOwnerEmail(c.MongoDBOwner); err != nil {
			return errors.Wrap(err, "validating MongoDB owner email")
		}
	}
	return nil
}

// ValidateAndDefault validates resource tags and fills in domains for older settings.
func (c *ResourceTagsConfig) ValidateAndDefault() error {
	if err := c.Validate(); err != nil {
		return err
	}
	c.MongoDBOwnerEmailDomains = c.OwnerEmailDomains()
	return nil
}

// ValidateOwnerEmail checks that an email is a bare address on an allowed domain.
func (c ResourceTagsConfig) ValidateOwnerEmail(email string) error {
	domains := c.OwnerEmailDomains()
	parsed, err := mail.ParseAddress(email)
	if err == nil && parsed.Address == email {
		domain := email[strings.LastIndex(email, "@")+1:]
		if slices.Contains(domains, domain) {
			return nil
		}
	}
	return errors.Errorf("email must be a valid address on an allowed domain: %s", strings.Join(domains, ", "))
}
