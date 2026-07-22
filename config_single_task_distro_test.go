package evergreen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSingleTaskDistroConfigValidateAndDefault(t *testing.T) {
	t.Run("ValidExactAndRegexProjectsPass", func(t *testing.T) {
		cfg := &SingleTaskDistroConfig{
			ProjectTasksPairs: []ProjectTasksPair{
				{ProjectID: "mongodb-mongo-master", AllowedTasks: []string{"all"}},
				{ProjectID: "mongodb-mongo-v.*", IsRegex: true, AllowedBVs: []string{"all"}},
			},
		}
		assert.NoError(t, cfg.ValidateAndDefault())
	})

	t.Run("InvalidRegexProjectFails", func(t *testing.T) {
		cfg := &SingleTaskDistroConfig{
			ProjectTasksPairs: []ProjectTasksPair{
				{ProjectID: "mongodb-mongo-v[", IsRegex: true, AllowedTasks: []string{"all"}},
			},
		}
		assert.Error(t, cfg.ValidateAndDefault())
	})

	t.Run("InvalidRegexSyntaxAllowedForExactMatch", func(t *testing.T) {
		cfg := &SingleTaskDistroConfig{
			ProjectTasksPairs: []ProjectTasksPair{
				{ProjectID: "mongodb-mongo-v[", AllowedTasks: []string{"all"}},
			},
		}
		assert.NoError(t, cfg.ValidateAndDefault())
	})

	t.Run("EmptyProjectIDFails", func(t *testing.T) {
		cfg := &SingleTaskDistroConfig{
			ProjectTasksPairs: []ProjectTasksPair{
				{ProjectID: "", AllowedTasks: []string{"all"}},
			},
		}
		assert.Error(t, cfg.ValidateAndDefault())
	})
}
