package model

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// resetTranslateSemaphore clears the process-wide limiter counters so a test starts from a known
// state and doesn't leak counts into later tests.
func resetTranslateSemaphore(t *testing.T) {
	reset := func() {
		SetTranslateConcurrencyLimit(0)
		translateInUse.Store(0)
		translateWaiting.Store(0)
		translateBlockedTotal.Store(0)
	}
	reset()
	t.Cleanup(reset)
}

func TestAcquireTranslateSlotConcurrencyExceedingLimitBlocksAndTracksCounts(t *testing.T) {
	resetTranslateSemaphore(t)

	const limit = 2
	const numGoroutines = 5
	SetTranslateConcurrencyLimit(limit)

	var maxWaitedNS atomic.Int64
	acquired := make(chan struct{}, numGoroutines)
	gate := make(chan struct{})
	var wg sync.WaitGroup
	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, waited, err := acquireTranslateSlot(t.Context())
			require.NoError(t, err)
			defer release()

			if waited > 0 {
				maxWaitedNS.Store(max(maxWaitedNS.Load(), waited.Nanoseconds()))
			}
			acquired <- struct{}{}
			<-gate
		}()
	}

	// Exactly the limit may hold a slot at once, leaving the rest queued.
	require.Eventually(t, func() bool {
		inUse, waiting, _, _ := translateSemaphoreStats()
		return inUse == limit && waiting == numGoroutines-limit
	}, 10*time.Second, time.Millisecond)

	_, _, _, blockedTotal := translateSemaphoreStats()
	require.GreaterOrEqual(t, blockedTotal, int64(numGoroutines-limit))

	close(gate)
	wg.Wait()

	require.Len(t, acquired, numGoroutines)
	inUse, waiting, gotLimit, _ := translateSemaphoreStats()
	require.Zero(t, inUse)
	require.Zero(t, waiting)
	require.Equal(t, int64(limit), gotLimit)
	require.Positive(t, maxWaitedNS.Load(), "a blocked acquire should report the time it waited")
}

func TestAcquireTranslateSlotContextDoneLeavesNoWaiters(t *testing.T) {
	resetTranslateSemaphore(t)
	SetTranslateConcurrencyLimit(1)

	release, _, err := acquireTranslateSlot(t.Context())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	waiterDone := make(chan struct{})
	go func() {
		defer close(waiterDone)
		_, _, err := acquireTranslateSlot(ctx)
		require.ErrorIs(t, err, context.Canceled)
	}()

	require.Eventually(t, func() bool {
		_, waiting, _, _ := translateSemaphoreStats()
		return waiting == 1
	}, 10*time.Second, time.Millisecond)

	cancel()
	<-waiterDone
	release()

	inUse, waiting, _, _ := translateSemaphoreStats()
	require.Zero(t, waiting, "an abandoned waiter must not leak queue depth")
	require.Zero(t, inUse)
}

func TestAcquireTranslateSlotUnlimitedStillCountsInFlight(t *testing.T) {
	resetTranslateSemaphore(t)

	release, waited, err := acquireTranslateSlot(t.Context())
	require.NoError(t, err)
	inUse, waiting, limit, blockedTotal := translateSemaphoreStats()
	require.Equal(t, int64(1), inUse, "in-flight translations must be counted even with no limit set")
	require.Zero(t, waiting)
	require.Zero(t, limit)
	require.Zero(t, blockedTotal)
	require.Zero(t, waited)

	release()
	inUse, _, _, _ = translateSemaphoreStats()
	require.Zero(t, inUse)
}

func TestTranslateProjectContextDoneWhileQueuedStillRecordsSpanAttributes(t *testing.T) {
	resetTranslateSemaphore(t)
	SetTranslateConcurrencyLimit(1)

	release, _, err := acquireTranslateSlot(t.Context())
	require.NoError(t, err)
	defer release()

	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	ctx, cancel := context.WithCancel(t.Context())
	ctx, span := tp.Tracer("test").Start(ctx, "test")

	translateDone := make(chan struct{})
	go func() {
		defer close(translateDone)
		_, err := TranslateProject(ctx, &ParserProject{})
		require.ErrorIs(t, err, context.Canceled)
	}()

	require.Eventually(t, func() bool {
		_, waiting, _, _ := translateSemaphoreStats()
		return waiting == 1
	}, 10*time.Second, time.Millisecond)

	cancel()
	<-translateDone
	span.End()
	require.NoError(t, tp.Shutdown(t.Context()))

	ended := spanRecorder.Ended()
	require.Len(t, ended, 1)
	attrs := map[string]any{}
	for _, kv := range ended[0].Attributes() {
		attrs[string(kv.Key)] = kv.Value.AsInterface()
	}
	require.Equal(t, int64(1), attrs[ppTranslateLimitOtelAttribute])
	require.Equal(t, int64(1), attrs[ppTranslateInFlightOtelAttribute])
	require.Equal(t, int64(1), attrs[ppTranslateBlockedTotalOtelAttribute])
	require.Contains(t, attrs, ppTranslateWaitMSOtelAttribute)
}

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
			release, _, err := acquireTranslateSlot(t.Context())
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
		release, _, err := acquireTranslateSlot(t.Context())
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

	release, _, err := acquireTranslateSlot(t.Context())
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _, err = acquireTranslateSlot(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSetTranslateConcurrencyLimitIncreaseWakesBlockedWaiters(t *testing.T) {
	t.Cleanup(func() { SetTranslateConcurrencyLimit(0) })
	SetTranslateConcurrencyLimit(1)

	release, _, err := acquireTranslateSlot(t.Context())
	require.NoError(t, err)

	waiterAcquired := make(chan func())
	go func() {
		r, _, err := acquireTranslateSlot(t.Context())
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
