package dispatcher

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const Collection = "pod_dispatchers"

var (
	IDKey                = bsonutil.MustHaveTag(PodDispatcher{}, "ID")
	GroupIDKey           = bsonutil.MustHaveTag(PodDispatcher{}, "GroupID")
	PodIDsKey            = bsonutil.MustHaveTag(PodDispatcher{}, "PodIDs")
	TaskIDsKey           = bsonutil.MustHaveTag(PodDispatcher{}, "TaskIDs")
	ModificationCountKey = bsonutil.MustHaveTag(PodDispatcher{}, "ModificationCount")
	LastModifiedKey      = bsonutil.MustHaveTag(PodDispatcher{}, "LastModified")
)

// FindOne finds one pod dispatcher for the given query.
func FindOne(ctx context.Context, q db.Q) (*PodDispatcher, error) {
	var pd PodDispatcher
	err := db.FindOneQ(ctx, Collection, q, &pd)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &pd, err
}

// Find finds all pod dispatchers for the given query.
func Find(ctx context.Context, q db.Q) ([]PodDispatcher, error) {
	pds := []PodDispatcher{}
	return pds, errors.WithStack(db.FindAllQ(ctx, Collection, q, &pds))
}

// UpsertOne updates an existing pod dispatcher if it exists based on the
// query; otherwise, it inserts a new pod dispatcher.
func UpsertOne(ctx context.Context, query, update any) (*adb.ChangeInfo, error) {
	return db.Upsert(ctx, Collection, query, update)
}

// FindOneByID finds one pod dispatcher by its ID.
func FindOneByID(ctx context.Context, id string) (*PodDispatcher, error) {
	return FindOne(ctx, db.Query(bson.M{
		IDKey: id,
	}))
}

// ByGroupID returns the query to find a pod dispatcher by its group ID.
func ByGroupID(groupID string) bson.M {
	return bson.M{
		GroupIDKey: groupID,
	}
}

// FindOneByGroupID finds one pod dispatcher by its group ID.
func FindOneByGroupID(ctx context.Context, groupID string) (*PodDispatcher, error) {
	return FindOne(ctx, db.Query(ByGroupID(groupID)))
}

// FindOneByPodID finds the dispatcher that manages the given pod by ID.
func FindOneByPodID(ctx context.Context, podID string) (*PodDispatcher, error) {
	return FindOne(ctx, db.Query(byPodID(podID)))
}

func byPodID(podID string) bson.M {
	return bson.M{
		PodIDsKey: podID,
	}
}

// Allocate sets up the given intent pod to the given task for dispatching.
func Allocate(ctx context.Context, env evergreen.Environment, t *task.Task, p *pod.Pod) (*PodDispatcher, error) {
	mongoClient := env.Client()
	session, err := mongoClient.StartSession()
	if err != nil {
		return nil, errors.Wrap(err, "starting transaction session")
	}
	defer session.EndSession(ctx)

	pd := &PodDispatcher{}
	allocateDispatcher := func(ctx mongo.SessionContext) (any, error) {
		groupID := GetGroupID(t)
		if err := env.DB().Collection(Collection).FindOne(ctx, ByGroupID(groupID)).Decode(pd); err != nil && !adb.ResultsNotFound(err) {
			return nil, errors.Wrap(err, "checking for existing pod dispatcher")
		} else if adb.ResultsNotFound(err) {
			newDispatcher := NewPodDispatcher(groupID, []string{t.Id}, []string{p.ID})
			pd = &newDispatcher
		} else {
			if !utility.StringSliceContains(pd.TaskIDs, t.Id) {
				pd.TaskIDs = append(pd.TaskIDs, t.Id)
			}

			if len(pd.TaskIDs) > len(pd.PodIDs) {
				// Avoid allocating another pod if it's not necessary. The
				// number of pods allocated to run the pod dispatchers' tasks
				// should not exceed the number of tasks that need to be run.
				pd.PodIDs = append(pd.PodIDs, p.ID)
			}
		}

		lastModified := utility.BSONTime(time.Now())
		res, err := env.DB().Collection(Collection).UpdateOne(ctx, pd.atomicUpsertQuery(), pd.atomicUpsertUpdate(lastModified), options.Update().SetUpsert(true))
		if err != nil {
			return nil, errors.Wrap(err, "upserting pod dispatcher")
		}
		if res.ModifiedCount == 0 && res.UpsertedCount == 0 {
			// This can occur due to the pod dispatcher being concurrently
			// updated elsewhere (such as when dispatching a task to a pod),
			// which is a transient issue.
			return nil, errors.Errorf("pod dispatcher was not upserted")
		}

		pd.ModificationCount++
		pd.LastModified = lastModified

		if utility.StringSliceContains(pd.PodIDs, p.ID) {
			// A pod will only be allocated if the dispatcher is actually in
			// need of another pod to run its tasks.
			if err := p.Insert(ctx); err != nil {
				return nil, errors.Wrap(err, "inserting new intent pod")
			}
		}

		if err := t.MarkAsContainerAllocated(ctx, env); err != nil {
			return nil, errors.Wrap(err, "marking task as container allocated")
		}

		return nil, nil
	}

	if _, err := session.WithTransaction(ctx, allocateDispatcher); err != nil {
		return nil, errors.Wrap(err, "allocating dispatcher in transaction")
	}

	event.LogTaskContainerAllocated(ctx, t.Id, t.Execution, time.Now())

	return pd, nil
}
