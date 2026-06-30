package model

import (
	"context"
	"sync/atomic"
)

var (
	translateLimit     atomic.Int64
	translateInUse     atomic.Int64
	translateSlotFreed = make(chan struct{}, 1)
)

func noopRelease() {}

// SetTranslateConcurrencyLimit caps the number of concurrent TranslateProject calls at n. By
// convention, n <= 0 means unlimited, which is also the default. Safe to call at any time,
// including while callers are already waiting for a slot: a waiter re-checks the current limit
// whenever it changes instead of blocking against whatever value was in effect when it started
// waiting.
func SetTranslateConcurrencyLimit(n int) {
	translateLimit.Store(int64(n))
	wakeTranslateWaiters()
}

func wakeTranslateWaiters() {
	select {
	case translateSlotFreed <- struct{}{}:
	default:
	}
}

// acquireTranslateSlot blocks until a concurrency slot is available, ctx is done, or no limit is
// set (in which case it returns immediately). Callers must defer the returned release func.
func acquireTranslateSlot(ctx context.Context) (func(), error) {
	for {
		limit := translateLimit.Load()
		if limit <= 0 {
			return noopRelease, nil
		}

		if translateInUse.Add(1) <= limit {
			return func() {
				translateInUse.Add(-1)
				wakeTranslateWaiters()
			}, nil
		}
		translateInUse.Add(-1)

		select {
		case <-translateSlotFreed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
