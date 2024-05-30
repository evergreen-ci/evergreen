package util

import (
	"errors"
	"fmt"
	"time"
)

// CachedIntValueRefresher provides a mechanism for CachedIntValues to
// update their values when the current cached value
// expires. Implementations are responsible for logging  errors, as
// needed.
type CachedIntValueRefresher func(int) (int, bool)

// CachedIntValue represents a calculated int value saved in a
// database with a expiration time. When the data is not expired, the
// value is returned directly by the Get() method, otherwise, a
// refresh function is called, to update the value.
type CachedIntValue struct {
	Value       int           `bson:"value"`
	TTL         time.Duration `bson:"ttl"`
	CollectedAt time.Time     `bson:"collected_at"`
	refresher   CachedIntValueRefresher
}

// String implements fmt.Stringer reporting how stale the value is in
// the stale case.
func (v *CachedIntValue) String() string {
	staleFor := time.Since(v.CollectedAt)
	if staleFor > v.TTL {
		return fmt.Sprintf("%d (stale; %s)", v.Value, staleFor)
	}

	return fmt.Sprintf("%d", v.Value)
}

// SetRefresher sets CachedIntValueRefresher for the object which is
// needed when reading CachedIntValue objects out of the database.
//
// It is not permissible to set the refresher to either nil or a value
// when it is *not* nil.
func (v *CachedIntValue) SetRefresher(r CachedIntValueRefresher) error {
	if r == nil {
		return errors.New("cannot set a nil refresher")
	}

	v.refresher = r
	return nil
}

// Get returns the value, refreshing it when its stale. The "ok" value
// reports errors with the refresh process and alerts callers that
// the value might be stale.
func (v *CachedIntValue) Get() (int, bool) {
	if time.Since(v.CollectedAt) < v.TTL {
		return v.Value, true
	}

	if v.refresher == nil {
		return v.Value, false
	}

	nv, ok := v.refresher(v.Value)
	if !ok {
		return v.Value, false
	}

	v.Value = nv
	v.CollectedAt = time.Now()

	return v.Value, true
}

type DurationStats struct {
	Average time.Duration
	StdDev  time.Duration
}

// CachedDurationValueRefresher provides a mechanism for CachedDurationValues to
// update their values when the current cached value
// expires. Implementations are responsible for logging errors, as
// needed.
type CachedDurationValueRefresher func(DurationStats) (DurationStats, bool)

// CachedDurationValue represents a calculated int value saved in a
// database with a expiration time. When the data is not expired, the
// value is returned directly by the Get() method, otherwise, a
// refresh function is called, to update the value.
type CachedDurationValue struct {
	Value       time.Duration `bson:"value"`
	StdDev      time.Duration `bson:"std_dev"`
	TTL         time.Duration `bson:"ttl"`
	CollectedAt time.Time     `bson:"collected_at"`
	refresher   CachedDurationValueRefresher
}

// String implements fmt.Stringer reporting how stale the value is in
// the stale case.
func (v *CachedDurationValue) String() string {
	staleFor := time.Since(v.CollectedAt)
	if staleFor > v.TTL {
		return fmt.Sprintf("%d (stale; %s)", v.Value, staleFor)
	}

	return v.Value.String()
}

// SetRefresher sets CachedDurationValueRefresher for the object which is
// needed when reading CachedDurationValue objects out of the database.
//
// It is not permissible to set the refresher to either nil or a value
// when it is *not* nil.
func (v *CachedDurationValue) SetRefresher(r CachedDurationValueRefresher) error {
	if r == nil {
		return errors.New("cannot set a nil refresher")
	}

	v.refresher = r
	return nil
}

// Get returns the value, refreshing it when its stale. The "ok" value
// tells the caller that the value needs to be persisted and may have
// changed since the last time Get was called.
func (v *CachedDurationValue) Get() (DurationStats, bool) {
	previous := DurationStats{Average: v.Value, StdDev: v.StdDev}
	if time.Since(v.CollectedAt) < v.TTL {
		return previous, false
	}

	if v.refresher == nil {
		return previous, false
	}

	nv, ok := v.refresher(previous)
	if !ok {
		return previous, false
	}

	v.Value = nv.Average
	v.StdDev = nv.StdDev
	v.CollectedAt = time.Now()

	return DurationStats{Average: v.Value, StdDev: v.StdDev}, true
}
