package model

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	maxTaskHistoryLimit = 50
)

// FindTaskHistoryOptions defines options that can be passed to queries for task history.
type FindTaskHistoryOptions struct {
	TaskName     string
	BuildVariant string
	ProjectId    string
	LowerBound   *int
	UpperBound   *int
	Limit        *int
}

// getBaseTaskHistoryFilter defines a basic match for the task history query. This is useful as fetching task history
// requires matching on multiple fields (i.e. the task name, build variant, and project fields).
func getBaseTaskHistoryFilter(opts FindTaskHistoryOptions) bson.M {
	return bson.M{
		task.DisplayNameKey:  opts.TaskName,
		task.BuildVariantKey: opts.BuildVariant,
		task.ProjectKey:      opts.ProjectId,
		task.RequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
	}
}

// findActiveTasksForHistory finds LIMIT active tasks with the given task name, build variant, and project ID between the specified bounds.
// Note that only one bound should be specified.
// The result is sorted by order numbers, descending (e.g. 100, 99, 98, 97, ...).
func findActiveTasksForHistory(ctx context.Context, opts FindTaskHistoryOptions) ([]task.Task, error) {
	filter := getBaseTaskHistoryFilter(opts)
	filter[task.ActivatedKey] = true

	var querySort []string // Requires different sorts so that the limit is taken correctly.
	var isSortedAsc bool

	if opts.LowerBound != nil {
		filter[task.RevisionOrderNumberKey] = bson.M{"$gte": utility.FromIntPtr(opts.LowerBound)}
		querySort = []string{task.RevisionOrderNumberKey}
		isSortedAsc = true
	}
	if opts.UpperBound != nil {
		filter[task.RevisionOrderNumberKey] = bson.M{"$lte": utility.FromIntPtr(opts.UpperBound)}
		querySort = []string{"-" + task.RevisionOrderNumberKey}
		isSortedAsc = false
	}

	queryLimit := maxTaskHistoryLimit
	if opts.Limit != nil {
		queryLimit = utility.FromIntPtr(opts.Limit)
	}

	q := db.Query(filter).Sort(querySort).Limit(queryLimit)
	tasks, err := task.FindAll(ctx, q)

	// We want the result to be sorted in descending order numbers. If it's currently sorted in ascending order,
	// sort the result correctly.
	if isSortedAsc {
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].RevisionOrderNumber > tasks[j].RevisionOrderNumber })
	}

	return tasks, err
}

// findInactiveTasksForHistory finds all inactive tasks with the given task name, build variant, and project ID between the specified bounds.
// The result is sorted by order numbers, descending (e.g. 100, 99, 98, 97, ...).
func findInactiveTasksForHistory(ctx context.Context, opts FindTaskHistoryOptions) ([]task.Task, error) {
	filter := getBaseTaskHistoryFilter(opts)
	filter[task.ActivatedKey] = false

	revisionFilter := bson.M{}
	if opts.LowerBound != nil {
		revisionFilter["$gte"] = utility.FromIntPtr(opts.LowerBound)
	}
	if opts.UpperBound != nil {
		revisionFilter["$lte"] = utility.FromIntPtr(opts.UpperBound)
	}
	filter[task.RevisionOrderNumberKey] = revisionFilter

	q := db.Query(filter).Sort([]string{"-" + task.RevisionOrderNumberKey})
	tasks, err := task.FindAll(ctx, q)
	return tasks, err
}

// FindTasksForHistory finds tasks with the given task name, build variant, and project ID between the specified bounds.
// The result is sorted by order numbers, descending (e.g. 100, 99, 98, 97, ...).
func FindTasksForHistory(ctx context.Context, opts FindTaskHistoryOptions) ([]task.Task, error) {
	// Active tasks must be fetched with either a lower bound or upper bound (not both), so we check for valid
	// arguments here.
	if (opts.UpperBound != nil && opts.LowerBound != nil) || (opts.UpperBound == nil && opts.LowerBound == nil) {
		return nil, errors.New("Exactly one bound must be defined.")
	}

	activeTasks, err := findActiveTasksForHistory(ctx, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "finding active tasks for history")
	}

	// Adjust the bounds because we want to fetch all inactive tasks that appear between the active tasks. This typically
	// means that both the lower bound and upper bound should be defined.
	// However, if all tasks are inactive, then one bound is sufficient, as we'll just fetch all inactive tasks. Note that
	// this is an uncommon edge case.
	if len(activeTasks) > 0 && opts.UpperBound == nil {
		newerActiveTask, err := getNewerActiveMainlineTask(ctx, activeTasks[0])
		if err != nil {
			return nil, errors.Wrapf(err, "fetching newer activated mainline task")
		}
		if newerActiveTask != nil {
			opts.UpperBound = utility.ToIntPtr(newerActiveTask.RevisionOrderNumber - 1)
		}
	}

	if len(activeTasks) > 0 && opts.LowerBound == nil {
		olderActiveTask, err := getOlderActiveMainlineTask(ctx, activeTasks[len(activeTasks)-1])
		if err != nil {
			return nil, errors.Wrapf(err, "fetching older activated mainline task")
		}
		if olderActiveTask != nil {
			opts.LowerBound = utility.ToIntPtr(olderActiveTask.RevisionOrderNumber + 1)
		}
	}

	inactiveTasks, err := findInactiveTasksForHistory(ctx, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "finding inactive tasks for history")
	}

	tasks := append(activeTasks, inactiveTasks...)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].RevisionOrderNumber > tasks[j].RevisionOrderNumber })
	return tasks, nil
}

// The index `branch_1_build_variant_1_display_name_1_status_1_r_1_activated_1_order_1` is a good index
// for GetLatestMainlineTask & GetOldestMainlineTask, but the query planner does not detect this.
// Using this index as a hint allows these queries to run efficiently.
var TaskHistoryIndex = bson.D{
	{Key: task.ProjectKey, Value: 1},
	{Key: task.BuildVariantKey, Value: 1},
	{Key: task.DisplayNameKey, Value: 1},
	{Key: task.StatusKey, Value: 1},
	{Key: task.RequesterKey, Value: 1},
	{Key: task.ActivatedKey, Value: 1},
	{Key: task.RevisionOrderNumberKey, Value: 1},
}

// GetLatestMainlineTask returns the most recent task matching the given parameters, activated or unactivated, on the waterfall.
func GetLatestMainlineTask(ctx context.Context, opts FindTaskHistoryOptions) (*task.Task, error) {
	filter := getBaseTaskHistoryFilter(opts)
	q := db.Query(filter).Sort([]string{"-" + task.RevisionOrderNumberKey}).Limit(1).Hint(TaskHistoryIndex)
	mostRecentTask, err := task.FindOne(ctx, q)

	if err != nil {
		return nil, err
	}
	if mostRecentTask == nil {
		return nil, errors.New("task not found on project history")
	}
	return mostRecentTask, nil
}

// GetOldestMainlineTask returns the oldest task matching the given parameters, activated or unactivated, on the waterfall.
// Note that we cannot assume that the oldest task has an order of 1, because new tasks can be introduced over time,
// and because the task TTL deletes old tasks.
func GetOldestMainlineTask(ctx context.Context, opts FindTaskHistoryOptions) (*task.Task, error) {
	filter := getBaseTaskHistoryFilter(opts)
	q := db.Query(filter).Sort([]string{task.RevisionOrderNumberKey}).Limit(1).Hint(TaskHistoryIndex)
	oldestTask, err := task.FindOne(ctx, q)

	if err != nil {
		return nil, err
	}
	if oldestTask == nil {
		return nil, errors.New("task not found on project history")
	}
	return oldestTask, nil
}

// getNewerActiveMainlineTask returns the next newer active mainline task, i.e. a more
// recent activated task than the current task.
func getNewerActiveMainlineTask(ctx context.Context, t task.Task) (*task.Task, error) {
	filter := getBaseTaskHistoryFilter(FindTaskHistoryOptions{
		TaskName:     t.DisplayName,
		BuildVariant: t.BuildVariant,
		ProjectId:    t.Project,
	})
	filter[task.ActivatedKey] = true
	filter[task.RevisionOrderNumberKey] = bson.M{
		"$gt": t.RevisionOrderNumber,
	}
	q := db.Query(filter).Sort([]string{task.RevisionOrderNumberKey}).Limit(1)

	newerActiveTask, err := task.FindOne(ctx, q)
	if err != nil {
		return nil, err
	}
	// newerActiveTask can be nil, since it may not exist.
	return newerActiveTask, nil
}

// getOlderActiveMainlineTask returns the next older active mainline task, i.e. an older
// activated task than the current task.
func getOlderActiveMainlineTask(ctx context.Context, t task.Task) (*task.Task, error) {
	filter := getBaseTaskHistoryFilter(FindTaskHistoryOptions{
		TaskName:     t.DisplayName,
		BuildVariant: t.BuildVariant,
		ProjectId:    t.Project,
	})
	filter[task.ActivatedKey] = true
	filter[task.RevisionOrderNumberKey] = bson.M{
		"$lt": t.RevisionOrderNumber,
	}
	q := db.Query(filter).Sort([]string{"-" + task.RevisionOrderNumberKey}).Limit(1)

	olderActiveTask, err := task.FindOne(ctx, q)
	if err != nil {
		return nil, err
	}
	// olderActiveTask can be nil, since it may not exist.
	return olderActiveTask, nil
}

// GetTaskOrderByDate returns the revision order of a system-requested task created on or before the given date.
// The task must match a specific display name, build variant, and project ID.
func GetTaskOrderByDate(ctx context.Context, date time.Time, opts FindTaskHistoryOptions) (int, error) {
	filter := getBaseTaskHistoryFilter(opts)
	filter[task.CreateTimeKey] = bson.M{"$lte": date}
	q := db.Query(filter).Sort([]string{"-" + task.RevisionOrderNumberKey}).Limit(1).WithFields(task.RevisionOrderNumberKey).Hint(TaskHistoryIndex)

	found, err := task.FindOne(ctx, q)
	if err != nil {
		return 0, errors.New(fmt.Sprintf("finding task on or before date '%s': %s", date.Format(time.DateOnly), err.Error()))
	} else if found == nil {
		return 0, errors.New(fmt.Sprintf("task on or before date '%s' not found", date.Format(time.DateOnly)))
	}
	return found.RevisionOrderNumber, nil
}
