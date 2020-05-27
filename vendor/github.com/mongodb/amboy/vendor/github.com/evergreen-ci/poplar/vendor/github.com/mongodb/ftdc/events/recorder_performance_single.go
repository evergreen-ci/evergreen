package events

import (
	"time"

	"github.com/mongodb/ftdc"
	"github.com/pkg/errors"
)

type singleStream struct {
	started   time.Time
	point     *Performance
	collector ftdc.Collector
}

// NewSingleRecorder records a single event every time the EndTest method is
// called, and otherwise just adds all counters and timing information to the
// underlying point.
//
// The Single recorder is not safe for concurrent access without a synchronized
// wrapper.
func NewSingleRecorder(collector ftdc.Collector) Recorder {
	return &singleStream{
		point:     &Performance{Timestamp: time.Time{}},
		collector: collector,
	}
}

func (r *singleStream) BeginIteration()                    { r.started = time.Now() }
func (r *singleStream) SetTime(t time.Time)                { r.point.Timestamp = t }
func (r *singleStream) SetID(id int64)                     { r.point.ID = id }
func (r *singleStream) SetTotalDuration(dur time.Duration) { r.point.Timers.Total += dur }
func (r *singleStream) SetDuration(dur time.Duration)      { r.point.Timers.Duration += dur }
func (r *singleStream) IncOperations(val int64)            { r.point.Counters.Operations += val }
func (r *singleStream) IncIterations(val int64)            { r.point.Counters.Number += val }
func (r *singleStream) IncSize(val int64)                  { r.point.Counters.Size += val }
func (r *singleStream) IncError(val int64)                 { r.point.Counters.Errors += val }
func (r *singleStream) SetState(val int64)                 { r.point.Gauges.State = val }
func (r *singleStream) SetWorkers(val int64)               { r.point.Gauges.Workers = val }
func (r *singleStream) SetFailed(val bool)                 { r.point.Gauges.Failed = val }
func (r *singleStream) EndIteration(dur time.Duration) {
	r.point.setTimestamp(r.started)
	r.point.Counters.Number++
	if !r.started.IsZero() {
		r.point.Timers.Total += time.Since(r.started)
		r.started = time.Time{}
	}
	r.point.Timers.Duration += dur
}

func (r *singleStream) EndTest() error {
	r.point.setTimestamp(r.started)
	err := errors.WithStack(r.collector.Add(r.point))
	r.Reset()
	return errors.WithStack(err)
}

func (r *singleStream) Reset() {
	r.point = &Performance{
		Gauges: r.point.Gauges,
	}
	r.started = time.Time{}
}
