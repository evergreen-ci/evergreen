package commitqueue

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const Collection = "commit_queue"

var (
	// bson fields for the CommitQueue struct
	IdKey         = bsonutil.MustHaveTag(CommitQueue{}, "ProjectID")
	QueueKey      = bsonutil.MustHaveTag(CommitQueue{}, "Queue")
	ProcessingKey = bsonutil.MustHaveTag(CommitQueue{}, "Processing")
	IssueKey      = bsonutil.MustHaveTag(CommitQueueItem{}, "Issue")
)

func updateOne(query interface{}, update interface{}) error {
	return db.Update(
		Collection,
		query,
		update,
	)
}

func updateAll(query interface{}, update interface{}) (int, error) {
	results, err := db.UpdateAll(Collection, query, update)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return results.Updated, err
}

func FindOneId(id string) (*CommitQueue, error) {
	return findOne(db.Query(bson.M{IdKey: id}))
}

func findOne(query db.Q) (*CommitQueue, error) {
	queue := &CommitQueue{}
	err := db.FindOneQ(Collection, query, &queue)

	return queue, err
}

func insert(q *CommitQueue) error {
	return db.Insert(Collection, q)
}

func add(id string, queue []CommitQueueItem, item CommitQueueItem) error {
	err := updateOne(
		bson.M{
			IdKey:    id,
			QueueKey: queue,
		},
		bson.M{"$push": bson.M{QueueKey: item}},
	)

	if adb.ResultsNotFound(err) {
		return errors.New("queue has changed in the database")
	}

	return err
}

func remove(id, issue string) error {
	return updateOne(
		bson.M{IdKey: id},
		bson.M{"$pull": bson.M{QueueKey: bson.M{IssueKey: issue}}},
	)
}

func setProcessing(id string, status bool) error {
	return updateOne(
		bson.M{IdKey: id},
		bson.M{"$set": bson.M{ProcessingKey: status}},
	)
}

func clearAll() (int, error) {
	return updateAll(
		struct{}{},
		bson.M{
			"$unset": bson.M{QueueKey: 1},
			"$set":   bson.M{ProcessingKey: false},
		},
	)
}
