package commitqueue

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const Collection = "commit_queue"

var (
	// bson fields for the CommitQueue struct
	IdKey                  = bsonutil.MustHaveTag(CommitQueue{}, "ProjectID")
	QueueKey               = bsonutil.MustHaveTag(CommitQueue{}, "Queue")
	IssueKey               = bsonutil.MustHaveTag(CommitQueueItem{}, "Issue")
	VersionKey             = bsonutil.MustHaveTag(CommitQueueItem{}, "Version")
	EnqueueTimeKey         = bsonutil.MustHaveTag(CommitQueueItem{}, "EnqueueTime")
	ProcessingStartTimeKey = bsonutil.MustHaveTag(CommitQueueItem{}, "ProcessingStartTime")
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
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding queue by ID")
	}
	return queue, err
}

func insert(q *CommitQueue) error {
	return db.Insert(Collection, q)
}

func add(id string, item CommitQueueItem) error {
	err := updateOne(
		bson.M{
			IdKey: id,
		},
		bson.M{"$push": bson.M{QueueKey: item}},
	)

	if adb.ResultsNotFound(err) {
		return errors.Wrap(err, "adding item to queue")
	}

	return err
}

func addAtPosition(id string, item CommitQueueItem, pos int) error {
	err := updateOne(
		bson.M{
			IdKey: id,
		},
		bson.M{"$push": bson.M{
			QueueKey: bson.M{
				"$each":     []CommitQueueItem{item},
				"$position": pos,
			},
		}},
	)
	if adb.ResultsNotFound(err) {
		return errors.Wrapf(err, "adding item to queue at position %d", pos)
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
			"$set": bson.M{
				bsonutil.GetDottedKeyName(QueueKey, "$", VersionKey):             item.Version,
				bsonutil.GetDottedKeyName(QueueKey, "$", ProcessingStartTimeKey): time.Now(),
			},
		})
}

// remove removes a given item from a project's commit queue. Make sure to pass the actual
// issue identifier and not the patch or version
func remove(project, issue string) error {
	return updateOne(
		bson.M{IdKey: project},
		bson.M{"$pull": bson.M{QueueKey: bson.M{IssueKey: issue}}},
	)
}

func clearAll() (int, error) {
	return updateAll(
		struct{}{},
		bson.M{
			"$unset": bson.M{QueueKey: 1},
		},
	)
}
