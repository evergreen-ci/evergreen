package ratelimit

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/go-redis/redis_rate/v9"
)

type Limiter struct {
	limiter *redis_rate.Limiter
}

func (l *Limiter) Allow(ctx context.Context, userID string, surface evergreen.RateLimitSurface, reqPerHour int, burst int) (*redis_rate.Result, error) {
	key := fmt.Sprintf("evergreen:ratelimit:%s:%s", userID, surface)
	limit := redis_rate.PerHour(reqPerHour)
	limit.Burst = burst // Override default burst, which is equal to hourly limit
	return l.limiter.Allow(ctx, key, limit)
}
