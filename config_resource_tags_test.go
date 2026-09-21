package evergreen

import "testing"

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
	} {
		t.Run(name, func(t *testing.T) {
			err := test.config.Validate()
			if test.valid && err != nil {
				t.Fatalf("expected valid config, got %s", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected invalid config")
			}
		})
	}
}
