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

// RetryWithContext provides the same semantics as Retry, but will not retry again if the context is canceled.
func RetryWithContext(ctx context.Context, op RetriableFunc, attempts int, sleep time.Duration) (bool, error) {
	if ctx.Err() != nil {
		return false, errors.New("context canceled")
	}
	backoff := getBackoff(sleep, attempts)
	for i := attempts; i >= 0; i-- {
		shouldRetry, err := op()

		if err == nil {
			//the attempt succeeded, so we return no error
			return false, nil
		}

		if shouldRetry {
			if i == 0 {
				// used up all retry attempts, so return the failure.
				return true, errors.Wrapf(err, "after %d retries, operation failed", attempts)
			}
			if ctx.Err() != nil {
				return false, errors.Errorf("context canceled after %d retries", attempts-i+1)
			}
			time.Sleep(backoff.Duration())
			if ctx.Err() != nil {
				return false, errors.Errorf("context canceled after %d retries", attempts-i+1)
			}
		} else {
			return false, err
		}
	}

	return false, errors.New("unable to complete retry operation")
}

// Retry provides a mechanism to retry an operation with exponential
// backoff (that uses some jitter,) Specify the maximum number of
// retry attempts that you want to permit as well as the initial
// period that you want to sleep between attempts.
//
// Retry requires that the starting sleep interval be at least 100
// milliseconds, and forces this interval if you attempt to use a
// shorter period.
//
// If you specify 0 attempts, Retry will use an attempt value of one.
func Retry(op RetriableFunc, attempts int, sleep time.Duration) (bool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return RetryWithContext(ctx, op, attempts, sleep)
}

func RehttpDelay(initialSleep time.Duration, numAttempts int) rehttp.DelayFn {
	backoff := getBackoff(initialSleep, numAttempts)
	return func(attempt rehttp.Attempt) time.Duration {
		return backoff.ForAttempt(float64(attempt.Index))
	}
}
