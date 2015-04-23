package model

import (
	"10gen.com/mci/db"
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"time"
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

type TaskQueueItem struct {
	Id                  string        `bson:"_id" json:"_id"`
	DisplayName         string        `bson:"display_name" json:"display_name"`
	BuildVariant        string        `bson:"build_variant" json:"build_variant"`
	RevisionOrderNumber int           `bson:"order" json:"order"`
	Requester           string        `bson:"requester" json:"requester"`
	Revision            string        `bson:"gitspec" json:"gitspec"`
	Project             string        `bson:"project" json:"project"`
	ExpectedDuration    time.Duration `bson:"exp_dur" json:"exp_dur"`
}

var (
	// bson fields for the task queue struct
	TaskQueueIdKey     = MustHaveBsonTag(TaskQueue{}, "Id")
	TaskQueueDistroKey = MustHaveBsonTag(TaskQueue{}, "Distro")
	TaskQueueQueueKey  = MustHaveBsonTag(TaskQueue{}, "Queue")

	// bson fields for the individual task queue items
	TaskQueueItemIdKey          = MustHaveBsonTag(TaskQueueItem{}, "Id")
	TaskQueueItemDisplayNameKey = MustHaveBsonTag(TaskQueueItem{},
		"DisplayName")
	TaskQueueItemBuildVariantKey = MustHaveBsonTag(TaskQueueItem{},
		"BuildVariant")
	TaskQueueItemConKey = MustHaveBsonTag(TaskQueueItem{},
		"RevisionOrderNumber")
	TaskQueueItemRequesterKey = MustHaveBsonTag(TaskQueueItem{},
		"Requester")
	TaskQueueItemRevisionKey = MustHaveBsonTag(TaskQueueItem{},
		"Revision")
	TaskQueueItemProjectKey = MustHaveBsonTag(TaskQueueItem{},
		"Project")
	TaskQueueItemExpDurationKey = MustHaveBsonTag(TaskQueueItem{},
		"ExpectedDuration")
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

func FindTaskQueueForDistro(distro string) (*TaskQueue, error) {
	taskQueue := &TaskQueue{}
	err := db.FindOne(
		TaskQueuesCollection,
		bson.M{
			TaskQueueDistroKey: distro,
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
