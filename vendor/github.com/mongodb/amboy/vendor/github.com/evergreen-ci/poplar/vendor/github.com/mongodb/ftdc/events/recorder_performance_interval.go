package events

import (
	"context"
	"sync"
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/ftdc/util"
	"github.com/pkg/errors"
)

type intervalStream struct {
	point     *Performance
	started   time.Time
	collector ftdc.Collector
	catcher   util.Catcher
	sync.Mutex

	interval time.Duration
	rootCtx  context.Context
	canceler context.CancelFunc
}

// NewIntervalRecorder has similar semantics to histogram Grouped recorder,
// but has a background process that persists data on the specified on the
// specified interval rather than as a side effect of the EndTest call.
//
// The background thread is started in the BeginIteration operation if it
// does not already exist and is terminated by the EndTest operation.
//
// The interval recorder is safe for concurrent use.
func NewIntervalRecorder(ctx context.Context, collector ftdc.Collector, interval time.Duration) Recorder {
	return &intervalStream{
		collector: collector,
		rootCtx:   ctx,
		point:     &Performance{Timestamp: time.Time{}},
		catcher:   util.NewCatcher(),
		interval:  interval,
	}
}

func (r *intervalStream) worker(ctx context.Context, interval time.Duration) {
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
			r.point = &Performance{
				Gauges: r.point.Gauges,
			}
			r.Unlock()
		}
	}
}

func (r *intervalStream) SetTime(t time.Time) {
	r.Lock()
	r.point.Timestamp = t
	r.Unlock()
}

func (r *intervalStream) SetID(id int64) {
	r.Lock()
	r.point.ID = id
	r.Unlock()
}

func (r *intervalStream) BeginIteration() {
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

func (r *intervalStream) EndIteration(dur time.Duration) {
	r.Lock()

	r.point.setTimestamp(r.started)
	if !r.started.IsZero() {
		r.point.Timers.Total += time.Since(r.started)
		r.started = time.Time{}
	}
	r.point.Timers.Duration += dur

	r.Unlock()
}

func (r *intervalStream) EndTest() error {
	r.Lock()

	if !r.point.Timestamp.IsZero() {
		r.catcher.Add(r.collector.Add(r.point))
	}
	err := r.catcher.Resolve()
	r.reset()

	r.Unlock()
	return errors.WithStack(err)
}

func (r *intervalStream) Reset() {
	r.Lock()
	r.reset()
	r.Unlock()
}

func (r *intervalStream) reset() {
	if r.canceler != nil {
		r.canceler()
		r.canceler = nil
	}
	r.catcher = util.NewCatcher()
	r.point = &Performance{
		Gauges: r.point.Gauges,
	}
	r.started = time.Time{}
}

func (r *intervalStream) SetTotalDuration(dur time.Duration) {
	r.Lock()
	r.point.Timers.Total += dur
	r.Unlock()
}

func (r *intervalStream) SetDuration(dur time.Duration) {
	r.Lock()
	r.point.Timers.Duration += dur
	r.Unlock()
}

func (r *intervalStream) IncIterations(val int64) {
	r.Lock()
	r.point.Counters.Number += val
	r.Unlock()
}

func (r *intervalStream) IncOperations(val int64) {
	r.Lock()
	r.point.Counters.Operations += val
	r.Unlock()
}

func (r *intervalStream) IncSize(val int64) {
	r.Lock()
	r.point.Counters.Size += val
	r.Unlock()
}

func (r *intervalStream) IncError(val int64) {
	r.Lock()
	r.point.Counters.Errors += val
	r.Unlock()
}

func (r *intervalStream) SetState(val int64) {
	r.Lock()
	r.point.Gauges.State = val
	r.Unlock()
}

func (r *intervalStream) SetWorkers(val int64) {
	r.Lock()
	r.point.Gauges.Workers = val
	r.Unlock()
}

func (r *intervalStream) SetFailed(val bool) {
	r.Lock()
	r.point.Gauges.Failed = val
	r.Unlock()
}
