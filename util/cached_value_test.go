package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCachedIntValue(t *testing.T) {
	assert := assert.New(t)
	trueRefresher := func(old int) (int, bool) { return 42, true }
	falseRefresher := func(old int) (int, bool) { return 42, false }
	cv := &CachedIntValue{
		Value:       21,
		TTL:         time.Minute,
		CollectedAt: time.Now(),
	}

	assert.True(time.Since(cv.CollectedAt) < cv.TTL)

	// its ok because it's not stale
	val, ok := cv.Get()
	assert.True(ok)
	assert.Equal(21, val)

	// make it stale but don't set a refresher
	cv.TTL = time.Second
	startingTime := time.Now().Add(-time.Minute)
	cv.CollectedAt = startingTime
	assert.False(time.Since(cv.CollectedAt) < cv.TTL)

	val, ok = cv.Get()
	assert.False(ok)
	assert.Equal(21, val)
	assert.Contains(cv.String(), "stale")

	// set the false refresher and make sure it doesn't change the value
	cv.refresher = falseRefresher
	val, ok = cv.Get()
	assert.False(ok)
	assert.Equal(21, val)
	assert.Contains(cv.String(), "stale")

	// set the true refresher and see the value change
	cv.refresher = trueRefresher
	val, ok = cv.Get()
	assert.True(ok)
	assert.Equal(42, val)
	assert.Equal(cv.String(), "42")
	assert.True(cv.CollectedAt.After(startingTime))

	// test refresher setting
	cv.refresher = nil
	assert.Error(cv.SetRefresher(nil))
	assert.NoError(cv.SetRefresher(trueRefresher))
	assert.NoError(cv.SetRefresher(trueRefresher))
	assert.NoError(cv.SetRefresher(falseRefresher))
}

func TestCachedDurationValue(t *testing.T) {
	assert := assert.New(t)
	trueRefresher := func(DurationStats) (DurationStats, bool) {
		return DurationStats{Average: 42 * time.Second, StdDev: 0}, true
	}
	falseRefresher := func(DurationStats) (DurationStats, bool) {
		return DurationStats{Average: 42 * time.Second, StdDev: 0}, false
	}
	cv := &CachedDurationValue{
		Value:       21 * time.Second,
		TTL:         time.Minute,
		CollectedAt: time.Now(),
	}

	assert.True(time.Since(cv.CollectedAt) < cv.TTL)

	// don't need to save (ok) if it's not stale
	val, ok := cv.Get()
	assert.False(ok)
	assert.Equal(21*time.Second, val.Average)

	// make it stale but don't set a refresher
	cv.TTL = time.Second
	startingTime := time.Now().Add(-time.Minute)
	cv.CollectedAt = startingTime
	assert.False(time.Since(cv.CollectedAt) < cv.TTL)

	val, ok = cv.Get()
	assert.False(ok)
	assert.Equal(21*time.Second, val.Average)
	assert.Contains(cv.String(), "stale")

	// set the false refresher and make sure it doesn't change the value
	cv.refresher = falseRefresher
	val, ok = cv.Get()
	assert.False(ok)
	assert.Equal(21*time.Second, val.Average)
	assert.Contains(cv.String(), "stale")

	// set the true refresher and see the value change
	cv.refresher = trueRefresher
	val, ok = cv.Get()
	assert.True(ok)
	assert.Equal(42*time.Second, val.Average)
	assert.Equal(cv.String(), "42s")
	assert.True(cv.CollectedAt.After(startingTime))

	// test refresher setting
	cv.refresher = nil
	assert.Error(cv.SetRefresher(nil))
	assert.NoError(cv.SetRefresher(trueRefresher))
	assert.NoError(cv.SetRefresher(trueRefresher))
	assert.NoError(cv.SetRefresher(falseRefresher))
}
