package events

import (
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/grip"
)

type histogramStream struct {
	point     *PerformanceHDR
	started   time.Time
	collector ftdc.Collector
	catcher   grip.Catcher
}

// NewHistogramRecorder collects data and stores them with a histogram
// format. Like the Collapsed recorder, the system saves each data
// point after a call to Begin.
//
// The timer histgrams have a minimum value of 1 microsecond, and a
// maximum value of 20 minutes, with 5 significant digits. The counter
// histograms store between 0 and 1 million, with 5 significant
// digits. The gauges are not stored as integers.
//
// The histogram reporter is not safe for concurrent use without a
// synchronixed wrapper.
func NewHistogramRecorder(collector ftdc.Collector) Recorder {
	return &histogramStream{
		point:     NewHistogramMillisecond(PerformanceGauges{}),
		collector: collector,
		catcher:   grip.NewExtendedCatcher(),
	}
}

func (r *histogramStream) SetID(id int64)       { r.point.ID = id }
func (r *histogramStream) SetState(val int64)   { r.point.Gauges.State = val }
func (r *histogramStream) SetWorkers(val int64) { r.point.Gauges.Workers = val }
func (r *histogramStream) SetFailed(val bool)   { r.point.Gauges.Failed = val }
func (r *histogramStream) IncOps(val int64) {
	r.catcher.Add(r.point.Counters.Operations.RecordValue(val))
}
func (r *histogramStream) IncSize(val int64) {
	r.catcher.Add(r.point.Counters.Size.RecordValue(val))
}
func (r *histogramStream) IncError(val int64) {
	r.catcher.Add(r.point.Counters.Errors.RecordValue(val))
}
func (r *histogramStream) End(dur time.Duration) {
	r.catcher.Add(r.point.Counters.Number.RecordValue(1))
	r.catcher.Add(r.point.Timers.Duration.RecordValue(int64(dur)))
	if !r.started.IsZero() {
		r.catcher.Add(r.point.Timers.Total.RecordValue(int64(time.Since(r.started))))
	}
}

func (r *histogramStream) Begin() {
	if r.point.Timestamp.IsZero() {
		r.point.Timestamp = r.started
	}

	if !r.started.IsZero() {
		r.catcher.Add(r.collector.Add(*r.point))
		r.point.Timestamp = time.Time{}
	}

	r.started = time.Now()
}

func (r *histogramStream) SetDuration(dur time.Duration) {
	r.catcher.Add(r.point.Timers.Duration.RecordValue(int64(dur)))
}

func (r *histogramStream) SetTotalDuration(dur time.Duration) {
	r.catcher.Add(r.point.Timers.Total.RecordValue(int64(dur)))
}

func (r *histogramStream) IncIterations(val int64) {
	r.catcher.Add(r.point.Counters.Number.RecordValue(val))
}

func (r *histogramStream) SetTime(t time.Time) { r.point.Timestamp = t }
func (r *histogramStream) Reset()              { r.started = time.Now() }

func (r *histogramStream) Flush() error {
	r.Begin()
	r.point = NewHistogramMillisecond(r.point.Gauges)
	r.started = time.Time{}
	err := r.catcher.Resolve()
	r.catcher = grip.NewBasicCatcher()
	return err
}
