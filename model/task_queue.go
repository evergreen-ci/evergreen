package model

import (
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/tychoish/tarjan"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	TaskQueuesCollection      = "task_queues"
	TaskAliasQueuesCollection = "task_alias_queues"
)

type TaskGroupInfo struct {
	Name                       string        `bson:"name" json:"name"`
	Count                      int           `bson:"count" json:"count"`
	CountFree                  int           `bson:"count_free" json:"count_free"`
	CountRequired              int           `bson:"count_required" json:"count_required"`
	MaxHosts                   int           `bson:"max_hosts" json:"max_hosts"`
	ExpectedDuration           time.Duration `bson:"expected_duration" json:"expected_duration"`
	CountDurationOverThreshold int           `bson:"count_over_threshold" json:"count_over_threshold"`
	CountWaitOverThreshold     int           `bson:"count_wait_over_threshold" json:"count_wait_over_threshold"`
	DurationOverThreshold      time.Duration `bson:"duration_over_threshold" json:"duration_over_threshold"`
}

type DistroQueueInfo struct {
	Length                     int             `bson:"length" json:"length"`
	ExpectedDuration           time.Duration   `bson:"expected_duration" json:"expected_duration"`
	MaxDurationThreshold       time.Duration   `bson:"max_duration_threshold" json:"max_duration_threshold"`
	PlanCreatedAt              time.Time       `bson:"created_at" json:"created_at"`
	CountDurationOverThreshold int             `bson:"count_over_threshold" json:"count_over_threshold"`
	DurationOverThreshold      time.Duration   `bson:"duration_over_threshold" json:"duration_over_threshold"`
	CountWaitOverThreshold     int             `bson:"count_wait_over_threshold" json:"count_wait_over_threshold"`
	TaskGroupInfos             []TaskGroupInfo `bson:"task_group_infos" json:"task_group_infos"`
	AliasQueue                 bool            `bson:"alias_queue" json:"alias_queue"`
}

func GetDistroQueueInfo(distroID string) (DistroQueueInfo, error) {
	rval, err := getDistroQueueInfoCollection(distroID, TaskQueuesCollection)
	return rval, err
}

func GetDistroAliasQueueInfo(distroID string) (DistroQueueInfo, error) {
	rval, err := getDistroQueueInfoCollection(distroID, TaskAliasQueuesCollection)
	return rval, err
}

func getDistroQueueInfoCollection(distroID, collection string) (DistroQueueInfo, error) {
	taskQueue := &TaskQueue{}
	q := db.Query(bson.M{taskQueueDistroKey: distroID}).Project(bson.M{taskQueueDistroQueueInfoKey: 1})
	err := db.FindOneQ(collection, q, taskQueue)

	if err != nil {
		return DistroQueueInfo{}, errors.Wrapf(err, "Database error retrieving DistroQueueInfo for distro id '%s'", distroID)
	}

	return taskQueue.DistroQueueInfo, nil
}

func RemoveTaskQueues(distroID string) error {
	query := db.Query(bson.M{taskQueueDistroKey: distroID})
	catcher := grip.NewBasicCatcher()
	err := db.RemoveAllQ(TaskQueuesCollection, query)
	catcher.AddWhen(!adb.ResultsNotFound(err), errors.Wrapf(err, "problem removing task queue for '%s'", distroID))
	err = db.RemoveAllQ(TaskAliasQueuesCollection, query)
	catcher.AddWhen(!adb.ResultsNotFound(err), errors.Wrapf(err, "problem removing task queue for '%s'", distroID))
	return catcher.Resolve()
}

// GetQueueCollection returns the collection associated with this queue.
func (q *DistroQueueInfo) GetQueueCollection() string {
	if q.AliasQueue {
		return TaskAliasQueuesCollection
	}

	return TaskQueuesCollection
}

// TaskQueue represents the next n tasks to be run on hosts of the distro
type TaskQueue struct {
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
	GroupIndex          int           `bson:"group_index,omitempty" json:"group_index,omitempty"`
	Version             string        `bson:"version" json:"version"`
	BuildVariant        string        `bson:"build_variant" json:"build_variant"`
	RevisionOrderNumber int           `bson:"order" json:"order"`
	Requester           string        `bson:"requester" json:"requester"`
	Revision            string        `bson:"gitspec" json:"gitspec"`
	Project             string        `bson:"project" json:"project"`
	ExpectedDuration    time.Duration `bson:"exp_dur" json:"exp_dur"`
	Priority            int64         `bson:"priority" json:"priority"`
	Dependencies        []string      `bson:"dependencies" json:"dependencies"`
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

	// taskQueueInfoLengthKey             = bsonutil.MustHaveTag(DistroQueueInfo{}, "Length")
	// taskQueueInfoExpectedDurationKey   = bsonutil.MustHaveTag(DistroQueueInfo{}, "ExpectedDuration")
	// taskQueueInfoMaxDurationKey        = bsonutil.MustHaveTag(DistroQueueInfo{}, "MaxDurationThreshold")
	taskQueueInfoPlanCreatedAtKey = bsonutil.MustHaveTag(DistroQueueInfo{}, "PlanCreatedAt")
	// taskQueueInfoCountDurationOverThresholdKey = bsonutil.MustHaveTag(DistroQueueInfo{}, "CountDurationOverThreshold")
	// taskQueueInfoCountWaitOverThresholdKey = bsonutil.MustHaveTag(DistroQueueInfo{}, "CountWaitOverThreshold")
	// taskQueueInfoTaskGroupInfosKey     = bsonutil.MustHaveTag(DistroQueueInfo{}, "TaskGroupInfos")
	// taskQueueInfoAliasQueueKey         = bsonutil.MustHaveTag(DistroQueueInfo{}, "AliasQueue")

	// taskQueueInfoGroupNameKey                  = bsonutil.MustHaveTag(TaskGroupInfo{}, "Name")
	// taskQueueInfoGroupCountKey                 = bsonutil.MustHaveTag(TaskGroupInfo{}, "Count")
	// taskQueueInfoGroupMaxHostsKey              = bsonutil.MustHaveTag(TaskGroupInfo{}, "MaxHosts")
	// taskQueueInfoGroupExpectedDuratioKey       = bsonutil.MustHaveTag(TaskGroupInfo{}, "ExpectedDuration")
	// taskQueueInfoGroupCountOverThresholdKey    = bsonutil.MustHaveTag(TaskGroupInfo{}, "CountOverThreshold")
	// taskQueueInfoGroupDurationOverThresholdKey = bsonutil.MustHaveTag(TaskGroupInfo{}, "DurationOverThreshold")
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

func LoadTaskQueue(distro string) (*TaskQueue, error) {
	return findTaskQueueForDistro(taskQueueQuery{DistroID: distro, Collection: TaskQueuesCollection})
}

func LoadDistroAliasTaskQueue(distroID string) (*TaskQueue, error) {
	return findTaskQueueForDistro(taskQueueQuery{DistroID: distroID, Collection: TaskAliasQueuesCollection})
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
	numHosts, err := host.NumHostsByTaskSpec(spec.Group, spec.BuildVariant, spec.Project, spec.Version)
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
	t, err := task.FindOneId(taskID)
	if err != nil {
		return errors.Wrapf(err, "problem finding task %s", taskID)
	}
	if t == nil {
		return errors.Errorf("found nil task %s", taskID)
	}

	p, err := FindProjectFromVersionID(t.Version)
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
		return errors.Wrapf(err, "problem finding tasks %s", strings.Join(taskNamesToBlock, ", "))
	}
	if err = ValidateNewGraph(t, tasksToBlock); err != nil {
		return errors.Wrap(err, "problem validating proposed dependencies")
	}

	catcher := grip.NewBasicCatcher()
	for _, taskToBlock := range tasksToBlock {
		catcher.Add(taskToBlock.AddDependency(task.Dependency{TaskId: taskID, Status: evergreen.TaskSucceeded, Unattainable: true}))
		err = dequeue(taskToBlock.Id, taskToBlock.DistroId)
		catcher.AddWhen(!adb.ResultsNotFound(err), err) // it's not an error if the task already isn't on the queue
		// this operation is recursive, maybe be refactorable
		// to use some kind of cache.
		catcher.Add(UpdateBlockedDependencies(&taskToBlock))
	}
	return catcher.Resolve()
}

func (self *TaskQueue) Save() error {
	if len(self.Queue) > 10000 {
		self.Queue = self.Queue[:10000]
	}

	return updateTaskQueue(self.Distro, self.Queue, self.DistroQueueInfo)
}

func (self *TaskQueue) FindNextTask(spec TaskSpec) (*TaskQueueItem, []string) {
	if self.Length() == 0 {
		return nil, nil
	}
	// With a spec, find a matching task.
	if spec.Group != "" && spec.Project != "" && spec.BuildVariant != "" && spec.Version != "" {
		for _, it := range self.Queue {
			if it.Project != spec.Project {
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
			return &it, nil
		}
	}

	// Otherwise, find the next dispatchable task.
	spec = TaskSpec{}
	for _, it := range self.Queue {
		// Always return a task if the task group is empty.
		if it.Group == "" {
			return &it, nil
		}

		// If we already determined that this task group is not runnable, continue.
		if it.Group == spec.Group &&
			it.BuildVariant == spec.BuildVariant &&
			it.Project == spec.Project &&
			it.Version == spec.Version &&
			it.GroupMaxHosts == spec.GroupMaxHosts {
			continue
		}

		// Otherwise, return the task if it is running on fewer than its task group's max hosts.
		spec = TaskSpec{
			Group:         it.Group,
			BuildVariant:  it.BuildVariant,
			Project:       it.Project,
			Version:       it.Version,
			GroupMaxHosts: it.GroupMaxHosts,
		}
		if shouldRun := shouldRunTaskGroup(it.Id, spec); shouldRun {
			return &it, nil
		}
	}
	return nil, nil
}

func updateTaskQueue(distro string, taskQueue []TaskQueueItem, distroQueueInfo DistroQueueInfo) error {
	_, err := db.Upsert(
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
func ClearTaskQueue(distroId string) error {
	grip.Info(message.Fields{
		"message": "clearing task queue",
		"distro":  distroId,
	})

	catcher := grip.NewBasicCatcher()

	// task queue should always exist, so proceed with clearing
	distroQueueInfo, err := GetDistroQueueInfo(distroId)
	if err != nil {
		catcher.Add(errors.Wrap(err, "error getting task queue info"))
	}
	distroQueueInfo = clearQueueInfo(distroQueueInfo)
	err = clearTaskQueueCollection(distroId, distroQueueInfo)
	if err != nil {
		catcher.Add(errors.Wrap(err, "error clearing task queue"))
	}

	// make sure task alias queue actually exists before modifying
	aliasQuery := bson.M{
		taskQueueDistroKey: distroId,
	}
	aliasCount, err := db.Count(TaskAliasQueuesCollection, aliasQuery)
	if err != nil {
		catcher.Add(err)
	}
	// want to at least try to clear even in the case of an error
	if aliasCount == 0 && err == nil {
		grip.Info(message.Fields{
			"message": "alias task queue not found, skipping",
			"distro":  distroId,
		})
		return catcher.Resolve()
	}
	distroQueueInfo, err = GetDistroAliasQueueInfo(distroId)
	if err != nil {
		catcher.Add(errors.Wrap(err, "error getting task alias queue info"))
	}
	distroQueueInfo = clearQueueInfo(distroQueueInfo)

	err = clearTaskQueueCollection(distroId, distroQueueInfo)
	if err != nil {
		catcher.Add(errors.Wrap(err, "error clearing task alias queue"))
	}
	return catcher.Resolve()
}

// clearQueueInfo takes in a DistroQueueInfo and blanks out appropriate fields
func clearQueueInfo(distroQueueInfo DistroQueueInfo) DistroQueueInfo {
	new_distroQueueInfo := DistroQueueInfo{
		Length:                     0,
		ExpectedDuration:           time.Duration(0),
		MaxDurationThreshold:       distroQueueInfo.MaxDurationThreshold,
		PlanCreatedAt:              distroQueueInfo.PlanCreatedAt,
		CountDurationOverThreshold: 0,
		CountWaitOverThreshold:     0,
		TaskGroupInfos:             []TaskGroupInfo{},
		AliasQueue:                 distroQueueInfo.AliasQueue,
	}

	return new_distroQueueInfo
}

func clearTaskQueueCollection(distroId string, distroQueueInfo DistroQueueInfo) error {

	_, err := db.Upsert(
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

func findTaskQueueForDistro(q taskQueueQuery) (*TaskQueue, error) {
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
					},
				},
			},
		},
	}

	out := []TaskQueue{}

	err := db.Aggregate(q.Collection, pipeline, &out)
	if err != nil {
		if adb.ResultsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "problem building task queue for '%s'", q.DistroID)
	}
	if len(out) == 0 {
		return nil, nil
	}
	if len(out) > 1 {
		return nil, errors.Errorf("task queue result malformed [num=%d, id=%s], programmer error",
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

// FindEnqueuedTaskIDs finds all tasks IDs that are already in a task queue for
// a given collection.
func FindEnqueuedTaskIDs(taskIDs []string, coll string) ([]string, error) {
	var results []struct {
		ID string `bson:"task_id"`
	}
	queueItemIdKey := bsonutil.GetDottedKeyName(taskQueueQueueKey, taskQueueItemIdKey)
	pipeline := []bson.M{
		{"$match": bson.M{
			queueItemIdKey: bson.M{"$in": taskIDs}}},
		{"$unwind": bson.M{
			"path": "$" + taskQueueQueueKey}},
		{"$match": bson.M{
			queueItemIdKey: bson.M{"$in": taskIDs},
		}},
		{"$project": bson.M{
			"task_id": "$" + queueItemIdKey,
		}},
	}

	err := db.Aggregate(coll, pipeline, &results)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ids := []string{}
	for _, res := range results {
		ids = append(ids, res.ID)
	}
	return ids, nil
}

func FindAllTaskQueues() ([]TaskQueue, error) {
	taskQueues := []TaskQueue{}
	err := db.FindAllQ(TaskQueuesCollection, db.Query(bson.M{}), &taskQueues)
	return taskQueues, err
}

func FindDistroTaskQueue(distroID string) (TaskQueue, error) {
	queue := TaskQueue{}
	err := db.FindOneQ(TaskQueuesCollection, db.Query(bson.M{taskQueueDistroKey: distroID}), &queue)

	grip.DebugWhen(err == nil, message.Fields{
		"message":                              "fetched the distro's TaskQueueItems to create its TaskQueue",
		"distro":                               distroID,
		"task_queue_generated_at":              queue.GeneratedAt,
		"num_task_queue_items":                 len(queue.Queue),
		"distro_queue_info_length":             queue.DistroQueueInfo.Length,
		"distro_queue_info_expected_duration":  queue.DistroQueueInfo.ExpectedDuration,
		"num_distro_queue_info_taskgroupinfos": len(queue.DistroQueueInfo.TaskGroupInfos),
	})

	return queue, errors.WithStack(err)
}

func FindDistroAliasTaskQueue(distroID string) (TaskQueue, error) {
	queue := TaskQueue{}
	q := db.Query(bson.M{taskQueueDistroKey: distroID})
	err := db.FindOneQ(TaskAliasQueuesCollection, q, &queue)

	return queue, errors.WithStack(err)
}

func taskQueueGenerationTimesPipeline() []bson.M {
	return []bson.M{
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
	}
}

func taskQueueGenerationRuntimePipeline() []bson.M {
	return []bson.M{
		{
			"$group": bson.M{
				"_id": 0,
				"distroQueue": bson.M{"$push": bson.M{
					"k": "$" + taskQueueDistroKey,
					"v": bson.M{"$multiply": []interface{}{
						// convert ms to ns
						// for duration value
						1000000,
						bson.M{"$subtract": []interface{}{
							"$" + taskQueueGeneratedAtKey,
							"$" + bsonutil.GetDottedKeyName(taskQueueDistroQueueInfoKey, taskQueueInfoPlanCreatedAtKey),
						}},
					}}}}},
		},
		{
			"$project": bson.M{
				"root": bson.M{"$arrayToObject": "$distroQueue"},
			},
		},
		{
			"$replaceRoot": bson.M{"newRoot": "$root"},
		},
	}
}

func runTimeMapAggregation(collection string, pipe []bson.M) (map[string]time.Time, error) {
	out := []map[string]time.Time{}

	err := db.Aggregate(collection, pipe, &out)

	if err != nil {
		return map[string]time.Time{}, errors.Wrapf(err, "problem running aggregation for %s", collection)
	}

	switch len(out) {
	case 0:
		return map[string]time.Time{}, nil
	case 1:
		return out[0], nil
	default:
		return map[string]time.Time{}, errors.Errorf("produced invalid results with too many elements: [%s:%d]", collection, len(out))
	}

}

func runDurationMapAggregation(collection string, pipe []bson.M) (map[string]time.Duration, error) {
	out := []map[string]time.Duration{}

	err := db.Aggregate(collection, pipe, &out)

	if err != nil {
		return map[string]time.Duration{}, errors.Wrapf(err, "problem running aggregation for %s", collection)
	}

	switch len(out) {
	case 0:
		return map[string]time.Duration{}, nil
	case 1:
		return out[0], nil
	default:
		return map[string]time.Duration{}, errors.Errorf("produced invalid results with too many elements: [%s:%d]", collection, len(out))
	}

}

func FindTaskQueueGenerationRuntime() (map[string]time.Duration, error) {
	return runDurationMapAggregation(TaskQueuesCollection, taskQueueGenerationRuntimePipeline())
}

func FindTaskAliasQueueGenerationRuntime() (map[string]time.Duration, error) {
	return runDurationMapAggregation(TaskAliasQueuesCollection, taskQueueGenerationRuntimePipeline())
}

func FindTaskQueueLastGenerationTimes() (map[string]time.Time, error) {
	return runTimeMapAggregation(TaskQueuesCollection, taskQueueGenerationTimesPipeline())
}

func FindTaskAliasQueueLastGenerationTimes() (map[string]time.Time, error) {
	return runTimeMapAggregation(TaskAliasQueuesCollection, taskQueueGenerationTimesPipeline())
}

// pull out the task with the specified id from both the in-memory and db
// versions of the task queue
func (self *TaskQueue) DequeueTask(taskId string) error {
	// first, remove it from the in-memory queue if it is present
outer:
	for {
		for idx, queueItem := range self.Queue {
			if queueItem.Id == taskId {
				self.Queue = append(self.Queue[:idx], self.Queue[idx+1:]...)
				continue outer
			}
		}
		break
	}

	// When something is dequeued from the in-memory queue on one app server, it
	// will still be present in every other app server's in-memory queue. It will
	// only no longer be present after the TTL has passed, and each app server
	// has re-created its in-memory queue.

	var err error
	err = dequeue(taskId, self.Distro)
	if adb.ResultsNotFound(err) {
		return nil
	}

	return errors.WithStack(err)
}

func dequeue(taskId, distroId string) error {
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

type DuplicateEnqueuedTasksResult struct {
	TaskID    string   `bson:"_id"`
	DistroIDs []string `bson:"distros"`
}

func FindDuplicateEnqueuedTasks(coll string) ([]DuplicateEnqueuedTasksResult, error) {
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
	if err := db.Aggregate(coll, pipeline, &res); err != nil {
		return nil, errors.WithStack(err)
	}
	return res, nil
}
