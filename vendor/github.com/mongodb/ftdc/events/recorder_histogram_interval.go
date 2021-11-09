package events

import (
	"context"
	"sync"
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/ftdc/util"
	"github.com/pkg/errors"
)

type intervalHistogramStream struct {
	point     *PerformanceHDR
	started   time.Time
	collector ftdc.Collector
	catcher   util.Catcher
	sync.Mutex

	interval time.Duration
	rootCtx  context.Context
	canceler context.CancelFunc
}

// NewIntervalHistogramRecorder has similar semantics to histogram Grouped
// recorder, but has a background process that persists data on the specified
// on the specified interval rather than as a side effect of the EndTest call.
//
// The background thread is started if it doesn't exist in the BeginIteration
// operation and is terminated by the EndTest operation.
//
// The interval histogram recorder is safe for concurrent use.
func NewIntervalHistogramRecorder(ctx context.Context, collector ftdc.Collector, interval time.Duration) Recorder {
	return &intervalHistogramStream{
		collector: collector,
		rootCtx:   ctx,
		catcher:   util.NewCatcher(),
		interval:  interval,
		point:     NewHistogramMillisecond(PerformanceGauges{}),
	}
}

func (r *intervalHistogramStream) worker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Lock()
			// check context error in case in between the time when
			// the lock is requested and when the lock is obtained,
			// the context has been canceled
			if ctx.Err() != nil {
				return
			}
			r.point.setTimestamp(r.started)
			r.catcher.Add(r.collector.Add(r.point))
			r.point.Timestamp = time.Time{}
			r.point = NewHistogramMillisecond(r.point.Gauges)
			r.Unlock()
		}
	}
}

func (r *intervalHistogramStream) BeginIteration() {
	r.Lock()
	if r.canceler == nil {
		// start new background ticker
		var newCtx context.Context
		newCtx, r.canceler = context.WithCancel(r.rootCtx)
		go r.worker(newCtx, r.interval)
		// release and return
	}

	r.started = time.Now()
	r.point.setTimestamp(r.started)
	r.Unlock()
}

func (r *intervalHistogramStream) EndIteration(dur time.Duration) {
	r.Lock()
	r.point.setTimestamp(r.started)
	r.catcher.Add(r.point.Counters.Number.RecordValue(1))
	r.catcher.Add(r.point.Timers.Duration.RecordValue(int64(dur)))

	if !r.started.IsZero() {
		r.catcher.Add(r.point.Timers.Total.RecordValue(int64(time.Since(r.started))))
	}

	r.Unlock()
}

func (r *intervalHistogramStream) SetTime(t time.Time) {
	r.Lock()
	r.point.Timestamp = t
	r.Unlock()
}

func (r *intervalHistogramStream) SetID(id int64) {
	r.Lock()
	r.point.ID = id
	r.Unlock()
}

func (r *intervalHistogramStream) SetTotalDuration(dur time.Duration) {
	r.Lock()
	r.catcher.Add(r.point.Timers.Total.RecordValue(int64(dur)))
	r.Unlock()
}

func (r *intervalHistogramStream) SetDuration(dur time.Duration) {
	r.Lock()
	r.catcher.Add(r.point.Timers.Duration.RecordValue(int64(dur)))
	r.Unlock()
}

func (r *intervalHistogramStream) EndTest() error {
	r.Lock()

	if !r.started.IsZero() {
		r.catcher.Add(r.point.Timers.Total.RecordValue(int64(time.Since(r.started))))
		r.started = time.Time{}
	}
	if !r.point.Timestamp.IsZero() {
		r.catcher.Add(r.collector.Add(r.point))
	}
	err := r.catcher.Resolve()
	r.reset()

	r.Unlock()
	return errors.WithStack(err)
}

func (r *intervalHistogramStream) Reset() {
	r.Lock()
	r.reset()
	r.Unlock()
}

func (r *intervalHistogramStream) reset() {
	if r.canceler != nil {
		r.canceler()
		r.canceler = nil
	}
	r.catcher = util.NewCatcher()
	r.point = NewHistogramMillisecond(r.point.Gauges)
	r.started = time.Time{}
}

func (r *intervalHistogramStream) IncOperations(val int64) {
	r.Lock()
	r.catcher.Add(r.point.Counters.Operations.RecordValue(val))
	r.Unlock()
}

func (r *intervalHistogramStream) IncIterations(val int64) {
	r.Lock()
	r.catcher.Add(r.point.Counters.Number.RecordValue(val))
	r.Unlock()
}

func (r *intervalHistogramStream) IncSize(val int64) {
	r.Lock()
	r.catcher.Add(r.point.Counters.Size.RecordValue(val))
	r.Unlock()
}

func (r *intervalHistogramStream) IncError(val int64) {
	r.Lock()
	r.catcher.Add(r.point.Counters.Errors.RecordValue(val))
	r.Unlock()
}

func (r *intervalHistogramStream) SetState(val int64) {
	r.Lock()
	r.point.Gauges.State = val
	r.Unlock()
}

func (r *intervalHistogramStream) SetWorkers(val int64) {
	r.Lock()
	r.point.Gauges.Workers = val
	r.Unlock()
}

func (r *intervalHistogramStream) SetFailed(val bool) {
	r.Lock()
	r.point.Gauges.Failed = val
	r.Unlock()
}
