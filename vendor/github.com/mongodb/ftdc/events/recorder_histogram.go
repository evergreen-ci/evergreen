package events

import (
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/ftdc/util"
	"github.com/pkg/errors"
)

type histogramStream struct {
	point     *PerformanceHDR
	started   time.Time
	collector ftdc.Collector
	catcher   util.Catcher
}

// NewHistogramRecorder collects data and stores them with a histogram format.
// Like the Raw recorder, the system saves each data point after each call to
// EndIteration.
//
// The timer histgrams have a minimum value of 1 microsecond, and a maximum
// value of 20 minutes, with 5 significant digits. The counter histograms store
// between 0 and 1 million, with 5 significant digits. The gauges are stored as
// integers.
//
// The histogram reporter is not safe for concurrent use without a synchronized
// wrapper.
func NewHistogramRecorder(collector ftdc.Collector) Recorder {
	return &histogramStream{
		point:     NewHistogramMillisecond(PerformanceGauges{}),
		collector: collector,
		catcher:   util.NewCatcher(),
	}
}

func (r *histogramStream) SetID(id int64)       { r.point.ID = id }
func (r *histogramStream) SetState(val int64)   { r.point.Gauges.State = val }
func (r *histogramStream) SetWorkers(val int64) { r.point.Gauges.Workers = val }
func (r *histogramStream) SetFailed(val bool)   { r.point.Gauges.Failed = val }
func (r *histogramStream) IncOperations(val int64) {
	r.catcher.Add(r.point.Counters.Operations.RecordValue(val))
}
func (r *histogramStream) IncSize(val int64) {
	r.catcher.Add(r.point.Counters.Size.RecordValue(val))
}
func (r *histogramStream) IncError(val int64) {
	r.catcher.Add(r.point.Counters.Errors.RecordValue(val))
}
func (r *histogramStream) EndIteration(dur time.Duration) {
	r.point.setTimestamp(r.started)
	r.catcher.Add(r.point.Counters.Number.RecordValue(1))
	r.catcher.Add(r.point.Timers.Duration.RecordValue(int64(dur)))
	if !r.started.IsZero() {
		r.catcher.Add(r.point.Timers.Total.RecordValue(int64(time.Since(r.started))))
		r.started = time.Time{}
	}

	r.catcher.Add(r.collector.Add(r.point))
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
func (r *histogramStream) BeginIteration()     { r.started = time.Now(); r.point.setTimestamp(r.started) }

func (r *histogramStream) EndTest() error {
	if !r.point.Timestamp.IsZero() {
		r.catcher.Add(r.collector.Add(r.point))
	}
	err := r.catcher.Resolve()
	r.Reset()
	return errors.WithStack(err)
}

func (r *histogramStream) Reset() {
	r.catcher = util.NewCatcher()
	r.point = NewHistogramMillisecond(r.point.Gauges)
	r.started = time.Time{}
}
