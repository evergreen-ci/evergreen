package model

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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
	FailedTests map[string][]task.TestResult
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
		version.RequesterKey:  evergreen.RepotrackerVersionRequester,
		version.IdentifierKey: iter.ProjectName,
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
		FailedTests: map[string][]task.TestResult{},
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

	pipeline := database.C(task.Collection).Pipe(
		[]bson.M{
			{"$match": bson.M{
				task.RequesterKey:    evergreen.RepotrackerVersionRequester,
				task.ProjectKey:      iter.ProjectName,
				task.DisplayNameKey:  iter.TaskName,
				task.BuildVariantKey: bson.M{"$in": iter.BuildVariants},
				task.RevisionOrderNumberKey: bson.M{
					"$gte": versionEndBoundary.RevisionOrderNumber,
					"$lte": versionStartBoundary.RevisionOrderNumber,
				},
			}},
			{"$project": bson.M{
				task.IdKey:                  1,
				task.StatusKey:              1,
				task.DetailsKey:             1,
				task.ActivatedKey:           1,
				task.TimeTakenKey:           1,
				task.BuildVariantKey:        1,
				task.RevisionKey:            1,
				task.RevisionOrderNumberKey: 1,
			}},
			{"$group": bson.M{
				"_id":   fmt.Sprintf("$%v", task.RevisionKey),
				"order": bson.M{"$first": fmt.Sprintf("$%v", task.RevisionOrderNumberKey)},
				"tasks": bson.M{
					"$push": bson.M{
						task.IdKey:           fmt.Sprintf("$%v", task.IdKey),
						task.StatusKey:       fmt.Sprintf("$%v", task.StatusKey),
						task.DetailsKey:      fmt.Sprintf("$%v", task.DetailsKey),
						task.ActivatedKey:    fmt.Sprintf("$%v", task.ActivatedKey),
						task.TimeTakenKey:    fmt.Sprintf("$%v", task.TimeTakenKey),
						task.BuildVariantKey: fmt.Sprintf("$%v", task.BuildVariantKey),
					},
				},
			}},
			{"$sort": bson.M{task.RevisionOrderNumberKey: -1}},
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

	pipeline := db.C(task.Collection).Pipe(
		[]bson.M{
			{
				"$match": bson.M{
					task.BuildVariantKey: bson.M{"$in": self.BuildVariants},
					task.DisplayNameKey:  self.TaskName,
				},
			},
			bson.M{"$sort": bson.D{{task.RevisionOrderNumberKey, -1}}},
			bson.M{"$limit": numCommits},
			bson.M{"$unwind": fmt.Sprintf("$%v", task.TestResultsKey)},
			bson.M{"$group": bson.M{"_id": fmt.Sprintf("$%v.%v", task.TestResultsKey, task.TestResultTestFileKey)}},
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
func (self *taskHistoryIterator) GetFailedTests(aggregatedTasks *mgo.Pipe) (map[string][]task.TestResult, error) {
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
	failedTestsMap := make(map[string][]task.TestResult)
	tasks, err := task.Find(task.ByIds(failedTaskIds).WithFields(task.IdKey, task.TestResultsKey))
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
