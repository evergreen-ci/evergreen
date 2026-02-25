package model

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	TaskQueuesCollection          = "task_queues"
	TaskSecondaryQueuesCollection = "task_alias_queues"
)

type TaskGroupInfo struct {
	// Name is the name of the task group. An empty string denotes that it is a standalone task
	Name string `bson:"name" json:"name"`
	// Count is the number of tasks in the group that are currently in the queue.
	Count int `bson:"count" json:"count"`
	// CountFree represents the number of hosts that are free (or expected to soon be free) to run tasks in the group
	CountFree int `bson:"count_free" json:"count_free"`
	// CountRequired represents the number of new hosts that will be spawned to run tasks in the group
	CountRequired int `bson:"count_required" json:"count_required"`
	// MaxHosts represents the maximum number of hosts that can be allocated to the group at once
	MaxHosts int `bson:"max_hosts" json:"max_hosts"`
	// ExpectedDuration represents the sum of the expected runtime of all tasks waiting in the group with their dependencies met
	ExpectedDuration time.Duration `bson:"expected_duration" json:"expected_duration"`
	// CountDurationOverThreshold represents the number of tasks in the group that have their dependencies
	// met and are expected to take over the distro queue's MaxDurationThreshold
	CountDurationOverThreshold int `bson:"count_over_threshold" json:"count_over_threshold"`
	// CountWaitOverThreshold represents the number of tasks in the group that have been waiting for over the distro queue's MaxDurationThreshold
	// since their dependencies were met
	CountWaitOverThreshold int `bson:"count_wait_over_threshold" json:"count_wait_over_threshold"`
	// CountDepFilledMergeQueueTasks represents the number of merge queue tasks that are in the queue.
	CountDepFilledMergeQueueTasks int `bson:"count_merge_tasks" json:"count_merge_tasks"`
	// DurationOverThreshold represents the sum of the expected durations of tasks in the group
	// that have their dependencies met and are expected to take over the distro queue's MaxDurationThreshold
	DurationOverThreshold time.Duration `bson:"duration_over_threshold" json:"duration_over_threshold"`
}

type DistroQueueInfo struct {
	// Length represents the number of tasks waiting in the queue
	Length int `bson:"length" json:"length"`
	// LengthWithDependenciesMet represents the number of tasks waiting in the queue with their dependencies met
	LengthWithDependenciesMet int `bson:"length_with_dependencies_met" json:"length_with_dependencies_met"`
	// CountDepFilledMergeQueueTasks represents the number of merge queue tasks that are in the queue.
	CountDepFilledMergeQueueTasks int `bson:"count_merge_tasks" json:"count_merge_tasks"`
	// ExpectedDuration represents the sum of the expected runtime of all tasks waiting in the queue with their dependencies met
	ExpectedDuration time.Duration `bson:"expected_duration" json:"expected_duration"`
	// MaxDurationThreshold is the target length of time the host allocator aims to complete dependency-fulfilled tasks in the queue
	// when determining how many hosts to spawn
	MaxDurationThreshold time.Duration `bson:"max_duration_threshold" json:"max_duration_threshold"`
	// PlanCreatedAt represents the timestamp at which the queue plan was initialized
	PlanCreatedAt time.Time `bson:"created_at" json:"created_at"`
	// CountDurationOverThreshold represents the number of tasks that have their dependencies met and are expected to take over MaxDurationThreshold
	CountDurationOverThreshold int `bson:"count_over_threshold" json:"count_over_threshold"`
	// DurationOverThreshold represents the sum of the expected durations of all tasks that have their dependencies met
	// and are expected to take over MaxDurationThreshold
	DurationOverThreshold time.Duration `bson:"duration_over_threshold" json:"duration_over_threshold"`
	// CountWaitOverThreshold represents the number of tasks that have been waiting the MaxDurationThreshold since their dependencies were met
	CountWaitOverThreshold int `bson:"count_wait_over_threshold" json:"count_wait_over_threshold"`
	// TaskGroupInfos is a list of info that contains the same information as in this struct, but granularized to be only for tasks in
	// a specific group (standalone tasks are included as well, denoted by an empty string for the group name)
	TaskGroupInfos []TaskGroupInfo `bson:"task_group_infos" json:"task_group_infos"`
	// SecondaryQueue refers to whether or not this info refers to a secondary queue.
	// Tags don't match due to outdated naming convention.
	SecondaryQueue bool `bson:"alias_queue" json:"alias_queue"`
}

func GetDistroQueueInfo(ctx context.Context, distroID string) (DistroQueueInfo, error) {
	rval, err := getDistroQueueInfoCollection(ctx, distroID, TaskQueuesCollection)
	return rval, err
}

func GetDistroSecondaryQueueInfo(ctx context.Context, distroID string) (DistroQueueInfo, error) {
	rval, err := getDistroQueueInfoCollection(ctx, distroID, TaskSecondaryQueuesCollection)
	return rval, err
}

func getDistroQueueInfoCollection(ctx context.Context, distroID, collection string) (DistroQueueInfo, error) {
	taskQueue := &TaskQueue{}
	q := db.Query(bson.M{taskQueueDistroKey: distroID}).Project(bson.M{taskQueueDistroQueueInfoKey: 1})
	err := db.FindOneQ(ctx, collection, q, taskQueue)

	if err != nil {
		return DistroQueueInfo{}, errors.Wrapf(err, "finding distro queue info for distro '%s'", distroID)
	}

	return taskQueue.DistroQueueInfo, nil
}

func RemoveTaskQueues(ctx context.Context, distroID string) error {
	query := db.Query(bson.M{taskQueueDistroKey: distroID})
	catcher := grip.NewBasicCatcher()
	err := db.RemoveAllQ(ctx, TaskQueuesCollection, query)
	catcher.AddWhen(!adb.ResultsNotFound(err), errors.Wrapf(err, "removing task queue for distro '%s'", distroID))
	err = db.RemoveAllQ(ctx, TaskSecondaryQueuesCollection, query)
	catcher.AddWhen(!adb.ResultsNotFound(err), errors.Wrapf(err, "removing task alias queue for distro '%s'", distroID))
	return catcher.Resolve()
}

// GetQueueCollection returns the collection associated with this queue.
func (q *DistroQueueInfo) GetQueueCollection() string {
	if q.SecondaryQueue {
		return TaskSecondaryQueuesCollection
	}

	return TaskQueuesCollection
}

// TaskQueue represents the next n tasks to be run on hosts of the distro
type TaskQueue struct {
	Distro          string          `bson:"distro" json:"distro"`
	GeneratedAt     time.Time       `bson:"generated_at" json:"generated_at"`
	Queue           []TaskQueueItem `bson:"queue" json:"queue"`
	DistroQueueInfo DistroQueueInfo `bson:"distro_queue_info" json:"distro_queue_info"`
}

type TaskDep struct {
	Id          string `bson:"task_id,omitempty" json:"task_id"`
	DisplayName string `bson:"display_name" json:"display_name"`
}

type TaskQueueItem struct {
	Id                    string                     `bson:"_id" json:"_id"`
	IsDispatched          bool                       `bson:"dispatched" json:"dispatched"`
	DisplayName           string                     `bson:"display_name" json:"display_name"`
	Group                 string                     `bson:"group_name" json:"group_name"`
	GroupMaxHosts         int                        `bson:"group_max_hosts,omitempty" json:"group_max_hosts,omitempty"`
	GroupIndex            int                        `bson:"group_index,omitempty" json:"group_index,omitempty"`
	Version               string                     `bson:"version" json:"version"`
	BuildVariant          string                     `bson:"build_variant" json:"build_variant"`
	RevisionOrderNumber   int                        `bson:"order" json:"order"`
	Requester             string                     `bson:"requester" json:"requester"`
	Revision              string                     `bson:"gitspec" json:"gitspec"`
	Project               string                     `bson:"project" json:"project"`
	ExpectedDuration      time.Duration              `bson:"exp_dur" json:"exp_dur"`
	Priority              int64                      `bson:"priority" json:"priority"`
	SortingValueBreakdown task.SortingValueBreakdown `bson:"sorting_value_breakdown" json:"sorting_value_breakdown"`
	Dependencies          []string                   `bson:"dependencies" json:"dependencies"`
	DependenciesMet       bool                       `bson:"dependencies_met" json:"dependencies_met"`
	ActivatedBy           string                     `bson:"activated_by" json:"activated_by"`
}

// must not no-lint these values
var (
	// bson fields for the task queue struct
	taskQueueDistroKey          = bsonutil.MustHaveTag(TaskQueue{}, "Distro")
	taskQueueGeneratedAtKey     = bsonutil.MustHaveTag(TaskQueue{}, "GeneratedAt")
	taskQueueQueueKey           = bsonutil.MustHaveTag(TaskQueue{}, "Queue")
	taskQueueDistroQueueInfoKey = bsonutil.MustHaveTag(TaskQueue{}, "DistroQueueInfo")

	// bson fields for the individual task queue items
	taskQueueItemIdKey            = bsonutil.MustHaveTag(TaskQueueItem{}, "Id")
	taskQueueItemIsDispatchedKey  = bsonutil.MustHaveTag(TaskQueueItem{}, "IsDispatched")
	taskQueueItemGroupIndexKey    = bsonutil.MustHaveTag(TaskQueueItem{}, "GroupIndex")
	taskQueueItemDisplayNameKey   = bsonutil.MustHaveTag(TaskQueueItem{}, "DisplayName")
	taskQueueItemGroupKey         = bsonutil.MustHaveTag(TaskQueueItem{}, "Group")
	taskQueueItemGroupMaxHostsKey = bsonutil.MustHaveTag(TaskQueueItem{}, "GroupMaxHosts")
	taskQueueItemVersionKey       = bsonutil.MustHaveTag(TaskQueueItem{}, "Version")
	taskQueueItemBuildVariantKey  = bsonutil.MustHaveTag(TaskQueueItem{}, "BuildVariant")
	taskQueueItemConKey           = bsonutil.MustHaveTag(TaskQueueItem{}, "RevisionOrderNumber")
	taskQueueItemRequesterKey     = bsonutil.MustHaveTag(TaskQueueItem{}, "Requester")
	taskQueueItemRevisionKey      = bsonutil.MustHaveTag(TaskQueueItem{}, "Revision")
	taskQueueItemProjectKey       = bsonutil.MustHaveTag(TaskQueueItem{}, "Project")
	taskQueueItemExpDurationKey   = bsonutil.MustHaveTag(TaskQueueItem{}, "ExpectedDuration")
	taskQueueItemPriorityKey      = bsonutil.MustHaveTag(TaskQueueItem{}, "Priority")
	taskQueueItemActivatedByKey   = bsonutil.MustHaveTag(TaskQueueItem{}, "ActivatedBy")
)

// TaskSpec is an argument structure to formalize the way that callers
// may query/select a task from an existing task queue to support
// out-of-order task execution for the purpose of task-groups.
type TaskSpec struct {
	Group         string `json:"group"`
	BuildVariant  string `json:"build_variant"`
	Project       string `json:"project"`
	Version       string `json:"version"`
	GroupMaxHosts int    `json:"group_max_hosts"`
}

func NewTaskQueue(distroID string, queue []TaskQueueItem, distroQueueInfo DistroQueueInfo) *TaskQueue {
	return &TaskQueue{
		Distro:          distroID,
		Queue:           queue,
		GeneratedAt:     time.Now(),
		DistroQueueInfo: distroQueueInfo,
	}
}

func LoadTaskQueue(ctx context.Context, distro string) (*TaskQueue, error) {
	return findTaskQueueForDistro(ctx, taskQueueQuery{DistroID: distro, Collection: TaskQueuesCollection})
}

func LoadDistroSecondaryTaskQueue(ctx context.Context, distroID string) (*TaskQueue, error) {
	return findTaskQueueForDistro(ctx, taskQueueQuery{DistroID: distroID, Collection: TaskSecondaryQueuesCollection})
}

func (tq *TaskQueue) Length() int {
	if tq == nil {
		return 0
	}
	return len(tq.Queue)
}

func (tq *TaskQueue) Save(ctx context.Context) error {
	if len(tq.Queue) > 10000 {
		tq.Queue = tq.Queue[:10000]
	}

	return updateTaskQueue(ctx, tq.Distro, tq.Queue, tq.DistroQueueInfo)
}

func updateTaskQueue(ctx context.Context, distro string, taskQueue []TaskQueueItem, distroQueueInfo DistroQueueInfo) error {
	_, err := db.Upsert(
		ctx,
		distroQueueInfo.GetQueueCollection(),
		bson.M{
			taskQueueDistroKey: distro,
		},
		bson.M{
			"$set": bson.M{
				taskQueueQueueKey:           taskQueue,
				taskQueueGeneratedAtKey:     time.Now(),
				taskQueueDistroQueueInfoKey: distroQueueInfo,
			},
		},
	)
	return errors.WithStack(err)
}

// ClearTaskQueue removes all tasks from task queue by updating them with a blank queue,
// and modifies the queue info.
// This is in contrast to RemoveTaskQueues, which simply deletes these documents.
func ClearTaskQueue(ctx context.Context, distroId string) error {
	grip.Info(message.Fields{
		"message": "clearing task queue",
		"distro":  distroId,
	})

	catcher := grip.NewBasicCatcher()

	// Task queue should always exist, so proceed with clearing
	distroQueueInfo, err := GetDistroQueueInfo(ctx, distroId)
	catcher.AddWhen(!adb.ResultsNotFound(err), errors.Wrapf(err, "getting task queue info"))
	distroQueueInfo = clearQueueInfo(distroQueueInfo)
	err = clearTaskQueueCollection(ctx, distroId, distroQueueInfo)
	if err != nil {
		catcher.Wrap(err, "clearing task queue")
	}

	// Make sure task secondary queue actually exists before modifying
	secondaryQueueQuery := bson.M{
		taskQueueDistroKey: distroId,
	}
	aliasCount, err := db.Count(ctx, TaskSecondaryQueuesCollection, secondaryQueueQuery)
	if err != nil {
		catcher.Wrap(err, "counting secondary queues matching distro")
	}
	// Want to at least try to clear even in the case of an error
	if aliasCount == 0 && err == nil {
		grip.Info(message.Fields{
			"message": "secondary task queue not found, skipping",
			"distro":  distroId,
		})
		return catcher.Resolve()
	}
	distroQueueInfo, err = GetDistroSecondaryQueueInfo(ctx, distroId)
	catcher.Wrap(err, "getting task secondary queue info")
	distroQueueInfo = clearQueueInfo(distroQueueInfo)

	err = clearTaskQueueCollection(ctx, distroId, distroQueueInfo)
	catcher.Wrap(err, "clearing task alias queue")
	return catcher.Resolve()
}

// clearQueueInfo takes in a DistroQueueInfo and blanks out appropriate fields
// for a cleared queue.
func clearQueueInfo(distroQueueInfo DistroQueueInfo) DistroQueueInfo {
	return DistroQueueInfo{
		Length:                        0,
		ExpectedDuration:              time.Duration(0),
		MaxDurationThreshold:          distroQueueInfo.MaxDurationThreshold,
		PlanCreatedAt:                 distroQueueInfo.PlanCreatedAt,
		CountDurationOverThreshold:    0,
		CountWaitOverThreshold:        0,
		CountDepFilledMergeQueueTasks: 0,
		TaskGroupInfos:                []TaskGroupInfo{},
		SecondaryQueue:                distroQueueInfo.SecondaryQueue,
	}
}

func clearTaskQueueCollection(ctx context.Context, distroId string, distroQueueInfo DistroQueueInfo) error {
	_, err := db.Upsert(
		ctx,
		distroQueueInfo.GetQueueCollection(),
		bson.M{
			taskQueueDistroKey: distroId,
		},
		bson.M{
			"$set": bson.M{
				taskQueueQueueKey:           []TaskQueueItem{},
				taskQueueGeneratedAtKey:     time.Now(),
				taskQueueDistroQueueInfoKey: distroQueueInfo,
			},
		},
	)
	return err
}

type taskQueueQuery struct {
	Collection string
	DistroID   string
}

func findTaskQueueForDistro(ctx context.Context, q taskQueueQuery) (*TaskQueue, error) {
	isDispatchedKey := bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemIsDispatchedKey)

	pipeline := []bson.M{
		{
			"$match": bson.M{taskQueueDistroKey: q.DistroID},
		},
		{
			"$unwind": bson.M{
				"path":                       "$" + taskQueueQueueKey,
				"preserveNullAndEmptyArrays": true,
			},
		},
		{
			"$match": bson.M{
				"$or": []bson.M{
					{ // queue is empty
						taskQueueQueueKey: bson.M{"$exists": false},
					},
					{ // omitempty/unpopulated is not dispatched
						isDispatchedKey: bson.M{"$exists": false},
					},
					{ // explicitly set to false
						isDispatchedKey: false,
					},
				},
			},
		},
		{
			"$group": bson.M{
				"_id": taskQueueItemIdKey,
				taskQueueQueueKey: bson.M{
					"$push": bson.M{
						taskQueueItemIdKey:            "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemIdKey),
						taskQueueItemIsDispatchedKey:  "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemIsDispatchedKey),
						taskQueueItemDisplayNameKey:   "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemDisplayNameKey),
						taskQueueItemGroupKey:         "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemGroupKey),
						taskQueueItemGroupMaxHostsKey: "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemGroupMaxHostsKey),
						taskQueueItemGroupIndexKey:    "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemGroupIndexKey),
						taskQueueItemVersionKey:       "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemVersionKey),
						taskQueueItemBuildVariantKey:  "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemBuildVariantKey),
						taskQueueItemConKey:           "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemConKey),
						taskQueueItemRequesterKey:     "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemRequesterKey),
						taskQueueItemRevisionKey:      "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemRevisionKey),
						taskQueueItemProjectKey:       "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemProjectKey),
						taskQueueItemExpDurationKey:   "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemExpDurationKey),
						taskQueueItemPriorityKey:      "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemPriorityKey),
						taskQueueItemActivatedByKey:   "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemActivatedByKey),
					},
				},
			},
		},
	}

	out := []TaskQueue{}

	err := db.Aggregate(ctx, q.Collection, pipeline, &out)
	if err != nil {
		if adb.ResultsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "building task queue for distro '%s'", q.DistroID)
	}
	if len(out) == 0 {
		return nil, nil
	}
	if len(out) > 1 {
		return nil, errors.Errorf("programmer error: task queue result malformed [num=%d, id=%s]",
			len(out), q.DistroID)
	}

	val := &out[0]

	if len(val.Queue) == 1 && val.Queue[0].Id == "" {
		val.Queue = nil
	}
	val.Distro = q.DistroID

	return val, nil
}

// FindMinimumQueuePositionForTask finds the position of a task in the many task queues
// where its position is the lowest. It returns an error if the aggregation it runs fails.
func FindMinimumQueuePositionForTask(ctx context.Context, taskId string) (int, error) {
	var results []struct {
		Index int `bson:"index"`
	}

	queueItemIdKey := bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemIdKey)
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

	err := db.Aggregate(ctx, TaskQueuesCollection, pipeline, &results)

	if len(results) == 0 {
		return -1, err
	}

	return results[0].Index + 1, err
}

func FindAllTaskQueues(ctx context.Context) ([]TaskQueue, error) {
	taskQueues := []TaskQueue{}
	err := db.FindAllQ(ctx, TaskQueuesCollection, db.Query(bson.M{}), &taskQueues)
	return taskQueues, err
}

func FindDistroTaskQueue(ctx context.Context, distroID string) (TaskQueue, error) {
	queue := TaskQueue{}
	err := db.FindOneQ(ctx, TaskQueuesCollection, db.Query(bson.M{taskQueueDistroKey: distroID}), &queue)
	return queue, errors.WithStack(err)
}

func FindDistroSecondaryTaskQueue(ctx context.Context, distroID string) (TaskQueue, error) {
	queue := TaskQueue{}
	q := db.Query(bson.M{taskQueueDistroKey: distroID})
	err := db.FindOneQ(ctx, TaskSecondaryQueuesCollection, q, &queue)

	return queue, errors.WithStack(err)
}

// pull out the task with the specified id from both the in-memory and db
// versions of the task queue
func (tq *TaskQueue) DequeueTask(ctx context.Context, taskId string) error {
	// first, remove it from the in-memory queue if it is present
outer:
	for {
		for idx, queueItem := range tq.Queue {
			if queueItem.Id == taskId {
				tq.Queue = append(tq.Queue[:idx], tq.Queue[idx+1:]...)
				continue outer
			}
		}
		break
	}

	// When something is dequeued from the in-memory queue on one app server, it
	// will still be present in every other app server's in-memory queue. It will
	// only no longer be present after the TTL has passed, and each app server
	// has re-created its in-memory queue.

	err := dequeue(ctx, taskId, tq.Distro)
	if adb.ResultsNotFound(err) {
		return nil
	}

	return errors.WithStack(err)
}

func dequeue(ctx context.Context, taskId, distroId string) error {
	itemKey := bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemIdKey)

	return errors.WithStack(db.Update(
		ctx,
		TaskQueuesCollection,
		bson.M{
			taskQueueDistroKey: distroId,
			itemKey:            taskId,
		},
		bson.M{
			"$set": bson.M{
				taskQueueQueueKey + ".$." + taskQueueItemIsDispatchedKey: true,
			},
		},
	))
}

type DuplicateEnqueuedTasksResult struct {
	TaskID    string   `bson:"_id"`
	DistroIDs []string `bson:"distros"`
}

func FindDuplicateEnqueuedTasks(ctx context.Context, coll string) ([]DuplicateEnqueuedTasksResult, error) {
	var res []DuplicateEnqueuedTasksResult
	taskIDKey := bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemIdKey)
	unwindTaskQueue := bson.M{
		"$unwind": "$" + taskQueueQueueKey,
	}
	countTaskOccurrences := bson.M{
		"$group": bson.M{
			"_id":     "$" + taskIDKey,
			"distros": bson.M{"$addToSet": "$" + taskQueueDistroKey},
			"count":   bson.M{"$sum": 1},
		},
	}
	includeNumQueues := bson.M{
		"$project": bson.M{
			"_id":                1,
			"distros":            1,
			"count":              1,
			"num_unique_distros": bson.M{"$size": "$distros"},
		},
	}
	matchDuplicateTasks := bson.M{
		"$match": bson.M{
			"count":              bson.M{"$gt": 1},
			"num_unique_distros": bson.M{"$gt": 1},
		},
	}
	pipeline := append([]bson.M{},
		unwindTaskQueue,
		countTaskOccurrences,
		includeNumQueues,
		matchDuplicateTasks,
	)
	if err := db.Aggregate(ctx, coll, pipeline, &res); err != nil {
		return nil, errors.WithStack(err)
	}
	return res, nil
}
