package dispatcher

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
)

const Collection = "pod_dispatchers"

var (
	IDKey                = bsonutil.MustHaveTag(PodDispatcher{}, "ID")
	GroupIDKey           = bsonutil.MustHaveTag(PodDispatcher{}, "GroupID")
	PodIDsKey            = bsonutil.MustHaveTag(PodDispatcher{}, "PodIDs")
	TaskIDsKey           = bsonutil.MustHaveTag(PodDispatcher{}, "TaskIDs")
	ModificationCountKey = bsonutil.MustHaveTag(PodDispatcher{}, "ModificationCount")
)

// FindOne finds one dispatcher queue for the given query.
// kim: TODO: test
func FindOne(query bson.M) (*PodDispatcher, error) {
	var pd PodDispatcher
	err := db.FindOneQ(Collection, db.Query(query), &pd)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &pd, err
}

// Find finds all dispatcher queues for the given query.
// kim: TODO: test
func Find(query bson.M) ([]PodDispatcher, error) {
	pds := []PodDispatcher{}
	return pds, errors.WithStack(db.FindAllQ(Collection, db.Query(query), &pds))
}

// FindOneByGroupID finds a pod dispatcher by its group ID.
// kim: TODO: test
func FindOneByGroupID(id string) (*PodDispatcher, error) {
	return FindOne(bson.M{
		GroupIDKey: id,
	})
}

// UpdateOne updates one dispatcher queue.
func UpdateOne(query bson.M, update interface{}) error {
	return db.Update(Collection, query, update)
}

// UpsertOne updates an existing dispatcher queue if it exists based on the
// query; otherwise, it inserts a new dispatcher queue.
func UpsertOne(query, update interface{}) (*adb.ChangeInfo, error) {
	return db.Upsert(Collection, query, update)
}
