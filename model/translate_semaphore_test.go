package model

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTranslateConcurrencyLimitCapsConcurrentAcquires(t *testing.T) {
	t.Cleanup(func() { SetTranslateConcurrencyLimit(0) })

	const limit = 3
	const numGoroutines = 20
	SetTranslateConcurrencyLimit(limit)

	var current, observedMax atomic.Int64
	done := make(chan struct{})
	for range numGoroutines {
		go func() {
			defer func() { done <- struct{}{} }()
			release, err := acquireTranslateSlot(t.Context())
			require.NoError(t, err)
			defer release()

			n := current.Add(1)
			for {
				max := observedMax.Load()
				if n <= max || observedMax.CompareAndSwap(max, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			current.Add(-1)
		}()
	}
	for range numGoroutines {
		<-done
	}

	require.LessOrEqual(t, observedMax.Load(), int64(limit))
}

func TestTranslateConcurrencyLimitUnsetNeverBlocks(t *testing.T) {
	t.Cleanup(func() { SetTranslateConcurrencyLimit(0) })

	SetTranslateConcurrencyLimit(0)

	done := make(chan struct{})
	go func() {
		release, err := acquireTranslateSlot(t.Context())
		require.NoError(t, err)
		defer release()
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("acquireTranslateSlot blocked with no limit set")
	}
}

func TestAcquireTranslateSlotContextDoneReturnsError(t *testing.T) {
	t.Cleanup(func() { SetTranslateConcurrencyLimit(0) })
	SetTranslateConcurrencyLimit(1)

	release, err := acquireTranslateSlot(t.Context())
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = acquireTranslateSlot(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSetTranslateConcurrencyLimitIncreaseWakesBlockedWaiters(t *testing.T) {
	t.Cleanup(func() { SetTranslateConcurrencyLimit(0) })
	SetTranslateConcurrencyLimit(1)

	release, err := acquireTranslateSlot(t.Context())
	require.NoError(t, err)

	waiterAcquired := make(chan func())
	go func() {
		r, err := acquireTranslateSlot(t.Context())
		require.NoError(t, err)
		waiterAcquired <- r
	}()

	// Give the goroutine a chance to block against the limit of 1, which is already held above.
	time.Sleep(50 * time.Millisecond)

	// Raising the limit should wake the blocked waiter immediately rather than stranding it until
	// the original holder releases.
	SetTranslateConcurrencyLimit(2)

	select {
	case r := <-waiterAcquired:
		r()
	case <-time.After(time.Second):
		t.Fatal("acquireTranslateSlot did not wake a blocked waiter after the limit was increased")
	}

	release()
}
