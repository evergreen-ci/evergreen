package dispatcher

import (
	"context"

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

// AssignNextTask assigns the given pod the next available task to run.
// kim: TODO: test
func (pd *PodDispatcher) AssignNextTask(p *pod.Pod) (*task.Task, error) {
	env := evergreen.GetEnvironment()
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
			if err := pd.pop(ctx, env); err != nil {
				return nil, errors.Wrapf(err, "popping nonexistent task '%s' from dispatch queue", taskID)
			}
			continue
		}

		shouldDispatch, err := pd.checkTaskShouldDispatch(ctx, env, t)
		if err != nil {
			return nil, errors.Wrap(err, "checking task runnability based on project ref")
		}
		if !shouldDispatch {
			if err := pd.pop(ctx, env); err != nil {
				return nil, errors.Wrapf(err, "popping undispatchable task '%s'", taskID)
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

		event.LogPodAssignedTask(p.ID, taskID)

		return t, nil
	}

	return nil, nil
}

// dispatchTask performs the DB updates to assign a task to run on a pod.
func (pd *PodDispatcher) dispatchTask(env evergreen.Environment, p *pod.Pod, t *task.Task) func(mongo.SessionContext) (interface{}, error) {
	return func(sessCtx mongo.SessionContext) (interface{}, error) {
		if err := p.SetRunningTask(sessCtx, env, t.Id); err != nil {
			return nil, errors.Wrapf(err, "setting pod's running task")
		}

		if err := t.MarkAsContainerDispatched(sessCtx, env); err != nil {
			return nil, errors.Wrapf(err, "marking task as dispatched")
		}

		if err := pd.pop(sessCtx, env); err != nil {
			return nil, errors.Wrapf(err, "popping task from dispatch queue")
		}

		return nil, nil
	}
}

// pop removes the head of the task dispatch queue.
// kim: TODO: test
func (pd *PodDispatcher) pop(ctx context.Context, env evergreen.Environment) (err error) {
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
	return nil
}

// checkTaskShouldDispatch checks if a task is allowed to dispatch based on its
// current state and its project ref's settings.
func (pd *PodDispatcher) checkTaskShouldDispatch(ctx context.Context, env evergreen.Environment, t *task.Task) (shouldRun bool, err error) {
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
	// exception is that GitHub PR tasks are still allowed to run for
	// disabled hidden projects.
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
