package model

import (
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/tychoish/tarjan"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	TaskQueuesCollection = "task_queues"
)

var useModernDequeueOp = true

type TaskGroupInfo struct {
	Name                  string        `bson:"name" json:"name"`
	Count                 int           `bson:"count" json:"count"`
	MaxHosts              int           `bson:"max_hosts" json:"max_hosts"`
	ExpectedDuration      time.Duration `bson:"expected_duration" json:"expected_duration"`
	CountOverThreshold    int           `bson:"count_over_threshold" json:"count_over_threshold"`
	DurationOverThreshold time.Duration `bson:"duration_over_threshold" json:"duration_over_threshold"`
}

type DistroQueueInfo struct {
	Length               int             `bson:"length" json:"length"`
	ExpectedDuration     time.Duration   `bson:"expected_duration" json:"expected_duration"`
	MaxDurationThreshold time.Duration   `bson:"max_duration_threshold" json:"max_duration_threshold"`
	CountOverThreshold   int             `bson:"count_over_threshold" json:"count_over_threshold"`
	TaskGroupInfos       []TaskGroupInfo `bson:"task_group_infos" json:"task_group_infos"`
}

func GetDistroQueueInfo(distroID string) (DistroQueueInfo, error) {
	taskQueue := &TaskQueue{}
	err := db.FindOne(
		TaskQueuesCollection,
		bson.M{taskQueueDistroKey: distroID},
		bson.M{taskQueueDistroQueueInfoKey: 1, taskQueueIdKey: 0},
		db.NoSort,
		taskQueue,
	)
	if err != nil {
		return DistroQueueInfo{}, errors.Wrapf(err, "Database error retrieving DistroQueueInfo for distro id '%s'", distroID)
	}

	return taskQueue.DistroQueueInfo, err
}

// represents the next n tasks to be run on hosts of the distro
type TaskQueue struct {
	Id              bson.ObjectId   `bson:"_id,omitempty" json:"_id"`
	Distro          string          `bson:"distro" json:"distro"`
	GeneratedAt     time.Time       `bson:"generated_at" json:"generated_at"`
	Queue           []TaskQueueItem `bson:"queue" json:"queue"`
	DistroQueueInfo DistroQueueInfo `bson:"distro_queue_info" json:"distro_queue_info"`

	useModerDequeueOp bool
}

type TaskDep struct {
	Id          string `bson:"task_id,omitempty" json:"task_id"`
	DisplayName string `bson:"display_name" json:"display_name"`
}
type TaskQueueItem struct {
	Id                  string        `bson:"_id" json:"_id"`
	IsDispatched        bool          `bson:"dispatched" json:"dispatched"`
	DisplayName         string        `bson:"display_name" json:"display_name"`
	Group               string        `bson:"group_name" json:"group_name"`
	GroupMaxHosts       int           `bson:"group_max_hosts,omitempty" json:"group_max_hosts,omitempty"`
	Version             string        `bson:"version" json:"version"`
	BuildVariant        string        `bson:"build_variant" json:"build_variant"`
	RevisionOrderNumber int           `bson:"order" json:"order"`
	Requester           string        `bson:"requester" json:"requester"`
	Revision            string        `bson:"gitspec" json:"gitspec"`
	Project             string        `bson:"project" json:"project"`
	ExpectedDuration    time.Duration `bson:"exp_dur" json:"exp_dur"`
	Priority            int64         `bson:"priority" json:"priority"`
}

// must not no-lint these values
var (
	// bson fields for the task queue struct
	taskQueueIdKey              = bsonutil.MustHaveTag(TaskQueue{}, "Id")
	taskQueueDistroKey          = bsonutil.MustHaveTag(TaskQueue{}, "Distro")
	taskQueueGeneratedAtKey     = bsonutil.MustHaveTag(TaskQueue{}, "GeneratedAt")
	taskQueueQueueKey           = bsonutil.MustHaveTag(TaskQueue{}, "Queue")
	taskQueueDistroQueueInfoKey = bsonutil.MustHaveTag(TaskQueue{}, "DistroQueueInfo")

	// bson fields for the individual task queue items
	taskQueueItemIdKey            = bsonutil.MustHaveTag(TaskQueueItem{}, "Id")
	taskQueueItemIsDispatchedKey  = bsonutil.MustHaveTag(TaskQueueItem{}, "IsDispatched")
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
)

// TaskSpec is an argument structure to formalize the way that callers
// may query/select a task from an existing task queue to support
// out-of-order task execution for the purpose of task-groups.
type TaskSpec struct {
	Group         string `json:"group"`
	BuildVariant  string `json:"build_variant"`
	ProjectID     string `json:"project_id"`
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

func LoadTaskQueue(distro string) (*TaskQueue, error) {
	return findTaskQueueForDistro(distro)
}

func (self *TaskQueue) Length() int {
	if self == nil {
		return 0
	}
	return len(self.Queue)
}

func (self *TaskQueue) NextTask() *TaskQueueItem {
	return &self.Queue[0]
}

// shouldRunTaskGroup returns true if the number of hosts running a task is less than the maximum for that task group.
func shouldRunTaskGroup(taskId string, spec TaskSpec) bool {
	// Get number of hosts running this spec.
	numHosts, err := host.NumHostsByTaskSpec(spec.Group, spec.BuildVariant, spec.ProjectID, spec.Version)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "error finding hosts for spec",
			"task_id":    taskId,
			"queue_item": spec,
		}))
		return false
	}
	// If the group is running on 0 hosts, return true early.
	if numHosts == 0 {
		return true
	}
	// If this spec is running on fewer hosts than max_hosts, dispatch this task.
	if numHosts < spec.GroupMaxHosts {
		return true
	}
	return false
}

func ValidateNewGraph(t *task.Task, tasksToBlock []task.Task) error {
	tasksInVersion, err := task.FindAllTasksFromVersionWithDependencies(t.Version)
	if err != nil {
		return errors.Wrap(err, "problem finding version for task")
	}

	// tmap maps tasks to their dependencies
	tmap := map[string][]string{}
	for _, t := range tasksInVersion {
		for _, d := range t.DependsOn {
			tmap[t.Id] = append(tmap[t.Id], d.TaskId)
		}
	}

	// simulate proposed dependencies
	for _, taskToBlock := range tasksToBlock {
		tmap[taskToBlock.Id] = append(tmap[taskToBlock.Id], t.Id)
	}

	catcher := grip.NewBasicCatcher()
	for _, group := range tarjan.Connections(tmap) {
		if len(group) > 1 {
			catcher.Add(errors.Errorf("Cycle detected: %s", strings.Join(group, ", ")))
		}
	}
	return catcher.Resolve()
}

func BlockTaskGroupTasks(taskID string) error {
	catcher := grip.NewBasicCatcher()
	t, err := task.FindOneId(taskID)
	if err != nil {
		return errors.Wrapf(err, "problem finding task %s", taskID)
	}
	if t == nil {
		return errors.Errorf("found nil task %s", taskID)
	}

	p, err := FindProjectFromTask(t)
	if err != nil {
		return errors.Wrapf(err, "problem getting project for task %s", t.Id)
	}
	tg := p.FindTaskGroup(t.TaskGroup)
	if tg == nil {
		return errors.Errorf("unable to find task group '%s' for task '%s'", t.TaskGroup, taskID)
	}

	indexOfTask := -1
	for i, tgTask := range tg.Tasks {
		if t.DisplayName == tgTask {
			indexOfTask = i
			break
		}
	}
	if indexOfTask == -1 {
		return errors.Errorf("Could not find task '%s' in task group", t.DisplayName)
	}
	taskNamesToBlock := []string{}
	for i := indexOfTask + 1; i < len(tg.Tasks); i++ {
		taskNamesToBlock = append(taskNamesToBlock, tg.Tasks[i])
	}
	tasksToBlock, err := task.Find(task.ByVersionsForNameAndVariant([]string{t.Version}, taskNamesToBlock, t.BuildVariant))
	if err != nil {
		catcher.Add(errors.Wrapf(err, "problem finding tasks %s", strings.Join(taskNamesToBlock, ", ")))
	}

	if err := ValidateNewGraph(t, tasksToBlock); err != nil {
		return errors.Wrap(err, "problem validating proposed dependencies")
	}
	for _, taskToBlock := range tasksToBlock {
		catcher.Add(taskToBlock.AddDependency(task.Dependency{TaskId: taskID, Status: evergreen.TaskSucceeded}))
		catcher.Add(dequeue(taskToBlock.Id, taskToBlock.DistroId))
	}
	return catcher.Resolve()
}

func (self *TaskQueue) Save() error {
	if len(self.Queue) > 500 {
		self.Queue = self.Queue[:500]
	}
	return updateTaskQueue(self.Distro, self.Queue, self.DistroQueueInfo)
}

func (self *TaskQueue) FindNextTask(spec TaskSpec) *TaskQueueItem {
	if self.Length() == 0 {
		return nil
	}
	// With a spec, find a matching task.
	if spec.Group != "" && spec.ProjectID != "" && spec.BuildVariant != "" && spec.Version != "" {
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
	}

	// Otherwise, find the next dispatchable task.
	spec = TaskSpec{}
	for _, it := range self.Queue {
		// Always return a task if the task group is empty.
		if it.Group == "" {
			return &it
		}

		// If we already determined that this task group is not runnable, continue.
		if it.Group == spec.Group &&
			it.BuildVariant == spec.BuildVariant &&
			it.Project == spec.ProjectID &&
			it.Version == spec.Version &&
			it.GroupMaxHosts == spec.GroupMaxHosts {
			continue
		}

		// Otherwise, return the task if it is running on fewer than its task group's max hosts.
		spec = TaskSpec{
			Group:         it.Group,
			BuildVariant:  it.BuildVariant,
			ProjectID:     it.Project,
			Version:       it.Version,
			GroupMaxHosts: it.GroupMaxHosts,
		}
		if shouldRun := shouldRunTaskGroup(it.Id, spec); shouldRun {
			return &it
		}
	}
	return nil
}

func updateTaskQueue(distro string, taskQueue []TaskQueueItem, distroQueueInfo DistroQueueInfo) error {
	_, err := db.Upsert(
		TaskQueuesCollection,
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

func ClearTaskQueue(distro string) error {
	grip.Info(message.Fields{
		"message": "clearing task queue",
		"distro":  distro,
	})
	_, err := db.Upsert(
		TaskQueuesCollection,
		bson.M{
			taskQueueDistroKey: distro,
		},
		bson.M{
			"$set": bson.M{
				taskQueueQueueKey:       []TaskQueueItem{},
				taskQueueGeneratedAtKey: time.Now(),
			},
		},
	)
	return errors.Wrap(err, "error clearing task queue")
}

func findTaskQueueForDistro(distroId string) (*TaskQueue, error) {
	isDispatchedKey := bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemIsDispatchedKey)

	pipeline := []bson.M{
		{
			"$match": bson.M{taskQueueDistroKey: distroId},
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
				"_id": taskQueueIdKey,
				taskQueueQueueKey: bson.M{
					"$push": bson.M{
						taskQueueItemIdKey:            "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemIdKey),
						taskQueueItemIsDispatchedKey:  "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemIsDispatchedKey),
						taskQueueItemDisplayNameKey:   "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemDisplayNameKey),
						taskQueueItemGroupKey:         "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemGroupKey),
						taskQueueItemGroupMaxHostsKey: "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemGroupMaxHostsKey),
						taskQueueItemVersionKey:       "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemVersionKey),
						taskQueueItemBuildVariantKey:  "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemBuildVariantKey),
						taskQueueItemConKey:           "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemConKey),
						taskQueueItemRequesterKey:     "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemRequesterKey),
						taskQueueItemRevisionKey:      "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemRevisionKey),
						taskQueueItemProjectKey:       "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemProjectKey),
						taskQueueItemExpDurationKey:   "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemExpDurationKey),
						taskQueueItemPriorityKey:      "$" + bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemPriorityKey),
					},
				},
			},
		},
	}

	out := []TaskQueue{}

	err := db.Aggregate(TaskQueuesCollection, pipeline, &out)
	if err != nil {
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "problem building task queue for '%s'", distroId)
	}
	if len(out) == 0 {
		return nil, nil
	}
	if len(out) > 1 {
		return nil, errors.Errorf("task queue result malformed [num=%d, id=%s], programmer error",
			len(out), distroId)
	}

	val := &out[0]

	if len(val.Queue) == 1 && val.Queue[0].Id == "" {
		val.Queue = nil
	}
	val.Distro = distroId

	return val, nil
}

// FindMinimumQueuePositionForTask finds the position of a task in the many task queues
// where its position is the lowest. It returns an error if the aggregation it runs fails.
func FindMinimumQueuePositionForTask(taskId string) (int, error) {
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

func FindDistroTaskQueue(distroID string) (TaskQueue, error) {
	queue := TaskQueue{}
	err := db.FindOne(
		TaskQueuesCollection,
		bson.M{taskQueueIdKey: distroID},
		db.NoProjection,
		db.NoSort,
		&queue)

	return queue, errors.WithStack(err)
}

func FindTaskQueueGenerationTimes() (map[string]time.Time, error) {
	out := []map[string]time.Time{}

	err := db.Aggregate(TaskQueuesCollection, []bson.M{
		{
			"$group": bson.M{
				"_id": 0,
				"distroQueue": bson.M{"$push": bson.M{
					"k": "$" + taskQueueDistroKey,
					"v": "$" + taskQueueGeneratedAtKey,
				}}},
		},
		{
			"$project": bson.M{
				"root": bson.M{"$arrayToObject": "$distroQueue"},
			},
		},
		{
			"$replaceRoot": bson.M{"newRoot": "$root"},
		},
	}, &out)

	if err != nil {
		return map[string]time.Time{}, errors.WithStack(err)
	}

	if len(out) != 1 {
		return map[string]time.Time{}, errors.New("produced invalid results")
	}

	return out[0], nil
}

// pull out the task with the specified id from both the in-memory and db
// versions of the task queue
func (self *TaskQueue) DequeueTask(taskId string) error {
	// first, remove from the in-memory queue
	found := false
outer:
	for {
		for idx, queueItem := range self.Queue {
			if queueItem.Id == taskId {
				found = true
				self.Queue = append(self.Queue[:idx], self.Queue[idx+1:]...)
				continue outer
			}
		}
		break
	}

	// validate that the task is there
	if !found {
		return errors.Errorf("task id %s was not present in queue for distro %s",
			taskId, self.Distro)
	}

	var err error
	err = dequeue(taskId, self.Distro)
	if errors.Cause(err) == mgo.ErrNotFound {
		return nil
	}

	return errors.WithStack(err)
}

func dequeue(taskId, distroId string) error {
	if useModernDequeueOp {
		return dequeueUpdate(taskId, distroId)
	} else {
		return legacyDequeueUpdate(taskId, distroId)
	}
}

func dequeueUpdate(taskId, distroId string) error {
	itemKey := bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemIdKey)

	return errors.WithStack(db.Update(
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

func legacyDequeueUpdate(taskId, distroId string) error {
	return errors.WithStack(db.Update(
		TaskQueuesCollection,
		bson.M{
			taskQueueDistroKey: distroId,
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
