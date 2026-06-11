package ratelimit

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/go-redis/redis_rate/v9"
	"github.com/pkg/errors"
)

type Limiter struct {
	limiter *redis_rate.Limiter
}

// Allow is used for REST and GraphQL rate limits, which have a cost of 1.
func (l *Limiter) Allow(ctx context.Context, userID string, surface evergreen.RateLimitSurface, reqPerHour int, burst int) (*redis_rate.Result, error) {
	return l.AllowN(ctx, userID, surface, reqPerHour, burst, 1)
}

func (l *Limiter) AllowN(ctx context.Context, userID string, surface evergreen.RateLimitSurface, reqPerHour int, burst int, n int) (*redis_rate.Result, error) {
	// AllowN is used for complexity queries
	switch surface {
	case evergreen.RateLimitSurfaceREST, evergreen.RateLimitSurfaceGraphQL, evergreen.RateLimitSurfaceComplexity:
	default:
		return nil, errors.Errorf("invalid rate limit surface '%s'", surface)
	}
	if burst > reqPerHour {
		return nil, errors.Errorf("burst limit '%d' cannot be greater than the per hour limit '%d'", burst, reqPerHour)
	}
	key := fmt.Sprintf("evergreen:ratelimit:%s:%s", userID, surface)
	limit := redis_rate.PerHour(reqPerHour)
	limit.Burst = burst // Override default burst, which is equal to hourly limit
	return l.limiter.AllowN(ctx, key, limit, n)
}
