package events

import (
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/ftdc/util"
	"github.com/pkg/errors"
)

type rawStream struct {
	started   time.Time
	point     *Performance
	collector ftdc.Collector
	catcher   util.Catcher
}

// NewRawRecorder records a new event every time that the EndIteration method
// is called.
//
// The Raw recorder is not safe for concurrent access without a synchronized
// wrapper.
func NewRawRecorder(collector ftdc.Collector) Recorder {
	return &rawStream{
		collector: collector,
		point:     &Performance{Timestamp: time.Time{}},
		catcher:   util.NewCatcher(),
	}
}

func (r *rawStream) BeginIteration()                    { r.started = time.Now(); r.point.setTimestamp(r.started) }
func (r *rawStream) SetTime(t time.Time)                { r.point.Timestamp = t }
func (r *rawStream) SetID(val int64)                    { r.point.ID = val }
func (r *rawStream) SetTotalDuration(dur time.Duration) { r.point.Timers.Total = dur }
func (r *rawStream) SetDuration(dur time.Duration)      { r.point.Timers.Duration = dur }
func (r *rawStream) IncOperations(val int64)            { r.point.Counters.Operations += val }
func (r *rawStream) IncIterations(val int64)            { r.point.Counters.Number += val }
func (r *rawStream) IncSize(val int64)                  { r.point.Counters.Size += val }
func (r *rawStream) IncError(val int64)                 { r.point.Counters.Errors += val }
func (r *rawStream) SetState(val int64)                 { r.point.Gauges.State = val }
func (r *rawStream) SetWorkers(val int64)               { r.point.Gauges.Workers = val }
func (r *rawStream) SetFailed(val bool)                 { r.point.Gauges.Failed = val }
func (r *rawStream) EndIteration(dur time.Duration) {
	r.point.Counters.Number++
	if !r.started.IsZero() {
		r.point.Timers.Total += time.Since(r.started)
	}

	r.point.setTimestamp(r.started)
	r.point.Timers.Duration += dur
	r.catcher.Add(r.collector.Add(r.point))
	r.started = time.Time{}
}

func (r *rawStream) EndTest() error {
	if !r.point.Timestamp.IsZero() {
		r.catcher.Add(r.collector.Add(r.point))
	}
	err := r.catcher.Resolve()
	r.Reset()
	return errors.WithStack(err)
}

func (r *rawStream) Reset() {
	r.catcher = util.NewCatcher()
	r.point = &Performance{
		Gauges: r.point.Gauges,
	}
	r.started = time.Time{}
}
