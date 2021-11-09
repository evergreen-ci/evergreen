package utility

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetry(t *testing.T) {
	const maxAttempts = 5
	const minDelay = 10 * time.Millisecond

	t.Run("ErrorsWithOperationThatNeverSucceeds", func(t *testing.T) {
		failingFunc := func() (bool, error) {
			return true, errors.New("something went wrong")
		}

		start := time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := Retry(ctx, failingFunc, RetryOptions{MaxAttempts: maxAttempts, MinDelay: minDelay})
		end := time.Now()
		require.Error(t, err, "retrying an operation that never succeeds should error")
		assert.True(t, end.After(start.Add((maxAttempts-1)*minDelay)))
	})
	t.Run("SucceedsWithOperationThatSucceedsAfterSomeAttempts", func(t *testing.T) {
		const triesTillPass = 2

		tryCounter := triesTillPass
		retryPassingFunc := func() (bool, error) {
			tryCounter--
			if tryCounter <= 0 {
				return false, nil
			}
			return true, errors.New("something went wrong")
		}

		opts := RetryOptions{
			MaxAttempts: maxAttempts,
			MinDelay:    minDelay,
		}
		opts.Validate()
		backoff := getBackoff(opts)

		start := time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := Retry(ctx, retryPassingFunc, opts)
		end := time.Now()
		require.NoError(t, err, "operation should have succeeded after some failed attempts")

		assert.True(t, end.After(start.Add((triesTillPass-1)*minDelay)))
		assert.True(t, end.Before(start.Add(backoff.Max)))
	})
	t.Run("NonRetryableErrorFails", func(t *testing.T) {
		failingFuncNoRetry := func() (bool, error) {
			return false, errors.New("something went wrong")
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := Retry(ctx, failingFuncNoRetry, RetryOptions{MaxAttempts: maxAttempts, MinDelay: minDelay})
		assert.Error(t, err)
	})
	t.Run("DoesNotDelayFirstAttempt", func(t *testing.T) {
		now := time.Now()
		noop := func() (bool, error) {
			return false, nil
		}
		require.NoError(t, Retry(context.Background(), noop, RetryOptions{MaxAttempts: 10, MinDelay: time.Second, MaxDelay: time.Second}))
		require.True(t, time.Since(now) < time.Millisecond)
	})
}
