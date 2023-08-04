package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// ContainerTaskQueue represents an iterator that represents an ordered queue of
// container tasks that are ready be allocated a container.
type ContainerTaskQueue struct {
	queue    []task.Task
	position int
}

// NewContainerTaskQueue returns a populated iterator representing an ordered
// queue of container tasks that are ready to be allocated a container.
func NewContainerTaskQueue() (*ContainerTaskQueue, error) {
	q := &ContainerTaskQueue{}
	if err := q.populate(); err != nil {
		return nil, errors.Wrap(err, "initial population of container task queue")
	}
	return q, nil
}

// Next returns the next task that's ready for container allocation. It will
// return a nil task once there are no tasks remaining in the queue.
func (q *ContainerTaskQueue) Next() *task.Task {
	if q.position >= len(q.queue) {
		return nil
	}

	next := q.queue[q.position]

	q.position++

	return &next
}

// HasNext returns whether or not there are more container tasks that have not
// yet been returned.
func (q *ContainerTaskQueue) HasNext() bool {
	return q.position < len(q.queue)
}

// Len returns the number of tasks remaining.
func (q *ContainerTaskQueue) Len() int {
	return len(q.queue) - q.position
}

func (q *ContainerTaskQueue) populate() error {
	startAt := time.Now()
	candidates, err := task.FindNeedsContainerAllocation()
	if err != nil {
		return errors.Wrap(err, "finding candidate container tasks for allocation")
	}

	readyForAllocation, err := q.filterByProjectRefSettings(candidates)
	if err != nil {
		return errors.Wrap(err, "filtering candidate container tasks for allocation by project ref settings")
	}

	grip.Info(message.Fields{
		"message":     "generated container task queue",
		"included_on": evergreen.ContainerHealthDashboard,
		"candidates":  len(candidates),
		"queue":       len(readyForAllocation),
		"duration":    time.Since(startAt),
	})

	q.queue = readyForAllocation

	q.setFirstScheduledTime(startAt)

	return nil
}

func (q *ContainerTaskQueue) filterByProjectRefSettings(tasks []task.Task) ([]task.Task, error) {
	projRefs, err := q.getProjectRefs(tasks)
	if err != nil {
		return nil, errors.Wrap(err, "getting project refs")
	}

	var readyForAllocation []task.Task
	for _, t := range tasks {
		ref, ok := projRefs[t.Project]
		if !ok {
			grip.Warning(message.Fields{
				"message": "skipping task that is a candidate for allocation because did not find the project associated with it",
				"outcome": "skipping",
				"task":    t.Id,
				"project": t.Project,
				"context": "container task queue",
			})
			continue
		}

		canDispatch, reason := ProjectCanDispatchTask(&ref, &t)
		if !canDispatch {
			grip.Debug(message.Fields{
				"message": "skipping allocation for undispatchable task",
				"outcome": "skipping",
				"reason":  reason,
				"task":    t.Id,
				"project": t.Project,
				"context": "container task queue",
			})
			continue
		}
		grip.DebugWhen(reason != "", message.Fields{
			"message": "allowing allocation for task that can be dispatched",
			"outcome": "not skipping",
			"reason":  reason,
			"task":    t.Id,
			"project": t.Project,
			"context": "container task queue",
		})

		readyForAllocation = append(readyForAllocation, t)
	}

	return readyForAllocation, nil
}

func (q *ContainerTaskQueue) getProjectRefs(tasks []task.Task) (map[string]ProjectRef, error) {
	seenProjRefIDs := map[string]struct{}{}
	var projRefIDs []string
	for _, t := range tasks {
		if _, ok := seenProjRefIDs[t.Project]; ok {
			continue
		}
		projRefIDs = append(projRefIDs, t.Project)
		seenProjRefIDs[t.Project] = struct{}{}
	}

	if len(projRefIDs) == 0 {
		return map[string]ProjectRef{}, nil
	}

	projRefs, err := FindMergedProjectRefsByIds(projRefIDs...)
	if err != nil {
		return nil, errors.Wrap(err, "finding project refs for tasks")
	}

	projRefsByID := map[string]ProjectRef{}
	for _, ref := range projRefs {
		projRefsByID[ref.Id] = ref
	}

	return projRefsByID, nil
}

func (q *ContainerTaskQueue) setFirstScheduledTime(scheduledTime time.Time) {
	var notScheduledBefore []task.Task
	for i := range q.queue {
		t := q.queue[i]
		if utility.IsZeroTime(t.ScheduledTime) {
			notScheduledBefore = append(notScheduledBefore, t)
		}
	}

	if len(notScheduledBefore) == 0 {
		return
	}

	if err := task.SetTasksScheduledTime(notScheduledBefore, scheduledTime); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message":                 "could not set first scheduled time for new container tasks",
			"num_new_container_tasks": len(notScheduledBefore),
		}))
	}
}
