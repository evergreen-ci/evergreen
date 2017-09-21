package amboy

import (
	"github.com/mongodb/grip"
	"golang.org/x/net/context"
)

// ResolveErrors takes a queue object and iterates over the results
// and returns a single aggregated error for the queue's job. The
// completeness of this operation depends on the implementation of a
// the queue implementation's Results() method.
func ResolveErrors(ctx context.Context, q Queue) error {
	catcher := grip.NewCatcher()

	for result := range q.Results() {
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
