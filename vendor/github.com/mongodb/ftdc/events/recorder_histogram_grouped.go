package events

import (
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/grip"
)

type histogramGroupedStream struct {
	point         *PerformanceHDR
	lastCollected time.Time
	started       time.Time
	interval      time.Duration
	collector     ftdc.Collector
	catcher       grip.Catcher
}

// NewHistogramGroupedRecorder captures data and stores them with a histogramGrouped
// format. Like the Grouped Recorder, it persists an event if the specified
// interval has elapsed since the last time an event was captured. The
// reset method also resets the last-collected time.
//
// The timer histgrams have a minimum value of 1 microsecond, and a
// maximum value of 20 minutes, with 5 significant digits. The counter
// histogramGroupeds store between 0 and 1 million, with 5 significant
// digits. The gauges are not stored as integers.
//
// The histogramGrouped reporter is not safe for concurrent use without a
// synchronixed wrapper.
func NewHistogramGroupedRecorder(collector ftdc.Collector, interval time.Duration) Recorder {
	return &histogramGroupedStream{
		point:     NewHistogramMillisecond(PerformanceGauges{}),
		collector: collector,
		catcher:   grip.NewExtendedCatcher(),
	}
}

func (r *histogramGroupedStream) SetID(id int64)       { r.point.ID = id }
func (r *histogramGroupedStream) SetState(val int64)   { r.point.Gauges.State = val }
func (r *histogramGroupedStream) SetWorkers(val int64) { r.point.Gauges.Workers = val }
func (r *histogramGroupedStream) SetFailed(val bool)   { r.point.Gauges.Failed = val }
func (r *histogramGroupedStream) IncOps(val int64) {
	r.catcher.Add(r.point.Counters.Operations.RecordValue(val))
}
func (r *histogramGroupedStream) IncSize(val int64) {
	r.catcher.Add(r.point.Counters.Size.RecordValue(val))
}
func (r *histogramGroupedStream) IncError(val int64) {
	r.catcher.Add(r.point.Counters.Errors.RecordValue(val))
}
func (r *histogramGroupedStream) End(dur time.Duration) {
	r.catcher.Add(r.point.Counters.Number.RecordValue(1))
	r.catcher.Add(r.point.Timers.Duration.RecordValue(int64(dur)))

	if !r.started.IsZero() {
		r.catcher.Add(r.point.Timers.Total.RecordValue(int64(time.Since(r.started))))
	}

	if r.point.Timestamp.IsZero() {
		r.point.Timestamp = r.started
	}

	if time.Since(r.lastCollected) >= r.interval {
		r.catcher.Add(r.collector.Add(*r.point))
		r.lastCollected = time.Now()
		r.point.Timestamp = time.Time{}
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
func (r *histogramGroupedStream) Begin()              { r.started = time.Now() }
func (r *histogramGroupedStream) Reset()              { r.started = time.Now(); r.lastCollected = time.Now() }

func (r *histogramGroupedStream) Flush() error {
	r.catcher.Add(r.collector.Add(*r.point))
	r.point = NewHistogramMillisecond(r.point.Gauges)
	r.started = time.Time{}
	err := r.catcher.Resolve()
	r.catcher = grip.NewBasicCatcher()
	return err
}
