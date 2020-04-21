package util

import (
	"context"
	"math"
	"math/rand"
	"net/http"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/jpillora/backoff"
	"github.com/pkg/errors"
)

func init() {
	rand.Seed(time.Now().Unix())
}

// RetriableFunc is any function that takes no parameters and returns only
// an error interface. These functions can be used with util.Retry.
type RetriableFunc func() (bool, error)

func getBackoff(numAttempts int, min time.Duration, max time.Duration) *backoff.Backoff {
	if min < 100*time.Millisecond {
		min = 100 * time.Millisecond
	}

	if numAttempts == 0 {
		numAttempts = 1
	}

	var factor float64 = 2
	if max == 0 {
		max = time.Duration(float64(min) * math.Pow(factor, float64(numAttempts)))
	}

	return &backoff.Backoff{
		Min: min,
		// the maximum value is uncapped. could change this
		// value so that we didn't avoid very long sleeps in
		// potential worst cases.
		Max:    max,
		Factor: factor,
		Jitter: true,
	}
}

// Retry provides a mechanism to retry an operation with exponential backoff with jitter. Specify
// minimum duration, maximum duration, and maximum number of retries.
//
// It will set min to 100ms if not set.
// It will set max to (min * 2^attempts) if not set.
// It will set attempts to 1 if not set.
func Retry(ctx context.Context, op RetriableFunc, attempts int, min time.Duration, max time.Duration) error {
	if attempts < 1 {
		attempts = 1
	}
	attempt := 0
	backoff := getBackoff(attempts, min, max)
	timer := time.NewTimer(0)
	for {
		select {
		case <-ctx.Done():
			return errors.Errorf("context canceled after %d retries", attempt)
		case <-timer.C:
			shouldRetry, err := op()
			if err == nil {
				return nil
			}
			if shouldRetry {
				attempt++
				if attempt == attempts {
					return errors.Wrapf(err, "after %d retries, operation failed", attempts)
				}
				timer.Reset(backoff.Duration())
			} else {
				return err
			}
		}
	}
}

// RetryArgs defines the policy for retrying an operation.
type RetryArgs struct {
	MaxAttempts int
	MinDelay    time.Duration
	MaxDelay    time.Duration
}

// RetryWithArgs is a convenience wrapper around Retry that passes the
// parameters from RetryArgs to Retry.
func RetryWithArgs(ctx context.Context, op RetriableFunc, args RetryArgs) error {
	return Retry(ctx, op, args.MaxAttempts, args.MinDelay, args.MaxDelay)
}

func RehttpDelay(initialSleep time.Duration, numAttempts int) utility.HTTPDelayFunction {
	backoff := getBackoff(numAttempts, initialSleep, 0)
	return func(index int, req *http.Request, resp *http.Response, err error) time.Duration {
		return backoff.ForAttempt(float64(index))
	}
}
