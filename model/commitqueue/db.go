package commitqueue

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const Collection = "commit_queue"

var (
	// bson fields for the CommitQueue struct
	IdKey                    = bsonutil.MustHaveTag(CommitQueue{}, "ProjectID")
	QueueKey                 = bsonutil.MustHaveTag(CommitQueue{}, "Queue")
	ProcessingKey            = bsonutil.MustHaveTag(CommitQueue{}, "Processing")
	ProcessingUpdatedTimeKey = bsonutil.MustHaveTag(CommitQueue{}, "ProcessingUpdatedTime")
	IssueKey                 = bsonutil.MustHaveTag(CommitQueueItem{}, "Issue")
	VersionKey               = bsonutil.MustHaveTag(CommitQueueItem{}, "Version")
	EnqueueTimeKey           = bsonutil.MustHaveTag(CommitQueueItem{}, "EnqueueTime")
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
	err := db.FindOneQ(Collection, query, queue)

	return queue, err
}

func insert(q *CommitQueue) error {
	return db.Insert(Collection, q)
}

func add(id string, queue []CommitQueueItem, item CommitQueueItem) error {
	err := updateOne(
		bson.M{
			IdKey: id,
		},
		bson.M{"$push": bson.M{QueueKey: item}},
	)

	if adb.ResultsNotFound(err) {
		grip.Error(errors.Wrapf(err, "update failed for queue '%s', %+v", id, queue))
		return errors.Errorf("update failed for queue '%s', %+v", id, queue)
	}

	return err
}

// assume that the first element in the array is being processed, so move after that
func addToFront(id string, queue []CommitQueueItem, item CommitQueueItem) error {
	err := updateOne(
		bson.M{
			IdKey: id,
		},
		bson.M{"$push": bson.M{
			QueueKey: bson.M{
				"$each":     []CommitQueueItem{item},
				"$position": 1,
			},
		}},
	)
	if adb.ResultsNotFound(err) {
		grip.Error(errors.Wrapf(err, "force update failed for queue '%s', %+v", id, queue))
		return errors.Errorf("force update failed for queue '%s', %+v", id, queue)
	}
	return err
}

func addVersionID(id string, item CommitQueueItem) error {
	return updateOne(
		bson.M{
			IdKey: id,
			bsonutil.GetDottedKeyName(QueueKey, IssueKey): item.Issue,
		},
		bson.M{
			"$set": bson.M{bsonutil.GetDottedKeyName(QueueKey, "$", VersionKey): item.Version},
		})
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
		bson.M{
			"$set": bson.M{
				ProcessingKey:            status,
				ProcessingUpdatedTimeKey: time.Now(),
			},
		},
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
