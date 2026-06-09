package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	limiter *redis_rate.Limiter
}

func EvergreenLimiter(client *redis.Client) *Limiter {
	return &Limiter{limiter: redis_rate.EvergreenLimiter(client)}
}

func (l *Limiter) Allow(ctx context.Context, surface, userID string, rph, burst int) (*redis_rate.Result, error) {
	key := fmt.Sprintf("evergreen:ratelimit:%s:%s", surface, userID)
	limit := redis_rate.Limit{Rate: rph, Period: time.Hour, Burst: burst}
	return l.limiter.Allow(ctx, key, limit)
}
