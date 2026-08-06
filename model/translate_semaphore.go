package model

import (
	"context"
	"sync/atomic"
	"time"
)

var (
	translateLimit     atomic.Int64
	translateInUse     atomic.Int64
	translateSlotFreed = make(chan struct{}, 1)

	// translateWaiting is the number of callers parked waiting for a slot.
	translateWaiting atomic.Int64
	// translateBlockedTotal is the cumulative count of acquires that had to wait for a slot.
	translateBlockedTotal atomic.Int64
)

func releaseTranslateSlot() {
	translateInUse.Add(-1)
	wakeTranslateWaiters()
}

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
// set (in which case it returns immediately). Callers must defer the returned release func. waited
// reports how long the caller spent queued behind the limit, and is zero when a slot was claimed
// without waiting.
func acquireTranslateSlot(ctx context.Context) (release func(), waited time.Duration, err error) {
	// The clock only starts on the first park, so an uncontended acquire stays free of it.
	var waitStart time.Time
	for {
		limit := translateLimit.Load()
		// In-flight translations are counted even when unlimited, so raising the limit later accounts
		// for translations already running.
		if translateInUse.Add(1) <= limit || limit <= 0 {
			return releaseTranslateSlot, sinceOrZero(waitStart), nil
		}
		translateInUse.Add(-1)

		if waitStart.IsZero() {
			waitStart = time.Now()
			translateBlockedTotal.Add(1)
		}

		if err := parkForTranslateSlot(ctx); err != nil {
			return nil, sinceOrZero(waitStart), err
		}
	}
}

// parkForTranslateSlot waits for a freed slot or a limit change, keeping the waiting count balanced
// even when ctx is done, so an abandoned waiter can't leak queue depth.
func parkForTranslateSlot(ctx context.Context) error {
	translateWaiting.Add(1)
	defer translateWaiting.Add(-1)

	select {
	case <-translateSlotFreed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func sinceOrZero(start time.Time) time.Duration {
	if start.IsZero() {
		return 0
	}
	return time.Since(start)
}

// translateSemaphoreStats reports the number of translations holding a slot right now, the number
// queued behind the limit, the configured limit (0 meaning unlimited), and the cumulative count of
// acquires that had to wait.
func translateSemaphoreStats() (inUse, waiting, limit, blockedTotal int64) {
	return translateInUse.Load(), translateWaiting.Load(), translateLimit.Load(), translateBlockedTotal.Load()
}
