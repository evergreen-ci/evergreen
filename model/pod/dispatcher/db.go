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
)

// FindOne finds one pod dispatcher for the given query.
func FindOne(q db.Q) (*PodDispatcher, error) {
	var pd PodDispatcher
	err := db.FindOneQ(Collection, q, &pd)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &pd, err
}

// Find finds all pod dispatchers for the given query.
func Find(q db.Q) ([]PodDispatcher, error) {
	pds := []PodDispatcher{}
	return pds, errors.WithStack(db.FindAllQ(Collection, q, &pds))
}

// UpdateOne updates one pod dispatcher.
func UpdateOne(query bson.M, update interface{}) error {
	return db.Update(Collection, query, update)
}

// UpsertOne updates an existing pod dispatcher if it exists based on the
// query; otherwise, it inserts a new pod dispatcher.
func UpsertOne(query, update interface{}) (*adb.ChangeInfo, error) {
	return db.Upsert(Collection, query, update)
}

// FindOneByID finds one pod dispatcher by its ID.
func FindOneByID(id string) (*PodDispatcher, error) {
	return FindOne(db.Query(bson.M{
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
func FindOneByGroupID(groupID string) (*PodDispatcher, error) {
	return FindOne(db.Query(ByGroupID(groupID)))
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
	allocateDispatcher := func(sessCtx mongo.SessionContext) (interface{}, error) {
		groupID := GetGroupID(t)
		if err := env.DB().Collection(Collection).FindOne(sessCtx, ByGroupID(groupID)).Decode(pd); err != nil && !adb.ResultsNotFound(err) {
			return nil, errors.Wrap(err, "checking for existing pod dispatcher")
		} else if adb.ResultsNotFound(err) {
			newDispatcher := NewPodDispatcher(groupID, []string{t.Id}, []string{p.ID})
			pd = &newDispatcher
		} else {
			pd.PodIDs = append(pd.PodIDs, p.ID)

			if !utility.StringSliceContains(pd.TaskIDs, t.Id) {
				pd.TaskIDs = append(pd.TaskIDs, t.Id)
			}
		}

		if _, err := env.DB().Collection(Collection).UpdateOne(sessCtx, pd.atomicUpsertQuery(), pd.atomicUpsertUpdate(), options.Update().SetUpsert(true)); err != nil {
			return nil, errors.Wrap(err, "upserting pod dispatcher")
		}
		pd.ModificationCount++

		if err := p.InsertWithSession(sessCtx, env); err != nil {
			return nil, errors.Wrap(err, "inserting new intent pod")
		}

		if err := t.MarkAsContainerAllocated(ctx, env); err != nil {
			return nil, errors.Wrap(err, "marking task as container allocated")
		}

		return nil, nil
	}

	if _, err := session.WithTransaction(ctx, allocateDispatcher); err != nil {
		return nil, errors.Wrap(err, "allocating dispatcher in transaction")
	}

	event.LogTaskContainerAllocated(t.Id, t.Execution, time.Now())

	return pd, nil
}
