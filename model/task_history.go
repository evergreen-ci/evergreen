package model

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	TaskTimeout       = "timeout"
	TaskSystemFailure = "sysfail"
	TaskSetupFailure  = "setup-fail"

	// this regex either matches against the exact 'test' string, or
	// against the 'test' string at the end of some kind of filepath.
	testMatchRegex = `(\Q%s\E|.*(\\|/)\Q%s\E)$`

	taskHistoryMaxTime = 90 * time.Second
)

type taskHistoryIterator struct {
	TaskName      string
	BuildVariants []string
	ProjectName   string
}

type TaskHistoryChunk struct {
	Tasks       []bson.M
	Versions    []Version
	FailedTests map[string][]string
	Exhausted   ExhaustedIterator
}

type ExhaustedIterator struct {
	Before, After bool
}

type TaskHistory struct {
	Id    string                  `bson:"_id" json:"_id"`
	Order int                     `bson:"order" json:"order"`
	Tasks []aggregatedTaskHistory `bson:"tasks" json:"tasks"`
}

type aggregatedTaskHistory struct {
	Id               string                   `bson:"_id" json:"_id"`
	Status           string                   `bson:"status" json:"status"`
	Activated        bool                     `bson:"activated" json:"activated"`
	TimeTaken        time.Duration            `bson:"time_taken" json:"time_taken"`
	BuildVariant     string                   `bson:"build_variant" json:"build_variant"`
	LocalTestResults apimodels.TaskEndDetails `bson:"status_details" json:"status_details"`
	DisplayOnly      bool                     `bson:"display_only" json:"display_only"`
}
type TaskDetails struct {
	TimedOut bool   `bson:"timed_out"`
	Status   string `bson:"st"`
}

type TaskHistoryIterator interface {
	GetChunk(version *Version, numBefore, numAfter int, include bool) (TaskHistoryChunk, error)
}

func NewTaskHistoryIterator(name string, buildVariants []string, projectName string) TaskHistoryIterator {
	return TaskHistoryIterator(&taskHistoryIterator{TaskName: name, BuildVariants: buildVariants, ProjectName: projectName})
}

func (iter *taskHistoryIterator) findAllVersions(v *Version, numRevisions int, before, include bool) ([]Version, bool, error) {
	versionQuery := bson.M{
		VersionRequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
		VersionIdentifierKey: iter.ProjectName,
	}

	// If including the specified version in the result, then should
	// get an additional revision
	if include {
		numRevisions++
	}

	// Determine the comparator to use based on whether the revisions
	// come before/after the specified version
	compare, order := "$gt", VersionRevisionOrderNumberKey
	if before {
		compare, order = "$lt", fmt.Sprintf("-%v", VersionRevisionOrderNumberKey)
		if include {
			compare = "$lte"
		}
	} else if include {
		compare = "$gte"
	}

	if v != nil {
		versionQuery[VersionRevisionOrderNumberKey] = bson.M{compare: v.RevisionOrderNumber}
	}

	// Get the next numRevisions, plus an additional one to check if have
	// reached the beginning/end of history
	versions, err := VersionFind(
		db.Query(versionQuery).WithFields(
			VersionIdKey,
			VersionRevisionOrderNumberKey,
			VersionRevisionKey,
			VersionMessageKey,
			VersionCreateTimeKey,
		).Sort([]string{order}).Limit(numRevisions + 1))

	// Check if there were fewer results returned by the query than what
	// the limit was set as
	exhausted := len(versions) <= numRevisions
	if !exhausted {
		// Exclude the last version because we actually only wanted
		// `numRevisions` number of commits
		versions = versions[:len(versions)-1]
	}

	// The iterator can only be exhausted if an actual version was specified
	exhausted = exhausted || (v == nil && numRevisions == 0)

	if !before {
		// Reverse the order so that the most recent version is first
		for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
			versions[i], versions[j] = versions[j], versions[i]
		}
	}
	return versions, exhausted, err
}

// GetChunk returns tasks grouped by their versions, and sorted with the most
// recent first (i.e. descending commit order number).
func (iter *taskHistoryIterator) GetChunk(v *Version, numBefore, numAfter int, include bool) (TaskHistoryChunk, error) {
	var chunk TaskHistoryChunk

	versionsBefore, exhausted, err := iter.findAllVersions(v, numBefore, true, include)
	if err != nil {
		return chunk, errors.WithStack(err)
	}
	chunk.Exhausted.Before = exhausted

	versionsAfter, exhausted, err := iter.findAllVersions(v, numAfter, false, false)
	if err != nil {
		return chunk, errors.WithStack(err)
	}
	chunk.Exhausted.After = exhausted

	versions := append(versionsAfter, versionsBefore...)
	if len(versions) == 0 {
		return chunk, nil
	}
	chunk.Versions = versions

	// versionStartBoundary is the most recent version (i.e. newest) that
	// should be included in the results.
	//
	// versionEndBoundary is the least recent version (i.e. oldest) that
	// should be included in the results.
	versionStartBoundary, versionEndBoundary := versions[0], versions[len(versions)-1]

	matchStage := bson.M{
		task.RequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
		task.ProjectKey:     iter.ProjectName,
		task.DisplayNameKey: iter.TaskName,
		task.RevisionOrderNumberKey: bson.M{
			"$gte": versionEndBoundary.RevisionOrderNumber,
			"$lte": versionStartBoundary.RevisionOrderNumber,
		},
	}
	if len(iter.BuildVariants) > 0 {
		// only filter on bv if passed in - this handles scenarios where a task may have been removed
		// from the project yaml but we want to know its history before that
		matchStage[task.BuildVariantKey] = bson.M{"$in": iter.BuildVariants}
	}
	projectStage := bson.M{
		task.IdKey:                  1,
		task.StatusKey:              1,
		task.DetailsKey:             1,
		task.ActivatedKey:           1,
		task.TimeTakenKey:           1,
		task.BuildVariantKey:        1,
		task.RevisionKey:            1,
		task.RevisionOrderNumberKey: 1,
	}
	groupStage := bson.M{
		"_id":   fmt.Sprintf("$%s", task.RevisionKey),
		"order": bson.M{"$first": fmt.Sprintf("$%s", task.RevisionOrderNumberKey)},
		"tasks": bson.M{
			"$push": bson.M{
				task.IdKey:           fmt.Sprintf("$%s", task.IdKey),
				task.StatusKey:       fmt.Sprintf("$%s", task.StatusKey),
				task.DetailsKey:      fmt.Sprintf("$%s", task.DetailsKey),
				task.ActivatedKey:    fmt.Sprintf("$%s", task.ActivatedKey),
				task.TimeTakenKey:    fmt.Sprintf("$%s", task.TimeTakenKey),
				task.BuildVariantKey: fmt.Sprintf("$%s", task.BuildVariantKey),
				task.DisplayOnlyKey:  fmt.Sprintf("$%s", task.DisplayOnlyKey),
			},
		},
	}

	pipeline := []bson.M{
		{"$match": matchStage},
		{"$project": projectStage},
		{"$group": groupStage},
		{"$sort": bson.M{task.RevisionOrderNumberKey: -1}},
	}

	var rawAggregatedTasks []bson.M
	if _, err = db.AggregateWithMaxTime(task.Collection, pipeline, &rawAggregatedTasks, taskHistoryMaxTime); err != nil {
		return chunk, errors.Wrap(err, "aggregating task history data")
	}
	var aggregatedTasks []TaskHistory
	if _, err = db.AggregateWithMaxTime(task.Collection, pipeline, &aggregatedTasks, taskHistoryMaxTime); err != nil {
		return chunk, errors.Wrap(err, "aggregating task history data")
	}
	chunk.Tasks = rawAggregatedTasks
	failedTests, err := iter.GetFailedTests(aggregatedTasks)
	if err != nil {
		return chunk, errors.Wrap(err, "getting failed tasks for aggregated task history data")
	}
	chunk.FailedTests = failedTests

	return chunk, nil
}

// GetFailedTests returns a mapping of task id to a slice of failed tasks
// extracted from a pipeline of aggregated tasks.
func (thi *taskHistoryIterator) GetFailedTests(aggregatedTasks []TaskHistory) (map[string][]string, error) {
	var failedTasks []apimodels.CedarTaskInfo
	for _, group := range aggregatedTasks {
		for _, task := range group.Tasks {
			if task.Status == evergreen.TaskFailed {
				failedTasks = append(failedTasks, apimodels.CedarTaskInfo{
					TaskID:      task.Id,
					DisplayTask: task.DisplayOnly,
				})
			}
		}
	}
	if failedTasks == nil {
		// This is an added hack to make tests pass when transitioning
		// between Mongo drivers.
		return map[string][]string{}, nil
	}

	// Find the relevant failed tests.
	results, err := apimodels.GetCedarFilteredFailedSamples(context.Background(), apimodels.GetCedarFailedTestResultsSampleOptions{
		BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
		SampleOptions: apimodels.CedarFailedTestSampleOptions{Tasks: failedTasks},
	})
	if err != nil {
		return nil, errors.Wrap(err, "getting failed test results samples from Cedar")
	}

	failedTestsMap := make(map[string][]string)
	for _, result := range results {
		if len(result.MatchingFailedTestNames) > 0 {
			failedTestsMap[utility.FromStringPtr(result.TaskID)] = result.MatchingFailedTestNames
		}
	}

	return failedTestsMap, nil
}

type PickaxeParams struct {
	Project       *Project
	TaskName      string
	NewestOrder   int64
	OldestOrder   int64
	BuildVariants []string
}

func TaskHistoryPickaxe(params PickaxeParams) ([]task.Task, error) {
	// If there are no build variants, use all of them for the given task
	// name. We need this because without the build variant specified, no
	// amount of hinting will get sort to use the proper index.
	repo, err := FindRepository(params.Project.Identifier)
	if err != nil {
		return nil, errors.Wrap(err, "finding repository")
	}
	if repo == nil {
		return nil, errors.New("unable to find repository")
	}
	grip.Info(repo)
	buildVariants, err := task.FindVariantsWithTask(params.TaskName, params.Project.Identifier, repo.RevisionOrderNumber-50, repo.RevisionOrderNumber)
	if err != nil {
		return nil, errors.Wrap(err, "finding build variants")
	}
	query := bson.M{
		task.DisplayNameKey: params.TaskName,
		task.RevisionOrderNumberKey: bson.M{
			"$gte": params.OldestOrder,
			"$lte": params.NewestOrder,
		},
		task.ProjectKey: params.Project.Identifier,
	}
	if len(params.BuildVariants) > 0 {
		query[task.BuildVariantKey] = bson.M{
			"$in": params.BuildVariants,
		}
	} else if len(buildVariants) > 0 {
		query[task.BuildVariantKey] = bson.M{
			"$in": buildVariants,
		}
	}
	projection := []string{
		task.IdKey,
		task.StatusKey,
		task.ActivatedKey,
		task.TimeTakenKey,
		task.BuildVariantKey,
	}
	last, err := task.FindWithFields(query, projection...)
	return last, errors.Wrap(err, "finding tasks")
}
