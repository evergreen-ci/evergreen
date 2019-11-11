package events

import (
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type groupStream struct {
	started       time.Time
	lastCollected time.Time
	interval      time.Duration
	point         Performance
	collector     ftdc.Collector
	catcher       grip.Catcher
}

// NewGroupedRecorder blends the collapsed and the interval recorders,
// but it persists during the Record call only if the specified
// interval has elapsed. The reset method also resets the
// last-collected time.
//
// The Group recorder is not safe for concurrent access.
func NewGroupedRecorder(collector ftdc.Collector, interval time.Duration) Recorder {
	return &groupStream{
		collector:     collector,
		catcher:       grip.NewExtendedCatcher(),
		interval:      interval,
		lastCollected: time.Now(),
	}
}

func (r *groupStream) Reset()                             { r.started = time.Now(); r.lastCollected = time.Now() }
func (r *groupStream) Begin()                             { r.started = time.Now() }
func (r *groupStream) IncOps(val int64)                   { r.point.Counters.Operations += val }
func (r *groupStream) IncIterations(val int64)            { r.point.Counters.Number += val }
func (r *groupStream) IncSize(val int64)                  { r.point.Counters.Size += val }
func (r *groupStream) IncError(val int64)                 { r.point.Counters.Errors += val }
func (r *groupStream) SetState(val int64)                 { r.point.Gauges.State = val }
func (r *groupStream) SetWorkers(val int64)               { r.point.Gauges.Workers = val }
func (r *groupStream) SetFailed(val bool)                 { r.point.Gauges.Failed = val }
func (r *groupStream) SetID(val int64)                    { r.point.ID = val }
func (r *groupStream) SetTime(t time.Time)                { r.point.Timestamp = t }
func (r *groupStream) SetDuration(dur time.Duration)      { r.point.Timers.Duration += dur }
func (r *groupStream) SetTotalDuration(dur time.Duration) { r.point.Timers.Total += dur }
func (r *groupStream) End(dur time.Duration) {
	r.point.Counters.Number++
	if !r.started.IsZero() {
		r.point.Timers.Total += time.Since(r.started)
		r.started = time.Time{}
	}
	r.point.Timers.Duration += dur

	if time.Since(r.lastCollected) >= r.interval {
		if r.point.Timestamp.IsZero() {
			r.point.Timestamp = r.started
		}

		r.catcher.Add(r.collector.Add(r.point))
		r.lastCollected = time.Now()
		r.point.Timestamp = time.Time{}
	}
}

func (r *groupStream) Flush() error {
	if r.point.Timestamp.IsZero() {
		if !r.started.IsZero() {
			r.point.Timestamp = r.started
		} else {
			r.point.Timestamp = time.Now()
		}
	}

	r.catcher.Add(r.collector.Add(r.point))
	r.lastCollected = time.Now()

	err := r.catcher.Resolve()
	r.catcher = grip.NewExtendedCatcher()
	r.point = Performance{
		Gauges: r.point.Gauges,
	}
	r.started = time.Time{}
	return errors.WithStack(err)
}
