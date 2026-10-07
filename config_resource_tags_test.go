package evergreen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceTagsConfigValidate(t *testing.T) {
	for name, test := range map[string]struct {
		config ResourceTagsConfig
		valid  bool
	}{
		"AllowsUnsetConfig": {
			config: ResourceTagsConfig{},
			valid:  true,
		},
		"AllowsValidConfig": {
			config: ResourceTagsConfig{
				MongoDBEnv:   MongoDBEnvironmentStaging,
				MongoDBOwner: "evergreen@mongodb.com",
			},
			valid: true,
		},
		"AllowsEnvironmentWithoutOwner": {
			config: ResourceTagsConfig{
				MongoDBEnv: MongoDBEnvironmentStaging,
			},
			valid: true,
		},
		"AllowsOwnerWithoutEnvironment": {
			config: ResourceTagsConfig{
				MongoDBOwner: "evergreen@mongodb.com",
			},
			valid: true,
		},
		"RejectsUnsupportedEnvironment": {
			config: ResourceTagsConfig{
				MongoDBEnv:   "production",
				MongoDBOwner: "evergreen@mongodb.com",
			},
		},
		"RejectsInvalidEmail": {
			config: ResourceTagsConfig{
				MongoDBEnv:   MongoDBEnvironmentStaging,
				MongoDBOwner: "evergreen",
			},
		},
		"RejectsEmailWithoutDomainSuffix": {
			config: ResourceTagsConfig{
				MongoDBEnv:   MongoDBEnvironmentStaging,
				MongoDBOwner: "evergreen@mongodb",
			},
		},
		"RejectsExternalEmail": {
			config: ResourceTagsConfig{MongoDBOwner: "evergreen@example.com"},
		},
		"AllowsLegacyEmail": {
			config: ResourceTagsConfig{MongoDBOwner: "evergreen@10gen.com"},
			valid:  true,
		},
		"RejectsMongoDBOrgEmail": {
			config: ResourceTagsConfig{MongoDBOwner: "evergreen@mongodb.org"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := test.config.Validate()
			if test.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateMongoDBEmail(t *testing.T) {
	for name, testCase := range map[string]struct {
		email string
		valid bool
	}{
		"MongoDBAddressShouldPass":     {email: "user@mongodb.com", valid: true},
		"PlusAddressShouldPass":        {email: "user+evergreen@mongodb.com", valid: true},
		"EmptyAddressShouldFail":       {},
		"MissingLocalPartShouldFail":   {email: "@mongodb.com"},
		"MissingDomainShouldFail":      {email: "user"},
		"ExternalDomainShouldFail":     {email: "user@example.com"},
		"LegacyDomainShouldPass":       {email: "user@10gen.com", valid: true},
		"LegacySubdomainShouldFail":    {email: "user@sub.10gen.com"},
		"LegacyLookalikeShouldFail":    {email: "user@not10gen.com"},
		"MongoDBOrgDomainShouldFail":   {email: "user@mongodb.org"},
		"SubdomainShouldFail":          {email: "user@sub.mongodb.com"},
		"LookalikeDomainShouldFail":    {email: "user@notmongodb.com"},
		"DomainSuffixShouldFail":       {email: "user@mongodb.com.example.com"},
		"UppercaseDomainShouldFail":    {email: "user@MONGODB.COM"},
		"DisplayNameShouldFail":        {email: "User <user@mongodb.com>"},
		"LeadingWhitespaceShouldFail":  {email: " user@mongodb.com"},
		"TrailingWhitespaceShouldFail": {email: "user@mongodb.com "},
		"MultipleAddressesShouldFail":  {email: "user@mongodb.com, other@mongodb.com"},
		"MultipleAtSignsShouldFail":    {email: "user@@mongodb.com"},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateMongoDBEmail(testCase.email)
			if testCase.valid {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, "@mongodb.com")
			}
		})
	}
}
