package ratelimit

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/evergreen-ci/evergreen"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRedisTestLimiter returns a Limiter backed by a mock Redis (miniredis)
// for tests that exercise the Redis-backed code path.
func newRedisTestLimiter(t *testing.T) *Limiter {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { assert.NoError(t, rdb.Close()) })
	l, err := NewRateLimiter(rdb)
	require.NoError(t, err)
	return l
}

func TestNewRateLimiterNilClientShouldError(t *testing.T) {
	l, err := NewRateLimiter(nil)
	assert.ErrorContains(t, err, "redis client")
	assert.Nil(t, l)
}

func TestAllowSurfaceOutsideTypeShouldError(t *testing.T) {
	l := newRedisTestLimiter(t)
	res, err := l.Allow(t.Context(), "user", evergreen.RateLimitSurface("bogus"), 100, 10)
	assert.ErrorContains(t, err, "surface")
	assert.Nil(t, res)
}

func TestAllowBurstGreaterThanPerHourShouldError(t *testing.T) {
	l := newRedisTestLimiter(t)
	res, err := l.Allow(t.Context(), "user", evergreen.RateLimitSurfaceREST, 100, 200)
	assert.ErrorContains(t, err, "burst")
	assert.Nil(t, res)
}

func TestAllowValidInputShouldPass(t *testing.T) {
	l := newRedisTestLimiter(t)
	res, err := l.Allow(t.Context(), "user", evergreen.RateLimitSurfaceREST, 100, 10)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestAllowNDebitsCostFromBurst(t *testing.T) {
	l := newRedisTestLimiter(t)
	ctx := t.Context()

	// A complexity-10 query consumes the entire burst of 10.
	res, err := l.AllowN(ctx, "user", evergreen.RateLimitSurfaceComplexity, 100, 10, 10)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 10, res.Allowed)

	// With the burst exhausted, even a cost-1 query is denied.
	res, err = l.AllowN(ctx, "user", evergreen.RateLimitSurfaceComplexity, 100, 10, 1)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 0, res.Allowed)
}

func TestAllowNCostExceedingBurstShouldDeny(t *testing.T) {
	l := newRedisTestLimiter(t)

	// A single query costing more than the burst can never be allowed.
	res, err := l.AllowN(t.Context(), "user", evergreen.RateLimitSurfaceComplexity, 100, 10, 20)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 0, res.Allowed)
}

func TestAllowNCostLessThanOneShouldError(t *testing.T) {
	l := newRedisTestLimiter(t)
	res, err := l.AllowN(t.Context(), "user", evergreen.RateLimitSurfaceComplexity, 100, 10, 0)
	assert.ErrorContains(t, err, "cost")
	assert.Nil(t, res)
}

func TestAllowNReqPerHourLessThanOneShouldError(t *testing.T) {
	l := newRedisTestLimiter(t)
	res, err := l.AllowN(t.Context(), "user", evergreen.RateLimitSurfaceREST, 0, 0, 1)
	assert.ErrorContains(t, err, "per hour")
	assert.Nil(t, res)
}

func TestAllowNBurstLessThanOneShouldError(t *testing.T) {
	l := newRedisTestLimiter(t)
	res, err := l.AllowN(t.Context(), "user", evergreen.RateLimitSurfaceREST, 100, 0, 1)
	assert.ErrorContains(t, err, "burst")
	assert.Nil(t, res)
}

func TestAllowExhaustingOneUserDoesNotAffectAnother(t *testing.T) {
	l := newRedisTestLimiter(t)
	ctx := t.Context()

	res, err := l.AllowN(ctx, "user1", evergreen.RateLimitSurfaceREST, 100, 10, 10)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 10, res.Allowed)

	// user1's bucket is exhausted, but user2 has its own bucket.
	res, err = l.Allow(ctx, "user2", evergreen.RateLimitSurfaceREST, 100, 10)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 1, res.Allowed)
}

func TestAllowExhaustingOneSurfaceDoesNotAffectAnother(t *testing.T) {
	l := newRedisTestLimiter(t)
	ctx := t.Context()

	res, err := l.AllowN(ctx, "user", evergreen.RateLimitSurfaceComplexity, 100, 10, 10)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 10, res.Allowed)

	// The complexity bucket is exhausted, but the same user's REST bucket is independent.
	res, err = l.Allow(ctx, "user", evergreen.RateLimitSurfaceREST, 100, 10)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 1, res.Allowed)
}
