package model

import (
	"context"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// FindTaskHistoryByCreateTimeOptions defines options for task history queries sorted by create_time.
type FindTaskHistoryByCreateTimeOptions struct {
	TaskName          string
	BuildVariant      string
	ProjectId         string
	LowerBound        *time.Time
	IncludeLowerBound bool
	UpperBound        *time.Time
	IncludeUpperBound bool
	Limit             *int
}

// getBaseTaskHistoryByCreateTimeFilter defines a basic match for create_time-based task history queries.
func getBaseTaskHistoryByCreateTimeFilter(opts FindTaskHistoryByCreateTimeOptions) bson.M {
	return bson.M{
		task.DisplayNameKey:  opts.TaskName,
		task.BuildVariantKey: opts.BuildVariant,
		task.ProjectKey:      opts.ProjectId,
		task.RequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
	}
}

// findActiveTasksForHistoryByCreateTime finds LIMIT active tasks between the specified create_time bounds.
// Note that only one bound should be specified.
// The result is sorted by create_time, descending.
func findActiveTasksForHistoryByCreateTime(ctx context.Context, opts FindTaskHistoryByCreateTimeOptions) ([]task.Task, error) {
	filter := getBaseTaskHistoryByCreateTimeFilter(opts)
	filter[task.ActivatedKey] = true

	var querySort []string
	var isSortedAsc bool

	if opts.LowerBound != nil {
		op := "$gte"
		if !opts.IncludeLowerBound {
			op = "$gt"
		}
		filter[task.CreateTimeKey] = bson.M{op: utility.FromTimePtr(opts.LowerBound)}
		querySort = []string{task.CreateTimeKey}
		isSortedAsc = true
	}
	if opts.UpperBound != nil {
		op := "$lte"
		if !opts.IncludeUpperBound {
			op = "$lt"
		}
		filter[task.CreateTimeKey] = bson.M{op: utility.FromTimePtr(opts.UpperBound)}
		querySort = []string{"-" + task.CreateTimeKey}
		isSortedAsc = false
	}

	queryLimit := maxTaskHistoryLimit
	if opts.Limit != nil {
		queryLimit = utility.FromIntPtr(opts.Limit)
	}

	q := db.Query(filter).Sort(querySort).Limit(queryLimit)
	tasks, err := task.FindAll(ctx, q)

	// We want the result sorted in descending create_time. If it's currently sorted ascending, reverse it.
	if isSortedAsc {
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreateTime.After(tasks[j].CreateTime) })
	}

	return tasks, err
}

// findInactiveTasksForHistoryByCreateTime finds all inactive tasks between the specified create_time bounds.
// The result is sorted by create_time, descending.
func findInactiveTasksForHistoryByCreateTime(ctx context.Context, opts FindTaskHistoryByCreateTimeOptions) ([]task.Task, error) {
	filter := getBaseTaskHistoryByCreateTimeFilter(opts)
	filter[task.ActivatedKey] = false

	createTimeFilter := bson.M{}
	if opts.LowerBound != nil {
		op := "$gte"
		if !opts.IncludeLowerBound {
			op = "$gt"
		}
		createTimeFilter[op] = utility.FromTimePtr(opts.LowerBound)
	}
	if opts.UpperBound != nil {
		op := "$lte"
		if !opts.IncludeUpperBound {
			op = "$lt"
		}
		createTimeFilter[op] = utility.FromTimePtr(opts.UpperBound)
	}
	filter[task.CreateTimeKey] = createTimeFilter

	q := db.Query(filter).Sort([]string{"-" + task.CreateTimeKey})
	tasks, err := task.FindAll(ctx, q)
	return tasks, err
}

// FindTasksForHistoryByCreateTime finds tasks sorted by create_time between the specified bounds.
// The result is sorted by create_time, descending.
func FindTasksForHistoryByCreateTime(ctx context.Context, opts FindTaskHistoryByCreateTimeOptions) ([]task.Task, error) {
	if (opts.UpperBound != nil && opts.LowerBound != nil) || (opts.UpperBound == nil && opts.LowerBound == nil) {
		return nil, errors.New("Exactly one bound must be defined.")
	}

	activeTasks, err := findActiveTasksForHistoryByCreateTime(ctx, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "finding active tasks for history")
	}

	// Adjust the bounds to fetch all inactive tasks that appear between the active tasks. The neighboring
	// active tasks are excluded since they are already in the active results.
	if len(activeTasks) > 0 && opts.UpperBound == nil {
		newerActiveTask, err := getNewerActiveMainlineTaskByCreateTime(ctx, activeTasks[0])
		if err != nil {
			return nil, errors.Wrapf(err, "fetching newer activated mainline task")
		}
		if newerActiveTask != nil {
			opts.UpperBound = &newerActiveTask.CreateTime
			opts.IncludeUpperBound = false
		}
	}

	if len(activeTasks) > 0 && opts.LowerBound == nil {
		olderActiveTask, err := getOlderActiveMainlineTaskByCreateTime(ctx, activeTasks[len(activeTasks)-1])
		if err != nil {
			return nil, errors.Wrapf(err, "fetching older activated mainline task")
		}
		if olderActiveTask != nil {
			opts.LowerBound = &olderActiveTask.CreateTime
			opts.IncludeLowerBound = false
		}
	}

	inactiveTasks, err := findInactiveTasksForHistoryByCreateTime(ctx, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "finding inactive tasks for history")
	}

	tasks := append(activeTasks, inactiveTasks...)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreateTime.After(tasks[j].CreateTime) })
	return tasks, nil
}

// GetLatestMainlineTaskByCreateTime returns the most recent task by create_time matching the given parameters.
func GetLatestMainlineTaskByCreateTime(ctx context.Context, opts FindTaskHistoryByCreateTimeOptions) (*task.Task, error) {
	filter := getBaseTaskHistoryByCreateTimeFilter(opts)
	q := db.Query(filter).Sort([]string{"-" + task.CreateTimeKey}).Limit(1)
	mostRecentTask, err := task.FindOne(ctx, q)
	if err != nil {
		return nil, err
	}
	if mostRecentTask == nil {
		return nil, errors.New("task not found on project history")
	}
	return mostRecentTask, nil
}

// GetOldestMainlineTaskByCreateTime returns the oldest task by create_time matching the given parameters.
func GetOldestMainlineTaskByCreateTime(ctx context.Context, opts FindTaskHistoryByCreateTimeOptions) (*task.Task, error) {
	filter := getBaseTaskHistoryByCreateTimeFilter(opts)
	q := db.Query(filter).Sort([]string{task.CreateTimeKey}).Limit(1)
	oldestTask, err := task.FindOne(ctx, q)
	if err != nil {
		return nil, err
	}
	if oldestTask == nil {
		return nil, errors.New("task not found on project history")
	}
	return oldestTask, nil
}

// getNewerActiveMainlineTaskByCreateTime returns the next newer active mainline task by create_time.
func getNewerActiveMainlineTaskByCreateTime(ctx context.Context, t task.Task) (*task.Task, error) {
	filter := getBaseTaskHistoryByCreateTimeFilter(FindTaskHistoryByCreateTimeOptions{
		TaskName:     t.DisplayName,
		BuildVariant: t.BuildVariant,
		ProjectId:    t.Project,
	})
	filter[task.ActivatedKey] = true
	filter[task.CreateTimeKey] = bson.M{"$gt": t.CreateTime}
	q := db.Query(filter).Sort([]string{task.CreateTimeKey}).Limit(1)

	newerActiveTask, err := task.FindOne(ctx, q)
	if err != nil {
		return nil, err
	}
	// newerActiveTask can be nil, since it may not exist.
	return newerActiveTask, nil
}

// getOlderActiveMainlineTaskByCreateTime returns the next older active mainline task by create_time.
func getOlderActiveMainlineTaskByCreateTime(ctx context.Context, t task.Task) (*task.Task, error) {
	filter := getBaseTaskHistoryByCreateTimeFilter(FindTaskHistoryByCreateTimeOptions{
		TaskName:     t.DisplayName,
		BuildVariant: t.BuildVariant,
		ProjectId:    t.Project,
	})
	filter[task.ActivatedKey] = true
	filter[task.CreateTimeKey] = bson.M{"$lt": t.CreateTime}
	q := db.Query(filter).Sort([]string{"-" + task.CreateTimeKey}).Limit(1)

	olderActiveTask, err := task.FindOne(ctx, q)
	if err != nil {
		return nil, err
	}
	// olderActiveTask can be nil, since it may not exist.
	return olderActiveTask, nil
}
