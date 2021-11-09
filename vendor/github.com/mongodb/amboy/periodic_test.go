package amboy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWaitUntil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const interval = 5 * time.Second

	t.Run("PastStartAt", func(t *testing.T) {
		tsa := time.Now().Round(time.Second)
		waitUntilInterval(ctx, time.Now().Round(time.Second).Add(-interval), interval)
		assert.WithinDuration(t, tsa.Add(interval), time.Now(), interval)
		assert.True(t, tsa.Before(time.Now()))
	})
	t.Run("FutureStartAt", func(t *testing.T) {
		tsa := time.Now().Round(time.Second)
		waitUntilInterval(ctx, time.Now().Round(time.Second).Add(interval), interval)
		assert.WithinDuration(t, tsa.Add(interval), time.Now(), interval)
		assert.True(t, tsa.Before(time.Now()))
	})
	t.Run("Cancelable", func(t *testing.T) {
		ctx, cancel = context.WithCancel(ctx)
		cancel()
		tsa := time.Now().Round(time.Second)
		waitUntilInterval(ctx, time.Now().Round(time.Second).Add(interval), interval)
		assert.Equal(t, time.Now().Round(time.Second), tsa)
	})
	t.Run("DuplicateJobError", func(t *testing.T) {
		t.Run("WithReportingDisabled", func(t *testing.T) {
			err := scheduleOp(ctx, nil, func(_ context.Context, q Queue) error {
				return NewDuplicateJobError("err")
			}, QueueOperationConfig{})
			assert.NoError(t, err)
		})
		t.Run("WithReportingEnabled", func(t *testing.T) {
			err := scheduleOp(ctx, nil, func(_ context.Context, q Queue) error {
				return NewDuplicateJobError("err")
			}, QueueOperationConfig{EnableDuplicateJobReporting: true})
			assert.Error(t, err)
			assert.True(t, IsDuplicateJobError(err))
		})
	})
}
