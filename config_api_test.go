package evergreen

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimitConfigValidate(t *testing.T) {
	t.Run("ZeroValuesAreValid", func(t *testing.T) {
		c := RateLimitConfig{}
		assert.NoError(t, c.validate())
	})

	t.Run("ValidPositiveValues", func(t *testing.T) {
		c := RateLimitConfig{
			RESTHumanRequestsPerHour:      1000,
			RESTHumanBurst:                50,
			RESTServiceRequestsPerHour:    5000,
			RESTServiceBurst:              200,
			GraphQLHumanRequestsPerHour:   500,
			GraphQLHumanBurst:             25,
			GraphQLServiceRequestsPerHour: 2000,
			GraphQLServiceBurst:           100,
			GraphQLComplexityLimit:        1000,
			ElevatedUserIDs:               []string{"alice", "bob"},
		}
		assert.NoError(t, c.validate())
	})

	t.Run("NegativePerHourShouldError", func(t *testing.T) {
		c := RateLimitConfig{RESTHumanRequestsPerHour: -1}
		assert.Error(t, c.validate())
	})

	t.Run("NegativeBurstShouldError", func(t *testing.T) {
		c := RateLimitConfig{RESTHumanBurst: -1}
		assert.Error(t, c.validate())
	})

	t.Run("BurstExceedsPerHourShouldError", func(t *testing.T) {
		c := RateLimitConfig{RESTHumanRequestsPerHour: 100, RESTHumanBurst: 200}
		assert.Error(t, c.validate())
	})

	t.Run("BurstEqualsPerHourIsValid", func(t *testing.T) {
		c := RateLimitConfig{RESTHumanRequestsPerHour: 100, RESTHumanBurst: 100}
		assert.NoError(t, c.validate())
	})

	t.Run("NegativeComplexityLimitShouldError", func(t *testing.T) {
		c := RateLimitConfig{GraphQLComplexityLimit: -1}
		assert.Error(t, c.validate())
	})

	t.Run("MultipleErrorsAreCaptured", func(t *testing.T) {
		c := RateLimitConfig{
			RESTHumanRequestsPerHour:    -1,
			GraphQLHumanRequestsPerHour: -1,
			GraphQLComplexityLimit:      -1,
		}
		err := c.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "REST human")
		assert.Contains(t, err.Error(), "GraphQL human")
		assert.Contains(t, err.Error(), "GraphQL complexity limit")
	})
}

func TestAPIConfigRateLimitBSONRoundTrip(t *testing.T) {
	t.Run("AllFieldsRoundTrip", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(t.Context()))
		t.Cleanup(func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(context.Background()))
		})

		original := APIConfig{
			URL: "https://example.com",
			RateLimit: RateLimitConfig{
				RESTHumanRequestsPerHour:      1000,
				RESTHumanBurst:                50,
				RESTServiceRequestsPerHour:    5000,
				RESTServiceBurst:              200,
				GraphQLHumanRequestsPerHour:   500,
				GraphQLHumanBurst:             25,
				GraphQLServiceRequestsPerHour: 2000,
				GraphQLServiceBurst:           100,
				GraphQLComplexityLimit:        1000,
				ElevatedUserIDs:               []string{"alice", "bob"},
			},
		}
		require.NoError(t, original.Set(t.Context()))

		retrieved := APIConfig{}
		require.NoError(t, retrieved.Get(t.Context()))

		assert.Equal(t, original.RateLimit, retrieved.RateLimit)
	})

	t.Run("ZeroValuesRoundTrip", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(t.Context()))
		t.Cleanup(func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(context.Background()))
		})

		original := APIConfig{
			URL:       "https://example.com",
			RateLimit: RateLimitConfig{},
		}
		require.NoError(t, original.Set(t.Context()))

		retrieved := APIConfig{}
		require.NoError(t, retrieved.Get(t.Context()))

		assert.Equal(t, RateLimitConfig{}, retrieved.RateLimit)
	})
}
