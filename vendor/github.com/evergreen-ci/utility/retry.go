package utility

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/jpillora/backoff"
	"github.com/pkg/errors"
)

func init() {
	rand.Seed(time.Now().Unix())
}

// RetriableFunc is any function that takes no parameters and returns an error,
// and whether or not the operation can be retried. These functions can be used
// with util.Retry.
type RetriableFunc func() (canRetry bool, err error)

// Retry provides a mechanism to retry an operation with exponential backoff
// and jitter.
func Retry(ctx context.Context, op RetriableFunc, opts RetryOptions) error {
	backoff := getBackoff(opts)
	timer := time.NewTimer(0)
	defer timer.Stop()
	var attempt int

	for {
		select {
		case <-ctx.Done():
			return errors.Errorf("context canceled after %d attempts", attempt)
		case <-timer.C:
			shouldRetry, err := op()
			if err == nil {
				return nil
			}
			if !shouldRetry {
				return err
			}

			attempt++
			if attempt == opts.MaxAttempts {
				return errors.Wrapf(err, "after %d attempts, operation failed", opts.MaxAttempts)
			}
			timer.Reset(backoff.Duration())
		}
	}
}

// backoffFactor is the exponential backoff factor.
const backoffFactor = 2

func getBackoff(opts RetryOptions) *backoff.Backoff {
	opts.Validate()
	return &backoff.Backoff{
		Min:    opts.MinDelay,
		Max:    opts.MaxDelay,
		Factor: float64(backoffFactor),
		Jitter: true,
	}
}

// RetryOptions defines the policy for retrying an operation. This is typically
// used with retries that use exponential backoff.
type RetryOptions struct {
	// MaxAttempts is the total number of times the operation can be attempted.
	// By default, it is 1 (i.e. no retries).
	MaxAttempts int
	// MinDelay is the minimum delay between operation attempts. By default, it
	// is 100ms.
	MinDelay time.Duration
	// MaxDelay is the maximum delay between operation attempts. By default, it
	// is (MinDelay * 2^MaxAttempts).
	MaxDelay time.Duration
}

// Validate sets defaults for unspecified or invalid options.
//
// It will set min to 100ms if not set.
// It will set max to (min * 2^attempts) if not set.
// It will set attempts to 1 if not set.
func (o *RetryOptions) Validate() {
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 1
	}

	const floor = 100 * time.Millisecond
	if o.MinDelay < floor {
		o.MinDelay = floor
	}

	if o.MaxDelay <= 0 {
		o.MaxDelay = time.Duration(float64(o.MinDelay) * math.Pow(backoffFactor, float64(o.MaxAttempts)))
	}
}
