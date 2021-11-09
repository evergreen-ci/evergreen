package events

import (
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/ftdc/util"
	"github.com/pkg/errors"
)

type groupStream struct {
	started       time.Time
	lastCollected time.Time
	interval      time.Duration
	point         *Performance
	collector     ftdc.Collector
	catcher       util.Catcher
}

// NewGroupedRecorder blends the single and the interval recorders, but it
// persists during the EndIteration call only if the specified interval has
// elapsed. EndTest will persist any left over data.
//
// The Group recorder is not safe for concurrent access without a synchronized
// wrapper.
func NewGroupedRecorder(collector ftdc.Collector, interval time.Duration) Recorder {
	return &groupStream{
		collector:     collector,
		point:         &Performance{Timestamp: time.Time{}},
		catcher:       util.NewCatcher(),
		interval:      interval,
		lastCollected: time.Now(),
	}
}

func (r *groupStream) BeginIteration()                    { r.started = time.Now(); r.point.setTimestamp(r.started) }
func (r *groupStream) IncOperations(val int64)            { r.point.Counters.Operations += val }
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
func (r *groupStream) EndIteration(dur time.Duration) {
	r.point.Counters.Number++
	if !r.started.IsZero() {
		r.point.Timers.Total += time.Since(r.started)
	}
	r.point.Timers.Duration += dur

	if time.Since(r.lastCollected) >= r.interval {
		r.point.setTimestamp(r.started)
		r.catcher.Add(r.collector.Add(r.point))
		r.lastCollected = time.Now()
		r.point.Timestamp = time.Time{}
	}
	r.started = time.Time{}
}

func (r *groupStream) EndTest() error {
	if !r.point.Timestamp.IsZero() {
		r.catcher.Add(r.collector.Add(r.point))
	}
	err := r.catcher.Resolve()
	r.Reset()
	return errors.WithStack(err)
}

func (r *groupStream) Reset() {
	r.catcher = util.NewCatcher()
	r.point = &Performance{
		Gauges: r.point.Gauges,
	}
	r.started = time.Time{}
	r.lastCollected = time.Time{}
}
