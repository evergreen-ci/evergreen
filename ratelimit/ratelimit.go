package ratelimit

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/go-redis/redis_rate/v10"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	limiter *redis_rate.Limiter
}

func NewRateLimiter(rdb *redis.Client) *Limiter {
	return &Limiter{limiter: redis_rate.NewLimiter(rdb)}
}

// Allow is used for REST and GraphQL rate limits, which have a cost of 1.
func (l *Limiter) Allow(ctx context.Context, userID string, surface evergreen.RateLimitSurface, reqPerHour int, burst int) (*redis_rate.Result, error) {
	return l.AllowN(ctx, userID, surface, reqPerHour, burst, 1)
}

// AllowN is used for GraphQL queries, which have a cost equal to their complexity score.
func (l *Limiter) AllowN(ctx context.Context, userID string, surface evergreen.RateLimitSurface, reqPerHour int, burst int, n int) (*redis_rate.Result, error) {
	switch surface {
	case evergreen.RateLimitSurfaceREST, evergreen.RateLimitSurfaceGraphQL, evergreen.RateLimitSurfaceComplexity:
	default:
		return nil, errors.Errorf("invalid rate limit surface '%s'", surface)
	}
	// Callers should not pass in reqPerHour/burst = 0, the middleware should treat 0 as unlimited and skip calling the limiter entirely.
	if reqPerHour < 1 {
		return nil, errors.Errorf("per hour limit %d must be at least 1", reqPerHour)
	}
	if burst < 1 {
		return nil, errors.Errorf("burst limit %d must be at least 1", burst)
	}
	if burst > reqPerHour {
		return nil, errors.Errorf("burst limit %d cannot be greater than the per hour limit %d", burst, reqPerHour)
	}
	if n < 1 {
		return nil, errors.Errorf("cost %d must be at least 1", n)
	}
	const format = "evergreen:ratelimit:%s:%s"
	key := fmt.Sprintf(format, userID, surface)
	limit := redis_rate.PerHour(reqPerHour)
	limit.Burst = burst // Override default burst, which is equal to hourly limit
	return l.limiter.AllowN(ctx, key, limit, n)
}
