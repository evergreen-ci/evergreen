package ratelimit

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/evergreen-ci/evergreen"
	"github.com/go-redis/redis/v8"
	"github.com/go-redis/redis_rate/v9"
	"github.com/stretchr/testify/assert"
)

// newTestLimiter returns a Limiter whose backing Redis client is nil. These
// tests only exercise validation that happens before Redis is contacted.
func newTestLimiter() *Limiter {
	return &Limiter{limiter: redis_rate.NewLimiter(nil)}
}

// newRedisTestLimiter returns a Limiter backed by an in-memory Redis (miniredis)
// for tests that exercise the Redis-backed code path.
func newRedisTestLimiter(t *testing.T) *Limiter {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { assert.NoError(t, rdb.Close()) })
	return &Limiter{limiter: redis_rate.NewLimiter(rdb)}
}

func TestAllowSurfaceOutsideTypeShouldError(t *testing.T) {
	l := newTestLimiter()
	res, err := l.Allow(t.Context(), "user", evergreen.RateLimitSurface("bogus"), 100, 10)
	assert.ErrorContains(t, err, "surface")
	assert.Nil(t, res)
}

func TestAllowBurstGreaterThanPerHourShouldError(t *testing.T) {
	l := newTestLimiter()
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
