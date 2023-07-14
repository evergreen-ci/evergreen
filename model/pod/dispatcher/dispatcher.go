package dispatcher

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// PodDispatcher represents a set of tasks that are dispatched to a set of pods
// that can run those tasks.
type PodDispatcher struct {
	// ID is the unique identifier for this dispatcher.
	ID string `bson:"_id" json:"id"`
	// GroupID is the unique identifier for the set of tasks that should run in
	// this dispatcher.
	GroupID string `bson:"group_id" json:"group_id"`
	// PodIDs are the identifiers for the pods that run the tasks.
	PodIDs []string `bson:"pod_ids" json:"pod_ids"`
	// TaskIDs is the identifiers for the set of tasks to run.
	TaskIDs []string `bson:"task_ids" json:"task_ids"`
	// ModificationCount is an incrementing lock used to resolve conflicting
	// updates to the dispatcher.
	ModificationCount int `bson:"modification_count" json:"modification_count"`
	// LastModified is the timestamp when the pod dispatcher was last modified.
	LastModified time.Time `bson:"last_modified" json:"last_modified"`
}

// NewPodDispatcher returns a new pod dispatcher.
func NewPodDispatcher(groupID string, taskIDs, podIDs []string) PodDispatcher {
	return PodDispatcher{
		ID:                primitive.NewObjectID().Hex(),
		GroupID:           groupID,
		PodIDs:            podIDs,
		TaskIDs:           taskIDs,
		ModificationCount: 0,
	}
}

// Insert inserts the pod dispatcher into the DB.
func (pd *PodDispatcher) Insert() error {
	return db.Insert(Collection, pd)
}

func (pd *PodDispatcher) atomicUpsertQuery() bson.M {
	return bson.M{
		IDKey:                pd.ID,
		GroupIDKey:           pd.GroupID,
		ModificationCountKey: pd.ModificationCount,
	}
}

func (pd *PodDispatcher) atomicUpsertUpdate(lastModified time.Time) bson.M {
	return bson.M{
		"$setOnInsert": bson.M{
			IDKey:      pd.ID,
			GroupIDKey: pd.GroupID,
		},
		"$set": bson.M{
			PodIDsKey:       pd.PodIDs,
			TaskIDsKey:      pd.TaskIDs,
			LastModifiedKey: lastModified,
		},
		"$inc": bson.M{
			ModificationCountKey: 1,
		},
	}
}

// UpsertAtomically inserts/updates the pod dispatcher depending on whether the
// document already exists.
func (pd *PodDispatcher) UpsertAtomically() (*adb.ChangeInfo, error) {
	lastModified := utility.BSONTime(time.Now())
	change, err := UpsertOne(pd.atomicUpsertQuery(), pd.atomicUpsertUpdate(lastModified))
	if err != nil {
		return change, err
	}
	pd.ModificationCount++
	pd.LastModified = lastModified
	return change, nil
}

// AssignNextTask assigns the pod the next available task to run. Returns nil if
// there's no task available to run. If the pod is already running a task, this
// will return an error.
func (pd *PodDispatcher) AssignNextTask(ctx context.Context, env evergreen.Environment, p *pod.Pod) (*task.Task, error) {
	if p.TaskRuntimeInfo.RunningTaskID != "" {
		return nil, errors.Errorf("cannot assign a new task to a pod that is already running task '%s' execution %d", p.TaskRuntimeInfo.RunningTaskID, p.TaskRuntimeInfo.RunningTaskExecution)
	}

	grip.WarningWhen(len(pd.TaskIDs) > 1, message.Fields{
		"message":    "programmatic error: task groups are not supported yet, so dispatcher should have at most 1 container task to dispatch",
		"pod":        p.ID,
		"dispatcher": pd.ID,
		"tasks":      pd.TaskIDs,
	})

	for len(pd.TaskIDs) > 0 {
		taskID := pd.TaskIDs[0]
		t, err := task.FindOneId(taskID)
		if err != nil {
			return nil, errors.Wrapf(err, "finding task '%s'", taskID)
		}
		if t == nil {
			grip.Notice(message.Fields{
				"message":    "nonexistent task in the dispatch queue",
				"outcome":    "dequeueing",
				"context":    "pod group task dispatcher",
				"task":       taskID,
				"dispatcher": pd.ID,
			})
			if err := pd.dequeue(ctx, env); err != nil {
				return nil, errors.Wrapf(err, "dequeueing nonexistent task '%s' from dispatch queue", taskID)
			}
			continue
		}

		isDispatchable, err := pd.checkTaskIsDispatchable(ctx, env, t)
		if err != nil {
			return nil, errors.Wrap(err, "checking task dispatchability")
		}
		if !isDispatchable {
			if err := pd.dequeueUndispatchableTask(ctx, env, t); err != nil {
				return nil, errors.Wrapf(err, "dequeueing undispatchable task '%s'", t.Id)
			}
			continue
		}

		if err := pd.dispatchTaskAtomically(ctx, env, p, t); err != nil {
			return nil, errors.Wrapf(err, "dispatching task '%s' to pod '%s'", t.Id, p.ID)
		}

		event.LogPodAssignedTask(p.ID, t.Id, t.Execution)
		event.LogContainerTaskDispatched(t.Id, t.Execution, p.ID)

		if t.IsPartOfDisplay() {
			if err := model.UpdateDisplayTaskForTask(t); err != nil {
				return nil, errors.Wrap(err, "updating parent display task")
			}
		}

		grip.Info(message.Fields{
			"message":                    "successfully dispatched task to pod",
			"task":                       t.Id,
			"pod":                        p.ID,
			"secs_since_task_activation": time.Since(t.ActivatedTime).Seconds(),
			"secs_since_pod_allocation":  time.Since(p.TimeInfo.Initializing).Seconds(),
			"secs_since_pod_creation":    time.Since(p.TimeInfo.Starting).Seconds(),
		})

		return t, nil
	}

	return nil, nil
}

// dispatchTaskAtomically performs the DB updates to assign a task to run on a
// pod.
func (pd *PodDispatcher) dispatchTaskAtomically(ctx context.Context, env evergreen.Environment, p *pod.Pod, t *task.Task) error {
	session, err := env.Client().StartSession()
	if err != nil {
		return errors.Wrap(err, "starting transaction session")
	}
	defer session.EndSession(ctx)

	if _, err := session.WithTransaction(ctx, pd.dispatchTask(env, p, t)); err != nil {
		return err
	}
	return nil
}

func (pd *PodDispatcher) dispatchTask(env evergreen.Environment, p *pod.Pod, t *task.Task) func(mongo.SessionContext) (interface{}, error) {
	return func(sessCtx mongo.SessionContext) (interface{}, error) {
		if err := p.SetRunningTask(sessCtx, env, t.Id, t.Execution); err != nil {
			return nil, errors.Wrapf(err, "setting pod's running task")
		}

		if err := t.MarkAsContainerDispatched(sessCtx, env, p.ID, p.AgentVersion); err != nil {
			return nil, errors.Wrapf(err, "marking task as dispatched")
		}

		if err := pd.dequeue(sessCtx, env); err != nil {
			return nil, errors.Wrapf(err, "dequeueing task")
		}

		return nil, nil
	}
}

// dequeue removes the head of the task queue from the dispatcher.
func (pd *PodDispatcher) dequeue(ctx context.Context, env evergreen.Environment) (err error) {
	oldTaskIDs := pd.TaskIDs
	pd.TaskIDs = pd.TaskIDs[1:]
	defer func() {
		if err != nil {
			pd.TaskIDs = oldTaskIDs
		}
	}()

	lastModified := utility.BSONTime(time.Now())
	res, err := env.DB().Collection(Collection).UpdateOne(ctx, pd.atomicUpsertQuery(), pd.atomicUpsertUpdate(lastModified))
	if err != nil {
		return errors.Wrap(err, "upserting dispatcher")
	}
	if res.ModifiedCount == 0 {
		return errors.New("dispatcher was not updated")
	}

	pd.ModificationCount++
	pd.LastModified = lastModified

	return nil
}

// dequeueUndispatchableTask removes the task from the dispatcher and, if the
// task indicates a container has been allocated for it, marks it as
// unallocated.
func (pd *PodDispatcher) dequeueUndispatchableTask(ctx context.Context, env evergreen.Environment, t *task.Task) error {
	if !t.ContainerAllocated {
		return errors.Wrap(pd.dequeue(ctx, env), "dequeueing task that is already in a non-allocated state")
	}

	if err := t.MarkAsContainerDeallocated(ctx, env); err != nil {
		return errors.Wrap(err, "marking task unallocated")
	}

	if err := pd.dequeue(ctx, env); err != nil {
		return errors.Wrap(err, "dequeueing task")
	}

	return nil
}

// checkTaskIsDispatchable checks if a task is able to dispatch based on its
// current state and its project ref's settings.
func (pd *PodDispatcher) checkTaskIsDispatchable(ctx context.Context, env evergreen.Environment, t *task.Task) (shouldRun bool, err error) {
	if !t.IsContainerDispatchable() {
		grip.Notice(message.Fields{
			"message":    "container task in dispatch queue is not dispatchable",
			"outcome":    "task is not dispatchable",
			"context":    "pod group task dispatcher",
			"task":       t.Id,
			"dispatcher": pd.ID,
		})
		return false, nil
	}

	refs, err := model.FindProjectRefsByIds(t.Project)
	if err != nil {
		return false, errors.Wrapf(err, "finding project ref '%s' for task '%s'", t.Project, t.Id)
	}
	if len(refs) == 0 {
		grip.Notice(message.Fields{
			"message":    "project ref does not exist for task in dispatch queue",
			"outcome":    "task is not dispatchable",
			"context":    "pod group task dispatcher",
			"task":       t.Id,
			"project":    t.Project,
			"dispatcher": pd.ID,
		})
		return false, nil
	}
	ref := refs[0]

	// Disabled projects generally are not allowed to run tasks. The one
	// exception is that GitHub PR tasks are still allowed to run for disabled
	// hidden projects.
	if !ref.Enabled && (t.Requester != evergreen.GithubPRRequester || !ref.IsHidden()) {
		grip.Notice(message.Fields{
			"message":    "project ref is disabled",
			"outcome":    "task is not dispatchable",
			"context":    "pod group task dispatcher",
			"task":       t.Id,
			"project":    t.Project,
			"dispatcher": pd.ID,
		})
		return false, nil
	}
	if ref.IsDispatchingDisabled() {
		grip.Notice(message.Fields{
			"message":    "project ref has disabled dispatching tasks",
			"outcome":    "task is not dispatchable",
			"context":    "pod group task dispatcher",
			"task":       t.Id,
			"project":    t.Project,
			"dispatcher": pd.ID,
		})
		return false, nil
	}

	return true, nil
}

// RemovePod removes a pod from the dispatcher. If it's the last remaining pod
// in the dispatcher, it removes all tasks from the dispatcher and marks those
// tasks as no longer allocated containers.
func (pd *PodDispatcher) RemovePod(ctx context.Context, env evergreen.Environment, podID string) error {
	if !utility.StringSliceContains(pd.PodIDs, podID) {
		return errors.Errorf("cannot remove pod '%s' from dispatcher because the pod is not associated with the dispatcher", podID)
	}

	var tasksToRemove []string
	if len(pd.PodIDs) == 1 && len(pd.TaskIDs) > 0 {
		// The last pod is about to be removed, so there will be no pod
		// remaining to run the tasks still in the dispatch queue.

		if err := model.MarkUnallocatableContainerTasksSystemFailed(ctx, env.Settings(), pd.TaskIDs); err != nil {
			return errors.Wrap(err, "marking unallocatable container tasks as system-failed")
		}

		if err := task.MarkTasksAsContainerDeallocated(pd.TaskIDs); err != nil {
			return errors.Wrap(err, "marking all tasks in dispatcher as container deallocated")
		}

		tasksToRemove = pd.TaskIDs
	}

	if err := pd.removePodsAndTasks(ctx, env, []string{podID}, tasksToRemove); err != nil {
		return errors.Wrap(err, "removing pod and tasks from dispatcher")
	}

	return nil
}

// removePodsAndTasks removes the given pods and tasks from the dispatcher. If
// a pod or task is given as a parameter but is not present in the dispatcher,
// it is ignored.
func (pd *PodDispatcher) removePodsAndTasks(ctx context.Context, env evergreen.Environment, podIDs []string, taskIDs []string) (err error) {
	oldPodIDs := pd.PodIDs
	oldTaskIDs := pd.TaskIDs
	pd.PodIDs, _ = utility.StringSliceSymmetricDifference(pd.PodIDs, podIDs)

	taskIDsSet := map[string]struct{}{}
	for _, taskID := range taskIDs {
		taskIDsSet[taskID] = struct{}{}
	}
	// Don't use StringSliceSymmetricDifference here, because the order of tasks
	// in the dispatch queue matters and StringSliceSymmetricDifference is not
	// guaranteed to return strings in the same order as they were originally
	// ordered in the slice. Iterate in reverse so that removing elements during
	// iteration doesn't change the cursor position.
	for i := len(pd.TaskIDs) - 1; i >= 0; i-- {
		if _, ok := taskIDsSet[pd.TaskIDs[i]]; !ok {
			continue
		}

		pd.TaskIDs = append(pd.TaskIDs[:i], pd.TaskIDs[i+1:]...)
	}

	defer func() {
		if err != nil {
			pd.PodIDs = oldPodIDs
			pd.TaskIDs = oldTaskIDs
		}
	}()

	lastModified := utility.BSONTime(time.Now())
	res, err := env.DB().Collection(Collection).UpdateOne(ctx, pd.atomicUpsertQuery(), pd.atomicUpsertUpdate(lastModified))
	if err != nil {
		return errors.Wrap(err, "upserting dispatcher")
	}
	if res.ModifiedCount == 0 {
		return errors.New("dispatcher was not updated")
	}

	pd.ModificationCount++
	pd.LastModified = lastModified

	return nil
}

// GetGroupID returns the pod dispatcher group ID for the task.
func GetGroupID(t *task.Task) string {
	// TODO (PM-2618): handle task units that represent task groups rather than
	// standalone tasks.
	return t.Id
}
