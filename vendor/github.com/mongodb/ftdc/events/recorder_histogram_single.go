package events

import (
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/ftdc/util"
	"github.com/pkg/errors"
)

type histogramSingle struct {
	point     *PerformanceHDR
	started   time.Time
	collector ftdc.Collector
	catcher   util.Catcher
}

// NewSingleHistogramRecorder collects data and stores them with a histogram
// format. Like the Single recorder, the implementation persists the histogram
// every time you call EndTest.
//
// The timer histograms have a minimum value of 1 microsecond, and a maximum
// value of 1 minute, with 5 significant digits. The counter histograms store
// store between 0 and 10 thousand, with 5 significant digits. The gauges are
// stored as integers.
//
// The histogram Single reporter is not safe for concurrent use without a
// synchronized wrapper.
func NewSingleHistogramRecorder(collector ftdc.Collector) Recorder {
	return &histogramSingle{
		point:     NewHistogramMillisecond(PerformanceGauges{}),
		collector: collector,
		catcher:   util.NewCatcher(),
	}
}

func (r *histogramSingle) SetID(id int64)       { r.point.ID = id }
func (r *histogramSingle) SetState(val int64)   { r.point.Gauges.State = val }
func (r *histogramSingle) SetWorkers(val int64) { r.point.Gauges.Workers = val }
func (r *histogramSingle) SetFailed(val bool)   { r.point.Gauges.Failed = val }
func (r *histogramSingle) IncOperations(val int64) {
	r.catcher.Add(r.point.Counters.Operations.RecordValue(val))
}
func (r *histogramSingle) IncSize(val int64) {
	r.catcher.Add(r.point.Counters.Size.RecordValue(val))
}
func (r *histogramSingle) IncError(val int64) {
	r.catcher.Add(r.point.Counters.Errors.RecordValue(val))
}

func (r *histogramSingle) IncIterations(val int64) {
	r.catcher.Add(r.point.Counters.Number.RecordValue(val))
}

func (r *histogramSingle) EndIteration(dur time.Duration) {
	r.point.setTimestamp(r.started)
	r.catcher.Add(r.point.Counters.Number.RecordValue(1))
	r.catcher.Add(r.point.Timers.Duration.RecordValue(int64(dur)))
	if !r.started.IsZero() {
		r.catcher.Add(r.point.Timers.Total.RecordValue(int64(time.Since(r.started))))
		r.started = time.Time{}
	}
}

func (r *histogramSingle) SetTotalDuration(dur time.Duration) {
	r.catcher.Add(r.point.Timers.Total.RecordValue(int64(dur)))
}

func (r *histogramSingle) SetDuration(dur time.Duration) {
	r.catcher.Add(r.point.Timers.Duration.RecordValue(int64(dur)))
}

func (r *histogramSingle) SetTime(t time.Time) { r.point.Timestamp = t }
func (r *histogramSingle) BeginIteration()     { r.started = time.Now() }

func (r *histogramSingle) EndTest() error {
	r.point.setTimestamp(r.started)
	r.catcher.Add(r.collector.Add(r.point))
	err := r.catcher.Resolve()
	r.Reset()
	return errors.WithStack(err)
}

func (r *histogramSingle) Reset() {
	r.catcher = util.NewCatcher()
	r.point = NewHistogramMillisecond(r.point.Gauges)
	r.started = time.Time{}
}
