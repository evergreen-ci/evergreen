package amboy

import (
	"context"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// ResolveErrors takes a Queue and iterates over the completed Jobs' results,
// returning a single aggregated error for all of the Queue's Jobs.
func ResolveErrors(ctx context.Context, q Queue) error {
	catcher := grip.NewCatcher()

	for result := range q.Results(ctx) {
		if err := ctx.Err(); err != nil {
			catcher.Add(err)
			break
		}

		catcher.Add(result.Error())
	}

	return catcher.Resolve()
}

// PopulateQueue adds Jobs from a channel to a Queue and returns an error with
// the aggregated results of these operations.
func PopulateQueue(ctx context.Context, q Queue, jobs <-chan Job) error {
	catcher := grip.NewCatcher()

	for j := range jobs {
		if err := ctx.Err(); err != nil {
			catcher.Add(err)
			break
		}

		catcher.Add(q.Put(ctx, j))
	}

	return catcher.Resolve()
}

// QueueReport holds the IDs of Jobs in a Queue based on their current state.
type QueueReport struct {
	Pending    []string `json:"pending"`
	InProgress []string `json:"in_progress"`
	Completed  []string `json:"completed"`
	Retrying   []string `json:"retrying"`
}

// Report returns a QueueReport status for the state of a Queue.
func Report(ctx context.Context, q Queue, limit int) QueueReport {
	var out QueueReport

	if limit == 0 {
		return out
	}

	var count int
	for info := range q.JobInfo(ctx) {
		switch {
		case info.Status.Completed:
			if info.Retry.ShouldRetry() {
				out.Retrying = append(out.Retrying, info.ID)
			} else {
				out.Completed = append(out.Completed, info.ID)
			}
		case info.Status.InProgress:
			out.InProgress = append(out.InProgress, info.ID)
		default:
			out.Pending = append(out.Pending, info.ID)
		}

		count++
		if limit > 0 && count >= limit {
			break
		}

	}

	return out
}

// RunJob executes a single job directly, without a Queue, with similar
// semantics as it would execute in a Queue: MaxTime is respected, and it uses
// similar logging as is present in the queue, with errors propagated
// functionally.
func RunJob(ctx context.Context, job Job) error {
	var cancel context.CancelFunc
	ti := job.TimeInfo()
	ti.Start = time.Now()
	job.UpdateTimeInfo(ti)
	if ti.MaxTime > 0 {
		ctx, cancel = context.WithTimeout(ctx, ti.MaxTime)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	job.Run(ctx)
	ti.End = time.Now()
	msg := message.Fields{
		"job_id":        job.ID(),
		"job_type":      job.Type().Name,
		"duration_secs": ti.Duration().Seconds(),
	}
	err := errors.WithStack(job.Error())
	if err != nil {
		grip.Error(message.WrapError(err, msg))
	} else {
		grip.Debug(msg)
	}

	return err
}
