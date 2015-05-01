package model

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/version"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"time"
)

type taskHistoryIterator struct {
	TaskName      string
	BuildVariants []string
	ProjectName   string
}

type TaskHistoryChunk struct {
	Tasks       []bson.M
	Versions    []version.Version
	FailedTests map[string][]TestResult
	Exhausted   ExhaustedIterator
}

type ExhaustedIterator struct {
	Before, After bool
}

type TaskHistory struct {
	Id    string                  `bson:"_id" json:"_id"`
	Order string                  `bson:"order" json:"order"`
	Tasks []aggregatedTaskHistory `bson:"tasks" json:"tasks"`
}

type aggregatedTaskHistory struct {
	Id           string                   `bson:"_id" json:"_id"`
	Status       string                   `bson:"status" json:"status"`
	Activated    bool                     `bson:"activated" json:"activated"`
	TimeTaken    time.Duration            `bson:"time_taken" json:"time_taken"`
	BuildVariant string                   `bson:"build_variant" json:"build_variant"`
	TestResults  apimodels.TaskEndDetails `bson:"status_details" json:"status_details"`
}

type TaskHistoryIterator interface {
	GetChunk(version *version.Version, numBefore, numAfter int, include bool) (TaskHistoryChunk, error)
	GetDistinctTestNames(numCommits int) ([]string, error)
}

func NewTaskHistoryIterator(name string, buildVariants []string, projectName string) TaskHistoryIterator {
	return TaskHistoryIterator(&taskHistoryIterator{TaskName: name, BuildVariants: buildVariants, ProjectName: projectName})
}

func (iter *taskHistoryIterator) findAllVersions(v *version.Version, numRevisions int, before, include bool) ([]version.Version, bool, error) {
	versionQuery := bson.M{
		version.RequesterKey: evergreen.RepotrackerVersionRequester,
		version.ProjectKey:   iter.ProjectName,
	}

	// If including the specified version in the result, then should
	// get an additional revision
	if include {
		numRevisions++
	}

	// Determine the comparator to use based on whether the revisions
	// come before/after the specified version
	compare, order := "$gt", version.RevisionOrderNumberKey
	if before {
		compare, order = "$lt", fmt.Sprintf("-%v", version.RevisionOrderNumberKey)
		if include {
			compare = "$lte"
		}
	} else if include {
		compare = "$gte"
	}

	if v != nil {
		versionQuery[version.RevisionOrderNumberKey] = bson.M{compare: v.RevisionOrderNumber}
	}

	// Get the next numRevisions, plus an additional one to check if have
	// reached the beginning/end of history
	versions, err := version.Find(
		db.Query(versionQuery).WithFields(
			version.IdKey,
			version.RevisionOrderNumberKey,
			version.RevisionKey,
			version.MessageKey,
			version.CreateTimeKey,
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

// Returns tasks grouped by their versions, and sorted with the most
// recent first (i.e. descending commit order number).
func (iter *taskHistoryIterator) GetChunk(v *version.Version, numBefore, numAfter int, include bool) (TaskHistoryChunk, error) {
	session, database, err := db.GetGlobalSessionFactory().GetSession()
	defer session.Close()

	chunk := TaskHistoryChunk{
		Tasks:       []bson.M{},
		Versions:    []version.Version{},
		FailedTests: map[string][]TestResult{},
	}

	versionsBefore, exhausted, err := iter.findAllVersions(v, numBefore, true, include)
	if err != nil {
		return chunk, err
	}
	chunk.Exhausted.Before = exhausted

	versionsAfter, exhausted, err := iter.findAllVersions(v, numAfter, false, false)
	if err != nil {
		return chunk, err
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

	pipeline := database.C(TasksCollection).Pipe(
		[]bson.M{
			{"$match": bson.M{
				TaskRequesterKey:    evergreen.RepotrackerVersionRequester,
				TaskProjectKey:      iter.ProjectName,
				TaskDisplayNameKey:  iter.TaskName,
				TaskBuildVariantKey: bson.M{"$in": iter.BuildVariants},
				TaskRevisionOrderNumberKey: bson.M{
					"$gte": versionEndBoundary.RevisionOrderNumber,
					"$lte": versionStartBoundary.RevisionOrderNumber,
				},
			}},
			{"$project": bson.M{
				TaskIdKey:                  1,
				TaskStatusKey:              1,
				TaskStatusDetailsKey:       1,
				TaskActivatedKey:           1,
				TaskTimeTakenKey:           1,
				TaskBuildVariantKey:        1,
				TaskRevisionKey:            1,
				TaskRevisionOrderNumberKey: 1,
			}},
			{"$group": bson.M{
				"_id":   fmt.Sprintf("$%v", TaskRevisionKey),
				"order": bson.M{"$first": fmt.Sprintf("$%v", TaskRevisionOrderNumberKey)},
				"tasks": bson.M{
					"$push": bson.M{
						TaskIdKey:            fmt.Sprintf("$%v", TaskIdKey),
						TaskStatusKey:        fmt.Sprintf("$%v", TaskStatusKey),
						TaskStatusDetailsKey: fmt.Sprintf("$%v", TaskStatusDetailsKey),
						TaskActivatedKey:     fmt.Sprintf("$%v", TaskActivatedKey),
						TaskTimeTakenKey:     fmt.Sprintf("$%v", TaskTimeTakenKey),
						TaskBuildVariantKey:  fmt.Sprintf("$%v", TaskBuildVariantKey),
					},
				},
			}},
			{"$sort": bson.M{TaskRevisionOrderNumberKey: -1}},
		},
	)

	var aggregatedTasks []bson.M
	if err := pipeline.All(&aggregatedTasks); err != nil {
		return chunk, err
	}
	chunk.Tasks = aggregatedTasks

	failedTests, err := iter.GetFailedTests(pipeline)
	if err != nil {
		return chunk, err
	}
	chunk.FailedTests = failedTests
	return chunk, nil
}

func (self *taskHistoryIterator) GetDistinctTestNames(numCommits int) ([]string, error) {
	session, db, err := db.GetGlobalSessionFactory().GetSession()
	defer session.Close()

	pipeline := db.C(TasksCollection).Pipe(
		[]bson.M{
			{
				"$match": bson.M{
					TaskBuildVariantKey: bson.M{"$in": self.BuildVariants},
					TaskDisplayNameKey:  self.TaskName,
				},
			},
			bson.M{"$sort": bson.D{{TaskRevisionOrderNumberKey, -1}}},
			bson.M{"$limit": numCommits},
			bson.M{"$unwind": fmt.Sprintf("$%v", TaskTestResultsKey)},
			bson.M{"$group": bson.M{"_id": fmt.Sprintf("$%v.%v", TaskTestResultsKey, TestResultTestFileKey)}},
		},
	)

	var output []bson.M
	err = pipeline.All(&output)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0)
	for _, doc := range output {
		names = append(names, doc["_id"].(string))
	}

	return names, nil
}

// GetFailedTests returns a mapping of task id to a slice of failed tasks
// extracted from a pipeline of aggregated tasks
func (self *taskHistoryIterator) GetFailedTests(aggregatedTasks *mgo.Pipe) (map[string][]TestResult, error) {
	// get the ids of the failed task
	var failedTaskIds []string
	var taskHistory TaskHistory
	iter := aggregatedTasks.Iter()
	for {
		if iter.Next(&taskHistory) {
			for _, task := range taskHistory.Tasks {
				if task.Status == evergreen.TaskFailed {
					failedTaskIds = append(failedTaskIds, task.Id)
				}
			}
		} else {
			break
		}
	}

	if err := iter.Err(); err != nil {
		return nil, err
	}

	// find all the relevant failed tests
	failedTestsMap := make(map[string][]TestResult)
	tasks, err := FindAllTasks(
		bson.M{
			TaskIdKey: bson.M{
				"$in": failedTaskIds,
			},
		},
		bson.M{
			TaskIdKey:          1,
			TaskTestResultsKey: 1,
		},
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
	if err != nil {
		return nil, err
	}

	// create the mapping of the task id to the list of failed tasks
	for _, task := range tasks {
		for _, test := range task.TestResults {
			if test.Status == evergreen.TestFailedStatus {
				failedTestsMap[task.Id] = append(failedTestsMap[task.Id], test)
			}
		}
	}
	return failedTestsMap, nil
}
