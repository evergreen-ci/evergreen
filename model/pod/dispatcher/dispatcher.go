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

func (pd *PodDispatcher) atomicUpsertUpdate() bson.M {
	return bson.M{
		"$setOnInsert": bson.M{
			IDKey:      pd.ID,
			GroupIDKey: pd.GroupID,
		},
		"$set": bson.M{
			PodIDsKey:  pd.PodIDs,
			TaskIDsKey: pd.TaskIDs,
		},
		"$inc": bson.M{
			ModificationCountKey: 1,
		},
	}
}

// UpsertAtomically inserts/updates the pod dispatcher depending on whether the
// document already exists.
func (pd *PodDispatcher) UpsertAtomically() (*adb.ChangeInfo, error) {
	change, err := UpsertOne(pd.atomicUpsertQuery(), pd.atomicUpsertUpdate())
	if err != nil {
		return change, err
	}
	pd.ModificationCount++
	return change, nil
}

// AssignNextTask assigns the pod the next available task to run. Returns nil if
// there's no task available to run.
func (pd *PodDispatcher) AssignNextTask(env evergreen.Environment, p *pod.Pod) (*task.Task, error) {
	ctx, cancel := env.Context()
	defer cancel()

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

		session, err := env.Client().StartSession()
		if err != nil {
			return nil, errors.Wrap(err, "starting transaction session")
		}
		defer session.EndSession(ctx)

		if _, err := session.WithTransaction(ctx, pd.dispatchTask(env, p, t)); err != nil {
			return nil, errors.Wrapf(err, "dispatching task '%s' to pod '%s'", t.Id, p.ID)
		}

		event.LogPodAssignedTask(p.ID, t.Id)
		event.LogContainerTaskDispatched(t.Id, t.Execution, p.ID)

		return t, nil
	}

	return nil, nil
}

// dispatchTask performs the DB updates to atomically assign a task to run on a
// pod.
func (pd *PodDispatcher) dispatchTask(env evergreen.Environment, p *pod.Pod, t *task.Task) func(mongo.SessionContext) (interface{}, error) {
	return func(sessCtx mongo.SessionContext) (interface{}, error) {
		if err := p.SetRunningTask(sessCtx, env, t.Id); err != nil {
			return nil, errors.Wrapf(err, "setting pod's running task")
		}

		if err := t.MarkAsContainerDispatched(sessCtx, env, p.AgentVersion, time.Now()); err != nil {
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
	res, err := env.DB().Collection(Collection).UpdateOne(ctx, pd.atomicUpsertQuery(), pd.atomicUpsertUpdate())
	if err != nil {
		return errors.Wrap(err, "upserting dispatcher")
	}
	if res.ModifiedCount == 0 {
		return errors.New("dispatcher was not updated")
	}

	pd.ModificationCount++

	return nil
}

// dequeueUndispatchableTask removes the task from the dispatcher and, if the
// task status indicates a container has been allocated for it, marks it as
// unallocated.
func (pd *PodDispatcher) dequeueUndispatchableTask(ctx context.Context, env evergreen.Environment, t *task.Task) error {
	if t.Status != evergreen.TaskContainerAllocated {
		return errors.Wrap(pd.dequeue(ctx, env), "dequeueing task")
	}

	if err := t.MarkAsContainerUnallocated(ctx, env); err != nil {
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
			"message":    "task in dispatch queue is not dispatchable",
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
	if !ref.IsEnabled() && (t.Requester != evergreen.GithubPRRequester || !ref.IsHidden()) {
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

// GetGroupID returns the pod dispatcher group ID for the task.
func GetGroupID(t *task.Task) string {
	// TODO (PM-2618): handle task units that represent task groups rather than
	// standalone tasks.
	return t.Id
}
