package route

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
)

func TestGetURL(t *testing.T) {
	env := evergreen.GetEnvironment()
	settings := env.Settings()
	originalCorpURL := settings.Api.CorpURL
	originalUIURL := settings.Ui.Url
	t.Cleanup(func() {
		settings.Api.CorpURL = originalCorpURL
		settings.Ui.Url = originalUIURL
	})

	settings.Api.CorpURL = "https://evergreen.corp.mongodb.com"
	settings.Ui.Url = "https://evergreen.mongodb.com"

	t.Run("CorpHostShouldReturnCorpURL", func(t *testing.T) {
		ctx := withRequestHost(t.Context(), "evergreen.corp.mongodb.com")
		assert.Equal(t, "https://evergreen.corp.mongodb.com", GetURL(ctx))
	})

	t.Run("NonCorpHostShouldReturnNonCorpURL", func(t *testing.T) {
		ctx := withRequestHost(t.Context(), "evergreen.mongodb.com")
		assert.Equal(t, "https://evergreen.mongodb.com", GetURL(ctx))
	})

	t.Run("MissingHostShouldReturnNonCorpURL", func(t *testing.T) {
		assert.Equal(t, "https://evergreen.mongodb.com", GetURL(t.Context()))
	})

	t.Run("CorpHostWithPortShouldReturnCorpURL", func(t *testing.T) {
		ctx := withRequestHost(t.Context(), "evergreen.corp.mongodb.com:443")
		assert.Equal(t, "https://evergreen.corp.mongodb.com", GetURL(ctx))
	})
}
