package model

import (
	"github.com/go-redis/redis_rate/v10"
)

type APIRateLimitStatus struct {
	Limit             int `json:"hourly_limit"`
	Burst             int `json:"burst_limit"`
	Remaining         int `json:"remaining"`
	ResetAfterSeconds int `json:"reset_after_seconds"`
}

// BuildFromService converts a redis_rate.Result into an APIRateLimitStatus.
func (s *APIRateLimitStatus) BuildFromService(result *redis_rate.Result) {
	if result == nil {
		return
	}
	s.Limit = result.Limit.Rate
	s.Burst = result.Limit.Burst
	s.Remaining = result.Remaining
	s.ResetAfterSeconds = int(result.ResetAfter.Seconds())
}
