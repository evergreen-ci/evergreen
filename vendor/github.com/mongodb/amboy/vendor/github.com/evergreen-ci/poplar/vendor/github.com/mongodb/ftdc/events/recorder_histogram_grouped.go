package events

import (
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/ftdc/util"
	"github.com/pkg/errors"
)

type histogramGroupedStream struct {
	point         *PerformanceHDR
	lastCollected time.Time
	started       time.Time
	interval      time.Duration
	collector     ftdc.Collector
	catcher       util.Catcher
}

// NewHistogramGroupedRecorder captures data and stores them with a
// histogram format. Like the Grouped recorder, it persists an event if the
// specified interval has elapsed since the last time an event was captured.
// The reset method also resets the last-collected time.
//
// The timer histgrams have a minimum value of 1 microsecond, and a maximum
// value of 20 minutes, with 5 significant digits. The counter histograms store
// between 0 and 1 million, with 5 significant digits. The gauges are stored as
// integers.
//
// The histogram Grouped reporter is not safe for concurrent use without a
// synchronixed wrapper.
func NewHistogramGroupedRecorder(collector ftdc.Collector, interval time.Duration) Recorder {
	return &histogramGroupedStream{
		point:     NewHistogramMillisecond(PerformanceGauges{}),
		collector: collector,
		catcher:   util.NewCatcher(),
	}
}

func (r *histogramGroupedStream) SetID(id int64)       { r.point.ID = id }
func (r *histogramGroupedStream) SetState(val int64)   { r.point.Gauges.State = val }
func (r *histogramGroupedStream) SetWorkers(val int64) { r.point.Gauges.Workers = val }
func (r *histogramGroupedStream) SetFailed(val bool)   { r.point.Gauges.Failed = val }
func (r *histogramGroupedStream) IncOperations(val int64) {
	r.catcher.Add(r.point.Counters.Operations.RecordValue(val))
}
func (r *histogramGroupedStream) IncSize(val int64) {
	r.catcher.Add(r.point.Counters.Size.RecordValue(val))
}
func (r *histogramGroupedStream) IncError(val int64) {
	r.catcher.Add(r.point.Counters.Errors.RecordValue(val))
}
func (r *histogramGroupedStream) EndIteration(dur time.Duration) {
	r.point.setTimestamp(r.started)
	r.catcher.Add(r.point.Counters.Number.RecordValue(1))
	r.catcher.Add(r.point.Timers.Duration.RecordValue(int64(dur)))

	if !r.started.IsZero() {
		r.catcher.Add(r.point.Timers.Total.RecordValue(int64(time.Since(r.started))))
		r.started = time.Time{}
	}

	if time.Since(r.lastCollected) >= r.interval {
		r.catcher.Add(r.collector.Add(r.point))
		r.lastCollected = time.Now()
	}
}

func (r *histogramGroupedStream) SetTotalDuration(dur time.Duration) {
	r.catcher.Add(r.point.Timers.Total.RecordValue(int64(dur)))
}

func (r *histogramGroupedStream) SetDuration(dur time.Duration) {
	r.catcher.Add(r.point.Timers.Duration.RecordValue(int64(dur)))
}

func (r *histogramGroupedStream) IncIterations(val int64) {
	r.catcher.Add(r.point.Counters.Number.RecordValue(val))
}

func (r *histogramGroupedStream) SetTime(t time.Time) { r.point.Timestamp = t }
func (r *histogramGroupedStream) BeginIteration() {
	r.started = time.Now()
	r.point.setTimestamp(r.started)
}

func (r *histogramGroupedStream) EndTest() error {
	if !r.point.Timestamp.IsZero() {
		r.catcher.Add(r.collector.Add(r.point))
	}
	err := r.catcher.Resolve()
	r.Reset()
	return errors.WithStack(err)
}

func (r *histogramGroupedStream) Reset() {
	r.catcher = util.NewCatcher()
	r.point = NewHistogramMillisecond(r.point.Gauges)
	r.lastCollected = time.Time{}
	r.started = time.Time{}
}
