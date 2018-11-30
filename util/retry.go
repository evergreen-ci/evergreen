package util

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/jpillora/backoff"
	"github.com/pkg/errors"
)

func init() {
	rand.Seed(time.Now().Unix())
}

// RetriableFunc is any function that takes no parameters and returns only
// an error interface. These functions can be used with util.Retry.
type RetriableFunc func() (bool, error)

func getBackoff(initialSleep time.Duration, numAttempts int) *backoff.Backoff {
	if initialSleep < 100*time.Millisecond {
		initialSleep = 100 * time.Millisecond
	}

	if numAttempts == 0 {
		numAttempts = 1
	}

	var factor float64 = 2

	return &backoff.Backoff{
		Min: initialSleep,
		// the maximum value is uncapped. could change this
		// value so that we didn't avoid very long sleeps in
		// potential worst cases.
		Max:    time.Duration(float64(initialSleep) * math.Pow(factor, float64(numAttempts))),
		Factor: factor,
		Jitter: true,
	}
}

// Retry provides a mechanism to retry an operation with exponential
// backoff (that uses some jitter) Specify the maximum number of
// retry attempts that you want to permit as well as the initial
// period that you want to sleep between attempts.
//
// Retry requires that the starting sleep interval be at least 100
// milliseconds, and forces this interval if you attempt to use a
// shorter period.
//
// If you specify less than 0 attempts, Retry will use an attempt value of 1.
func Retry(ctx context.Context, op RetriableFunc, attempts int, sleep time.Duration) (bool, error) {
	if attempts < 1 {
		attempts = 1
	}
	attempt := 0
	backoff := getBackoff(sleep, attempts)
	timer := time.NewTimer(backoff.Duration())
	for {
		select {
		case <-ctx.Done():
			return false, errors.Errorf("context canceled after %d retries", attempt)
		case <-timer.C:
			shouldRetry, err := op()
			if err == nil {
				return false, nil
			}
			if shouldRetry {
				attempt++
				if attempt == attempts {
					return true, errors.Wrapf(err, "after %d retries, operation failed", attempts)
				}
				timer.Reset(backoff.Duration())
			} else {
				return false, err
			}
		}
	}
}

func RehttpDelay(initialSleep time.Duration, numAttempts int) rehttp.DelayFn {
	backoff := getBackoff(initialSleep, numAttempts)
	return func(attempt rehttp.Attempt) time.Duration {
		return backoff.ForAttempt(float64(attempt.Index))
	}
}
