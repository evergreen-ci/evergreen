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
		assert.NoError(t, c.ValidateAndDefault())
	})

	t.Run("ValidPositiveValues", func(t *testing.T) {
		c := RateLimitConfig{
			RESTUserPerHour:        1000,
			RESTUserBurst:          50,
			RESTServicePerHour:     5000,
			RESTServiceBurst:       200,
			GraphQLUserPerHour:     500,
			GraphQLUserBurst:       25,
			GraphQLServicePerHour:  2000,
			GraphQLServiceBurst:    100,
			GraphQLComplexityLimit: 1000,
			ElevatedUserIDs:        []string{"alice", "bob"},
		}
		assert.NoError(t, c.ValidateAndDefault())
	})

	t.Run("NegativePerHourShouldError", func(t *testing.T) {
		c := RateLimitConfig{RESTUserPerHour: -1}
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("NegativeBurstShouldError", func(t *testing.T) {
		c := RateLimitConfig{RESTUserBurst: -1}
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("BurstExceedsPerHourShouldError", func(t *testing.T) {
		c := RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 200}
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("BurstEqualsPerHourIsValid", func(t *testing.T) {
		c := RateLimitConfig{RESTUserPerHour: 100, RESTUserBurst: 100}
		assert.NoError(t, c.ValidateAndDefault())
	})

	t.Run("NegativeComplexityLimitShouldError", func(t *testing.T) {
		c := RateLimitConfig{GraphQLComplexityLimit: -1}
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("MultipleErrorsAreCaptured", func(t *testing.T) {
		c := RateLimitConfig{
			RESTUserPerHour:        -1,
			GraphQLUserPerHour:     -1,
			GraphQLComplexityLimit: -1,
		}
		err := c.ValidateAndDefault()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "REST user")
		assert.Contains(t, err.Error(), "GraphQL user")
		assert.Contains(t, err.Error(), "GraphQL complexity limit")
	})
}

func TestRateLimitConfigBSONRoundTrip(t *testing.T) {
	t.Run("AllFieldsRoundTrip", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(t.Context()))
		t.Cleanup(func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(context.Background()))
		})

		original := RateLimitConfig{
			RESTUserPerHour:        1000,
			RESTUserBurst:          50,
			RESTServicePerHour:     5000,
			RESTServiceBurst:       200,
			GraphQLUserPerHour:     500,
			GraphQLUserBurst:       25,
			GraphQLServicePerHour:  2000,
			GraphQLServiceBurst:    100,
			GraphQLComplexityLimit: 1000,
			ElevatedUserIDs:        []string{"alice", "bob"},
		}
		require.NoError(t, original.Set(t.Context()))

		retrieved := RateLimitConfig{}
		require.NoError(t, retrieved.Get(t.Context()))

		assert.Equal(t, original, retrieved)
	})

	t.Run("ZeroValuesRoundTrip", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(t.Context()))
		t.Cleanup(func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(context.Background()))
		})

		original := RateLimitConfig{}
		require.NoError(t, original.Set(t.Context()))

		retrieved := RateLimitConfig{}
		require.NoError(t, retrieved.Get(t.Context()))

		assert.Equal(t, RateLimitConfig{}, retrieved)
	})
}
