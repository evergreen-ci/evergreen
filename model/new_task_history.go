package model

import (
	"context"
	"sort"

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

// FindTaskHistoryOptions defines options that can be passed to queries in this file.
type FindTaskHistoryOptions struct {
	TaskName     string
	BuildVariant string
	ProjectId    string
	LowerBound   *int
	UpperBound   *int
	Limit        *int
}

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

// FindActiveTasksForHistory finds LIMIT active tasks with the given task name, build variant, and project ID between the specified bounds.
// The result is sorted by order numbers, descending (e.g. 100, 99, 98, 97, ...).
func FindActiveTasksForHistory(ctx context.Context, opts FindTaskHistoryOptions) ([]task.Task, error) {
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

// FindInactiveTasksForHistory finds all inactive tasks with the given task name, build variant, and project ID between the specified bounds.
// The result is sorted by order numbers, descending (e.g. 100, 99, 98, 97, ...).
func FindInactiveTasksForHistory(ctx context.Context, opts FindTaskHistoryOptions) ([]task.Task, error) {
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

// GetNewestWaterfallTask returns the most recent task, activated or unactivated, on the waterfall.
func GetNewestWaterfallTask(ctx context.Context, opts FindTaskHistoryOptions) (*task.Task, error) {
	filter := getBaseTaskHistoryFilter(opts)
	q := db.Query(filter).Sort([]string{"-" + task.RevisionOrderNumberKey}).Limit(1)
	mostRecentTask, err := task.FindOne(ctx, q)

	if err != nil {
		return nil, err
	}
	if mostRecentTask == nil {
		return nil, errors.New("task doesn't exist on project history")
	}
	return mostRecentTask, nil
}

// GetOldestWaterfallTask returns the oldest task, activated or unactivated, on the waterfall.
// Note that we cannot assume that the oldest task has an order of 1, because new tasks can be introduced over time,
// and because the task TTL deletes old tasks.
func GetOldestWaterfallTask(ctx context.Context, opts FindTaskHistoryOptions) (*task.Task, error) {
	filter := getBaseTaskHistoryFilter(opts)
	q := db.Query(filter).Sort([]string{task.RevisionOrderNumberKey}).Limit(1)
	oldestTask, err := task.FindOne(ctx, q)

	if err != nil {
		return nil, err
	}
	if oldestTask == nil {
		return nil, errors.New("task doesn't exist on project history")
	}
	return oldestTask, nil
}
