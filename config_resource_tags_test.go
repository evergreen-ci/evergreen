package evergreen

import "testing"

func TestResourceTagsConfigValidateAndDefault(t *testing.T) {
	for name, test := range map[string]struct {
		config ResourceTagsConfig
		valid  bool
	}{
		"allows unset config": {
			config: ResourceTagsConfig{},
			valid:  true,
		},
		"allows valid config": {
			config: ResourceTagsConfig{
				MongoDBEnv:   "staging",
				MongoDBOwner: "evergreen@mongodb.com",
			},
			valid: true,
		},
		"rejects unsupported environment": {
			config: ResourceTagsConfig{
				MongoDBEnv:   "production",
				MongoDBOwner: "evergreen@mongodb.com",
			},
		},
		"rejects invalid email": {
			config: ResourceTagsConfig{
				MongoDBEnv:   "staging",
				MongoDBOwner: "evergreen",
			},
		},
		"rejects email without domain suffix": {
			config: ResourceTagsConfig{
				MongoDBEnv:   "staging",
				MongoDBOwner: "evergreen@mongodb",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := test.config.ValidateAndDefault()
			if test.valid && err != nil {
				t.Fatalf("expected valid config, got %s", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected invalid config")
			}
		})
	}
}
