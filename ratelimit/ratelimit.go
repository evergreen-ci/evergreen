package ratelimit

import (
	"context"
	"fmt"
	"slices"

	"github.com/evergreen-ci/evergreen"
	"github.com/go-redis/redis_rate/v10"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	limiter *redis_rate.Limiter
}

func NewRateLimiter(rdb *redis.Client) (*Limiter, error) {
	if rdb == nil {
		return nil, errors.New("redis client must not be nil")
	}
	return &Limiter{limiter: redis_rate.NewLimiter(rdb)}, nil
}

// Allow reports whether a request with the given userID and surface is allowed under the specified rate and burst limits.
// It is used for REST and GraphQL rate limits, which have a cost of 1.
func (l *Limiter) Allow(ctx context.Context, userID string, surface evergreen.RateLimitSurface, reqPerHour int, burst int) (*redis_rate.Result, error) {
	return l.AllowN(ctx, userID, surface, reqPerHour, burst, 1)
}

// AllowN reports whether a request with the given userID, surface, and cost is allowed under the specified rate and burst limits.
func (l *Limiter) AllowN(ctx context.Context, userID string, surface evergreen.RateLimitSurface, reqPerHour int, burst int, n int) (*redis_rate.Result, error) {
	if !slices.Contains(evergreen.ValidRateLimitSurfaces, surface) {
		return nil, errors.Errorf("invalid rate limit surface '%s'", surface)
	}
	if n < 1 {
		return nil, errors.Errorf("cost %d must be at least 1", n)
	}
	// Skip limiting if limits are not set by returning a nil Result.
	if reqPerHour == 0 { // Burst is guaranteed to also be 0 by validation in config.
		return nil, nil
	}
	const format = "evergreen:ratelimit:%s:%s"
	key := fmt.Sprintf(format, userID, surface)
	limit := redis_rate.PerHour(reqPerHour)
	limit.Burst = burst // Override default burst, which is equal to hourly limit
	return l.limiter.AllowN(ctx, key, limit, n)
}
