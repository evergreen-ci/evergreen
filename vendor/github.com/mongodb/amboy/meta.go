package amboy

import (
	"context"

	"github.com/mongodb/grip"
)

// ResolveErrors takes a queue object and iterates over the results
// and returns a single aggregated error for the queue's job. The
// completeness of this operation depends on the implementation of a
// the queue implementation's Results() method.
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

// PopulateQueue adds jobs from a channel to a queue and returns an
// error with the aggregated results of these operations.
func PopulateQueue(ctx context.Context, q Queue, jobs <-chan Job) error {
	catcher := grip.NewCatcher()

	for j := range jobs {
		if err := ctx.Err(); err != nil {
			catcher.Add(err)
			break
		}

		catcher.Add(q.Put(j))
	}

	return catcher.Resolve()
}

// QueueReport holds the ids of all tasks in a queue by state.
type QueueReport struct {
	Completed  []string `json:"completed"`
	InProgress []string `json:"in_progress"`
	Pending    []string `json:"pending"`
}

// Report returns a QueueReport status for the state of a queue.
func Report(ctx context.Context, q Queue, limit int) QueueReport {
	var out QueueReport

	if limit == 0 {
		return out
	}

	var count int
	for stat := range q.JobStats(ctx) {
		switch {
		case stat.Completed:
			out.Completed = append(out.Completed, stat.ID)
		case stat.InProgress:
			out.InProgress = append(out.InProgress, stat.ID)
		default:
			out.Pending = append(out.Pending, stat.ID)
		}

		count++
		if limit > 0 && count >= limit {
			break
		}

	}

	return out
}
