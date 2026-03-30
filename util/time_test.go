package util

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextIntervalBoundary(t *testing.T) {
	epoch := time.Unix(0, 0).UTC()
	week := 7 * 24 * time.Hour

	t.Run("OnEpoch", func(t *testing.T) {
		got := NextIntervalBoundary(epoch, week, epoch)
		assert.True(t, got.Equal(epoch))
	})

	t.Run("ExactlyOnBoundary", func(t *testing.T) {
		onBoundary := epoch.Add(2 * week)
		got := NextIntervalBoundary(onBoundary, week, epoch)
		assert.True(t, got.Equal(onBoundary))
	})

	t.Run("BetweenBoundaries", func(t *testing.T) {
		between := epoch.Add(2*week + 3*time.Hour)
		want := epoch.Add(3 * week)
		got := NextIntervalBoundary(between, week, epoch)
		assert.True(t, got.Equal(want))
	})

	t.Run("ThirtyDayInterval", func(t *testing.T) {
		month := 30 * 24 * time.Hour
		between := epoch.Add(month + time.Hour)
		want := epoch.Add(2 * month)
		got := NextIntervalBoundary(between, month, epoch)
		assert.True(t, got.Equal(want))
	})

	t.Run("InvalidIntervalReturnsNow", func(t *testing.T) {
		now := time.Unix(12345, 0).UTC()
		got := NextIntervalBoundary(now, 0, epoch)
		assert.True(t, got.Equal(now))
		got = NextIntervalBoundary(now, 500*time.Millisecond, epoch)
		assert.True(t, got.Equal(now))
	})

	t.Run("NowBeforeEpoch", func(t *testing.T) {
		before := epoch.Add(-time.Hour)
		got := NextIntervalBoundary(before, week, epoch)
		assert.True(t, got.Equal(epoch))
	})
}

// nextBoundaryIncremental advances from epoch in whole-second steps until
// reaching a time >= nowUnix. It mirrors the documented contract without
// using the closed-form formula in NextIntervalBoundary.
func nextBoundaryIncremental(nowUnix, intervalSecs, epochUnix int64) int64 {
	if intervalSecs <= 0 {
		return nowUnix
	}
	if nowUnix < epochUnix {
		return epochUnix
	}
	t := epochUnix
	for t < nowUnix {
		t += intervalSecs
	}
	return t
}

func TestNextIntervalBoundaryMatchesIncrementalSearch(t *testing.T) {
	epoch := time.Unix(0, 0).UTC()
	epochUnix := epoch.Unix()
	weekSecs := int64((7 * 24 * time.Hour) / time.Second)
	monthSecs := int64((30 * 24 * time.Hour) / time.Second)

	cases := []struct {
		name   string
		now    time.Time
		ivSecs int64
	}{
		{"epoch", epoch, weekSecs},
		{"1970 second week", time.Date(1970, 1, 10, 0, 0, 0, 0, time.UTC), weekSecs},
		{"2026 mid year", time.Date(2026, 7, 4, 12, 30, 45, 0, time.UTC), weekSecs},
		{"2026 with nanos truncated like Unix", time.Date(2026, 1, 1, 0, 0, 0, 999999999, time.UTC), weekSecs},
		{"30 day step 2026", time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC), monthSecs},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			interval := time.Duration(tc.ivSecs) * time.Second
			got := NextIntervalBoundary(tc.now, interval, epoch)
			wantUnix := nextBoundaryIncremental(tc.now.Unix(), tc.ivSecs, epochUnix)
			assert.Equal(t, wantUnix, got.Unix(), "incremental search should match NextIntervalBoundary")
		})
	}
}

func TestNextIntervalBoundaryGridInvariants(t *testing.T) {
	epoch := time.Unix(0, 0).UTC()
	week := 7 * 24 * time.Hour
	weekSecs := int64(week / time.Second)
	month := 30 * 24 * time.Hour
	monthSecs := int64(month / time.Second)

	nows := []time.Time{
		time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC),
		time.Date(2019, 12, 31, 23, 59, 59, 0, time.UTC),
		time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	for i, now := range nows {
		t.Run(fmt.Sprintf("week_%d", i), func(t *testing.T) {
			got := NextIntervalBoundary(now, week, epoch)
			require.False(t, got.Before(now), "result must be >= now")
			elapsed := got.Unix() - epoch.Unix()
			require.Zero(t, elapsed%weekSecs, "result must sit on the epoch week grid")
			prev := got.Add(-week)
			require.True(t, prev.Unix() < now.Unix(),
				"the prior grid tick must be strictly before now (smallest future boundary)")
		})
	}

	for i, now := range nows {
		t.Run(fmt.Sprintf("month_%d", i), func(t *testing.T) {
			got := NextIntervalBoundary(now, month, epoch)
			require.False(t, got.Before(now))
			elapsed := got.Unix() - epoch.Unix()
			require.Zero(t, elapsed%monthSecs)
			prev := got.Add(-month)
			require.True(t, prev.Unix() < now.Unix())
		})
	}
}

func TestNextIntervalBoundaryOnBoundaryPreviousIsStrictlyBefore(t *testing.T) {
	epoch := time.Unix(0, 0).UTC()
	week := 7 * 24 * time.Hour
	// A time known to be exactly on the week grid (epoch + 400 weeks).
	onGrid := epoch.Add(400 * week)
	got := NextIntervalBoundary(onGrid, week, epoch)
	require.True(t, got.Equal(onGrid))
	require.True(t, got.Add(-week).Unix() < onGrid.Unix())
}
