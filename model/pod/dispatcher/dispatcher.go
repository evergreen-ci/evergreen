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
	// kim: NOTE: all DB ops should be atomic and double-check state. Maybe need
	// to use transaction.
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
			// Task does not exist, so just dequeue it.
			if err := pd.pop(ctx, env); err != nil {
				return nil, errors.Wrapf(err, "popping nonexistent task '%s' from dispatch queue", taskID)
			}
			grip.Notice(message.Fields{
				"message":    "task does not exist - removing it from the dispatch queue",
				"outcome":    "dequeueing",
				"context":    "pod group task dispatcher",
				"pod":        p.ID,
				"task":       taskID,
				"dispatcher": pd.ID,
			})
			continue
		}

		if !t.IsContainerDispatchable() {
			grip.Notice(message.Fields{
				"message":    "task in dispatch queue is not dispatchable",
				"outcome":    "dequeueing",
				"context":    "pod group task dispatcher",
				"pod":        p.ID,
				"task":       taskID,
				"dispatcher": pd.ID,
			})
			if err := pd.pop(ctx, env); err != nil {
				return nil, errors.Wrapf(err, "popping undispatchable task '%s' from dispatch queue", taskID)
			}
			continue
		}

		// kim: TODO: Check if task is runnable (status is container allocated,
		// activated, not disabled, dependencies are met) and not disabled by
		// project - if not, dequeue task. If task is not in runnable state
		// (i.e. container-allocated, activated, not disabled, dependencies
		// met), it should be safe to dequeue the task since any
		// container-unallocated task would later allocate until it's finally
		// container-allocated.
		refs, err := model.FindProjectRefsByIds(t.Project)
		if err != nil {
			return nil, errors.Wrapf(err, "finding project ref '%s' for task '%s'", t.Project, t.Id)
		}
		if len(refs) == 0 {
			grip.Notice(message.Fields{
				"message":    "project ref does not exist for task in dispatch queue",
				"outcome":    "dequeueing",
				"context":    "pod group task dispatcher",
				"pod":        p.ID,
				"task":       t.Id,
				"project":    t.Project,
				"dispatcher": pd.ID,
			})
			// Project does not exist, so dequeue the task.
			if err := pd.pop(ctx, env); err != nil {
				return nil, errors.Wrapf(err, "popping task '%s' with nonexistent project ref '%s'", t.Id, t.Project)
			}
			continue
		}
		ref := refs[0]

		// Disabled projects generally are not allowed to run tasks. The one
		// exception is that GitHub PR tasks are still allowed to run for
		// disabled hidden projects.
		if !ref.IsEnabled() && (t.Requester != evergreen.GithubPRRequester || !ref.IsHidden()) {
			return nil, errors.Wrapf(err, "popping task '%s' when its project ref '%s' is disabled", t.Id, ref.Id)
		}
		if ref.IsDispatchingDisabled() {
			if err := pd.pop(ctx, env); err != nil {
				return nil, errors.Wrapf(err, "popping task '%s' when its project ref '%s' has disabled dispatching", t.Id, ref.Id)
			}
			continue
		}

		session, err := env.Client().StartSession()
		if err != nil {
			return nil, errors.Wrap(err, "starting transaction session")
		}
		defer session.EndSession(ctx)

		dispatchTaskToPod := func(sesCtx mongo.SessionContext) (interface{}, error) {
			if err := p.SetRunningTask(ctx, env, t.Id); err != nil {
				return nil, errors.Wrap(err, "setting pod's running task")
			}

			if err := t.MarkAsContainerDispatched(ctx, env); err != nil {
				return nil, errors.Wrap(err, "marking task as dispatched")
			}

			if err := pd.pop(ctx, env); err != nil {
				return nil, errors.Wrap(err, "popping task from dispatch queue")
			}

			return nil, nil
		}

		if _, err := session.WithTransaction(ctx, dispatchTaskToPod); err != nil {
			return nil, errors.Wrapf(err, "dispatching task '%s' to pod '%s'", t.Id, p.ID)
		}

		event.LogPodAssignedTask(p.ID, taskID)

		return t, nil
	}

	return nil, nil
}

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

// GetGroupID returns the pod dispatcher group ID for the task.
func GetGroupID(t *task.Task) string {
	// TODO (PM-2618): handle task units that represent task groups rather than
	// standalone tasks.
	return t.Id
}
