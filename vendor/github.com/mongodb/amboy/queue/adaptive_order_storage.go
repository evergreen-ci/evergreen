package queue

import (
	"context"
	"math/rand"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/pkg/errors"
)

type adaptiveOrderItems struct {
	jobs      map[string]amboy.Job
	ready     []string
	waiting   []string
	stalled   []string
	completed []string
	passed    []string
}

func (items *adaptiveOrderItems) add(j amboy.Job) error {
	id := j.ID()
	if _, ok := items.jobs[id]; ok {
		return errors.Errorf("cannot add duplicate job with id '%s'", id)
	}

	items.jobs[id] = j

	if j.Status().Completed {
		items.completed = append(items.completed, id)
		return nil
	}

	switch j.Dependency().State() {
	case dependency.Ready:
		items.ready = append(items.ready, id)
	case dependency.Blocked:
		items.waiting = append(items.waiting, id)
	case dependency.Unresolved:
		items.stalled = append(items.stalled, id)
	case dependency.Passed:
		items.passed = append(items.passed, id)
	}

	return nil
}

func (items *adaptiveOrderItems) refilter(ctx context.Context) {
	new := []string{}
	for _, id := range items.waiting {
		if ctx.Err() != nil {
			return
		}

		job, ok := items.jobs[id]
		if !ok {
			continue
		}
		status := job.Status()
		if status.Completed || status.InProgress {
			items.completed = append(items.completed, id)
			continue
		}

		state := job.Dependency().State()
		if state == dependency.Ready {
			items.ready = append(items.ready, id)
			continue
		}

		if state == dependency.Blocked {
			new = append(new, id)
			continue
		}
		if state == dependency.Unresolved {
			items.stalled = append(items.stalled, id)
			continue
		}
	}
	items.waiting = new

	new = []string{}
	for _, id := range items.stalled {
		if ctx.Err() != nil {
			return
		}

		job, ok := items.jobs[id]
		if !ok {
			continue
		}

		status := job.Status()
		if status.Completed || status.InProgress {
			items.completed = append(items.completed, id)
			continue
		}

		state := job.Dependency().State()
		if state == dependency.Ready {
			items.ready = append(items.ready, id)
			continue
		}

		if state == dependency.Blocked {
			items.waiting = append(items.waiting, id)
			continue
		}

		if state == dependency.Unresolved {
			new = append(new, id)
			continue
		}
	}

	items.stalled = new

	// shuffle the order of the ready queue.
	//   in the future this might be good to sort based on the
	//   number of edges, and randomized otherwise.
	if len(items.ready) > 1 {
		new = make([]string, len(items.ready))
		for i, r := range rand.Perm(len(items.ready)) {
			new[i] = items.ready[r]
		}
		items.ready = new
	}
}
