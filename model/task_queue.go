package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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
	Group               string        `bson:"group_name" json:"group_name"`
	Version             string        `bson:"version" json:"version"`
	BuildVariant        string        `bson:"build_variant" json:"build_variant"`
	RevisionOrderNumber int           `bson:"order" json:"order"`
	Requester           string        `bson:"requester" json:"requester"`
	Revision            string        `bson:"gitspec" json:"gitspec"`
	Project             string        `bson:"project" json:"project"`
	ExpectedDuration    time.Duration `bson:"exp_dur" json:"exp_dur"`
	Priority            int64         `bson:"priority" json:"priority"`
}

// nolint
var (
	// bson fields for the task queue struct
	taskQueueIdKey     = bsonutil.MustHaveTag(TaskQueue{}, "Id")
	taskQueueDistroKey = bsonutil.MustHaveTag(TaskQueue{}, "Distro")
	taskQueueQueueKey  = bsonutil.MustHaveTag(TaskQueue{}, "Queue")

	// bson fields for the individual task queue items
	taskQueueItemIdKey           = bsonutil.MustHaveTag(TaskQueueItem{}, "Id")
	taskQueueItemDisplayNameKey  = bsonutil.MustHaveTag(TaskQueueItem{}, "DisplayName")
	taskQueueItemBuildVariantKey = bsonutil.MustHaveTag(TaskQueueItem{}, "BuildVariant")
	taskQueueItemConKey          = bsonutil.MustHaveTag(TaskQueueItem{}, "RevisionOrderNumber")
	taskQueueItemRequesterKey    = bsonutil.MustHaveTag(TaskQueueItem{}, "Requester")
	taskQueueItemRevisionKey     = bsonutil.MustHaveTag(TaskQueueItem{}, "Revision")
	taskQueueItemProjectKey      = bsonutil.MustHaveTag(TaskQueueItem{}, "Project")
	taskQueueItemExpDurationKey  = bsonutil.MustHaveTag(TaskQueueItem{}, "ExpectedDuration")
	taskQueuePriorityKey         = bsonutil.MustHaveTag(TaskQueueItem{}, "Priority")
)

func NewTaskQueue(distro string, queue []TaskQueueItem) TaskQueueAccessor {
	return &TaskQueue{
		Distro: distro,
		Queue:  queue,
	}
}

func LoadTaskQueue(distro string) (TaskQueueAccessor, error) {
	return findTaskQueueForDistro(distro)
}

func (self *TaskQueue) Length() int {
	return len(self.Queue)
}

func (self *TaskQueue) NextTask() *TaskQueueItem {
	return &self.Queue[0]
}

func (self *TaskQueue) Save() error {
	return updateTaskQueue(self.Distro, self.Queue)
}

func (self *TaskQueue) FindTask(spec TaskSpec) *TaskQueueItem {
	if spec.Group == "" || spec.ProjectID == "" || spec.BuildVariant == "" || spec.Version == "" {
		return nil
	}

	for _, it := range self.Queue {
		if it.Project != spec.ProjectID {
			continue
		}

		if it.Version != spec.Version {
			continue
		}

		if it.BuildVariant != spec.BuildVariant {
			continue
		}

		if it.Group != spec.Group {
			continue
		}

		return &it
	}

	// TODO decide if we should return NextTask or nil
	return nil
}

func updateTaskQueue(distro string, taskQueue []TaskQueueItem) error {
	_, err := db.Upsert(
		TaskQueuesCollection,
		bson.M{
			taskQueueDistroKey: distro,
		},
		bson.M{
			"$set": bson.M{
				taskQueueQueueKey: taskQueue,
			},
		},
	)
	return errors.WithStack(err)
}

func findTaskQueueForDistro(distroId string) (*TaskQueue, error) {
	taskQueue := &TaskQueue{}
	err := db.FindOne(
		TaskQueuesCollection,
		bson.M{
			taskQueueDistroKey: distroId,
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

	queueItemIdKey := bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemIdKey)

	// NOTE: this aggregation requires 3.2+ because of its use of
	// $unwind's 'path' and 'includeArrayIndex'
	pipeline := []bson.M{
		{"$match": bson.M{
			queueItemIdKey: taskId}},
		{"$unwind": bson.M{
			"path":              "$" + taskQueueQueueKey,
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
		return errors.Errorf("task id %s was not present in queue for distro %s",
			taskId, self.Distro)
	}

	return errors.WithStack(db.Update(
		TaskQueuesCollection,
		bson.M{
			taskQueueDistroKey: self.Distro,
		},
		bson.M{
			"$pull": bson.M{
				taskQueueQueueKey: bson.M{
					taskQueueItemIdKey: taskId,
				},
			},
		},
	))
}
