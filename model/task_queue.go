package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	_ fmt.Stringer = nil
)

const (
	TaskQueuesCollection = "task_queues"
)

// represents the next n tasks to be run on hosts of the distro
type TaskQueue struct {
	Id     bson.ObjectId   `bson:"_id,omitempty" json:"_id"`
	Distro string          `bson:"distro" json:"distro"`
	Queue  []TaskQueueItem `bson:"queue" json:"queue"`
}

type TaskDep struct {
	Id          string `bson:"task_id,omitempty" json:"task_id"`
	DisplayName string `bson:"display_name" json:"display_name"`
}
type TaskQueueItem struct {
	Id                  string        `bson:"_id" json:"_id"`
	DisplayName         string        `bson:"display_name" json:"display_name"`
	BuildVariant        string        `bson:"build_variant" json:"build_variant"`
	RevisionOrderNumber int           `bson:"order" json:"order"`
	Requester           string        `bson:"requester" json:"requester"`
	Revision            string        `bson:"gitspec" json:"gitspec"`
	Project             string        `bson:"project" json:"project"`
	ExpectedDuration    time.Duration `bson:"exp_dur" json:"exp_dur"`
	Priority            int64         `bson:"priority" json:"priority"`
}

var (
	// bson fields for the task queue struct
	TaskQueueIdKey     = bsonutil.MustHaveTag(TaskQueue{}, "Id")
	TaskQueueDistroKey = bsonutil.MustHaveTag(TaskQueue{}, "Distro")
	TaskQueueQueueKey  = bsonutil.MustHaveTag(TaskQueue{}, "Queue")

	// bson fields for the individual task queue items
	TaskQueueItemIdKey          = bsonutil.MustHaveTag(TaskQueueItem{}, "Id")
	TaskQueueItemDisplayNameKey = bsonutil.MustHaveTag(TaskQueueItem{},
		"DisplayName")
	TaskQueueItemBuildVariantKey = bsonutil.MustHaveTag(TaskQueueItem{},
		"BuildVariant")
	TaskQueueItemConKey = bsonutil.MustHaveTag(TaskQueueItem{},
		"RevisionOrderNumber")
	TaskQueueItemRequesterKey = bsonutil.MustHaveTag(TaskQueueItem{},
		"Requester")
	TaskQueueItemRevisionKey = bsonutil.MustHaveTag(TaskQueueItem{},
		"Revision")
	TaskQueueItemProjectKey = bsonutil.MustHaveTag(TaskQueueItem{},
		"Project")
	TaskQueueItemExpDurationKey = bsonutil.MustHaveTag(TaskQueueItem{},
		"ExpectedDuration")
	TaskQueuePriorityKey = bsonutil.MustHaveTag(TaskQueueItem{},
		"Priority")
)

func (self *TaskQueue) Length() int {
	return len(self.Queue)
}

func (self *TaskQueue) IsEmpty() bool {
	return len(self.Queue) == 0
}

func (self *TaskQueue) NextTask() TaskQueueItem {
	return self.Queue[0]
}

func (self *TaskQueue) Save() error {
	return UpdateTaskQueue(self.Distro, self.Queue)
}

func UpdateTaskQueue(distro string, taskQueue []TaskQueueItem) error {
	_, err := db.Upsert(
		TaskQueuesCollection,
		bson.M{
			TaskQueueDistroKey: distro,
		},
		bson.M{
			"$set": bson.M{
				TaskQueueQueueKey: taskQueue,
			},
		},
	)
	return err
}

func FindTaskQueueForDistro(distroId string) (*TaskQueue, error) {
	taskQueue := &TaskQueue{}
	err := db.FindOne(
		TaskQueuesCollection,
		bson.M{
			TaskQueueDistroKey: distroId,
		},
		db.NoProjection,
		db.NoSort,
		taskQueue,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return taskQueue, err
}

// FindMinimumQueuePositionForTask finds the position of a task in the many task queues
// where its position is the lowest. It returns an error if the aggregation it runs fails.
func FindMinimumQueuePositionForTask(taskId string) (int, error) {
	var results []struct {
		Index int `bson:"index"`
	}
	queueItemIdKey := fmt.Sprintf("%v.%v", TaskQueueQueueKey, TaskQueueItemIdKey)

	// NOTE: this aggregation requires 3.2+ because of its use of
	// $unwind's 'path' and 'includeArrayIndex'
	pipeline := []bson.M{
		{"$match": bson.M{
			queueItemIdKey: taskId}},
		{"$unwind": bson.M{
			"path":              fmt.Sprintf("$%", TaskQueueQueueKey),
			"includeArrayIndex": "index"}},
		{"$match": bson.M{
			queueItemIdKey: taskId}},
		{"$sort": bson.M{
			"index": 1}},
		{"$limit": 1},
	}

	err := db.Aggregate(TaskQueuesCollection, pipeline, &results)

	if len(results) == 0 {
		return -1, err
	}

	return (results[0].Index + 1), err
}

func FindAllTaskQueues() ([]TaskQueue, error) {
	taskQueues := []TaskQueue{}
	err := db.FindAll(
		TaskQueuesCollection,
		bson.M{},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
		&taskQueues,
	)
	return taskQueues, err
}

// pull out the task with the specified id from both the in-memory and db
// versions of the task queue
func (self *TaskQueue) DequeueTask(taskId string) error {

	// first, remove from the in-memory queue
	found := false
	for idx, queueItem := range self.Queue {
		if queueItem.Id == taskId {
			found = true
			self.Queue = append(self.Queue[:idx], self.Queue[idx+1:]...)
			continue
		}
	}

	// validate that the task is there
	if !found {
		return fmt.Errorf("task id %v was not present in queue for distro"+
			" %v", taskId, self.Distro)
	}

	return db.Update(
		TaskQueuesCollection,
		bson.M{
			TaskQueueDistroKey: self.Distro,
		},
		bson.M{
			"$pull": bson.M{
				TaskQueueQueueKey: bson.M{
					TaskQueueItemIdKey: taskId,
				},
			},
		},
	)
}
