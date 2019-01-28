package commitqueue

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const Collection = "commit_queue"

var (
	// bson fields for the CommitQueue struct
	IdKey    = bsonutil.MustHaveTag(CommitQueue{}, "ProjectID")
	QueueKey = bsonutil.MustHaveTag(CommitQueue{}, "Queue")
)

func updateOne(query interface{}, update interface{}) error {
	return db.Update(
		Collection,
		query,
		update,
	)
}

func FindOneId(id string) (*CommitQueue, error) {
	return findOne(db.Query(bson.M{IdKey: id}))
}

func findOne(query db.Q) (*CommitQueue, error) {
	queue := &CommitQueue{}
	err := db.FindOneQ(Collection, query, &queue)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return queue, err
}

func insert(q *CommitQueue) error {
	return db.Insert(Collection, q)
}

func add(id string, queue []string, item string) error {
	err := updateOne(
		bson.M{
			IdKey:    id,
			QueueKey: queue,
		},
		bson.M{"$push": bson.M{QueueKey: item}},
	)

	if err == mgo.ErrNotFound {
		return errors.New("queue has changed in the database")
	}
	return err
}

func remove(id string, item string) error {
	return updateOne(
		bson.M{IdKey: id},
		bson.M{"$pull": bson.M{QueueKey: item}},
	)
}

func removeAll(id string) error {
	return updateOne(
		bson.M{IdKey: id},
		bson.M{"$set": bson.M{QueueKey: []string{}}},
	)
}
