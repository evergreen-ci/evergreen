package model

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	TaskTimeout       = "timeout"
	TaskSystemFailure = "sysfail"
	TaskSetupFailure  = "setup-fail"
	testResultsKey    = "test_results"

	// this regex either matches against the exact 'test' string, or
	// against the 'test' string at the end of some kind of filepath.
	testMatchRegex = `(\Q%s\E|.*(\\|/)\Q%s\E)$`
)

type taskHistoryIterator struct {
	TaskName      string
	BuildVariants []string
	ProjectName   string
}

type TaskHistoryChunk struct {
	Tasks       []bson.M
	Versions    []Version
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
	Id               string                   `bson:"_id" json:"_id"`
	Status           string                   `bson:"status" json:"status"`
	Activated        bool                     `bson:"activated" json:"activated"`
	TimeTaken        time.Duration            `bson:"time_taken" json:"time_taken"`
	BuildVariant     string                   `bson:"build_variant" json:"build_variant"`
	LocalTestResults apimodels.TaskEndDetails `bson:"status_details" json:"status_details"`
}
type TaskDetails struct {
	TimedOut bool   `bson:"timed_out"`
	Status   string `bson:"st"`
}

// TestHistoryResult represents what is returned by the aggregation
type TestHistoryResult struct {
	TestFile        string  `bson:"tf"`
	TaskName        string  `bson:"tn"`
	TaskStatus      string  `bson:"task_status"`
	TestStatus      string  `bson:"test_status"`
	Revision        string  `bson:"r"`
	Project         string  `bson:"p"`
	TaskId          string  `bson:"tid"`
	BuildVariant    string  `bson:"bv"`
	StartTime       float64 `bson:"st"`
	EndTime         float64 `bson:"et"`
	Execution       int     `bson:"ex"`
	Url             string  `bson:"url"`
	UrlRaw          string  `bson:"url_r"`
	OldTaskId       string  `bson:"otid"`
	TaskTimedOut    bool    `bson:"to"`
	TaskDetailsType string  `bson:"tdt"`
	LogId           string  `bson:"lid"`
	Order           int     `bson:"order"`
}

// TestHistoryResult bson tags
var (
	TestFileKey        = bsonutil.MustHaveTag(TestHistoryResult{}, "TestFile")
	TaskNameKey        = bsonutil.MustHaveTag(TestHistoryResult{}, "TaskName")
	TaskStatusKey      = bsonutil.MustHaveTag(TestHistoryResult{}, "TaskStatus")
	TestStatusKey      = bsonutil.MustHaveTag(TestHistoryResult{}, "TestStatus")
	RevisionKey        = bsonutil.MustHaveTag(TestHistoryResult{}, "Revision")
	ProjectKey         = bsonutil.MustHaveTag(TestHistoryResult{}, "Project")
	TaskIdKey          = bsonutil.MustHaveTag(TestHistoryResult{}, "TaskId")
	BuildVariantKey    = bsonutil.MustHaveTag(TestHistoryResult{}, "BuildVariant")
	EndTimeKey         = bsonutil.MustHaveTag(TestHistoryResult{}, "EndTime")
	StartTimeKey       = bsonutil.MustHaveTag(TestHistoryResult{}, "StartTime")
	ExecutionKey       = bsonutil.MustHaveTag(TestHistoryResult{}, "Execution")
	OldTaskIdKey       = bsonutil.MustHaveTag(TestHistoryResult{}, "OldTaskId")
	UrlKey             = bsonutil.MustHaveTag(TestHistoryResult{}, "Url")
	UrlRawKey          = bsonutil.MustHaveTag(TestHistoryResult{}, "UrlRaw")
	TaskTimedOutKey    = bsonutil.MustHaveTag(TestHistoryResult{}, "TaskTimedOut")
	TaskDetailsTypeKey = bsonutil.MustHaveTag(TestHistoryResult{}, "TaskDetailsType")
	LogIdKey           = bsonutil.MustHaveTag(TestHistoryResult{}, "LogId")
)

// TestHistoryParameters are the parameters that are used
// to retrieve Test Results.
type TestHistoryParameters struct {
	Sort  int `json:"sort"`
	Limit int `json:"limit"`

	// task parameters
	Project         string    `json:"project"`
	TaskNames       []string  `json:"task_names"`
	BuildVariants   []string  `json:"variants"`
	TaskStatuses    []string  `json:"task_statuses"`
	BeforeRevision  string    `json:"before_revision"`
	AfterRevision   string    `json:"after_revision"`
	TaskRequestType string    `json:"task_request"`
	BeforeDate      time.Time `json:"before_date"`
	AfterDate       time.Time `json:"after_date"`

	// test result parameters
	TestNames    []string `json:"test_names"`
	TestStatuses []string `json:"test_statuses"`
}

func (t TestHistoryParameters) QueryString() string {
	out := []string{
		"testStatuses=" + strings.Join(t.TestStatuses, ","),
		"taskStatuses=" + strings.Join(t.TaskStatuses, ","),
	}

	if t.TaskRequestType != "" {
		out = append(out, "buildType="+t.TaskRequestType)
	}

	if len(t.TaskNames) > 0 {
		out = append(out, "tasks="+strings.Join(t.TaskNames, ","))
	}

	if len(t.TestNames) > 0 {
		out = append(out, "tests="+strings.Join(t.TestNames, ","))
	}

	if len(t.BuildVariants) > 0 {
		out = append(out, "variants="+strings.Join(t.BuildVariants, ","))
	}

	if t.BeforeRevision != "" {
		out = append(out, "beforeRevision="+t.BeforeRevision)
	}

	if t.AfterRevision != "" {
		out = append(out, "afterRevision="+t.AfterRevision)
	}
	if !util.IsZeroTime(t.BeforeDate) {
		out = append(out, "beforeDate="+t.BeforeDate.Format(time.RFC3339))
	}
	if !util.IsZeroTime(t.AfterDate) {
		out = append(out, "afterDate="+t.AfterDate.Format(time.RFC3339))
	}

	if t.Limit != 0 {
		out = append(out, fmt.Sprintf("limit=%d", t.Limit))
	}

	return strings.Join(out, "&")
}

type TaskHistoryIterator interface {
	GetChunk(version *Version, numBefore, numAfter int, include bool) (TaskHistoryChunk, error)
	GetDistinctTestNames(numCommits int) ([]string, error)
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

// Returns tasks grouped by their versions, and sorted with the most
// recent first (i.e. descending commit order number).
func (iter *taskHistoryIterator) GetChunk(v *Version, numBefore, numAfter int, include bool) (TaskHistoryChunk, error) {
	chunk := TaskHistoryChunk{
		Tasks:       []bson.M{},
		Versions:    []Version{},
		FailedTests: map[string][]task.TestResult{},
	}

	session, database, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		return chunk, errors.Wrap(err, "problem getting database session")
	}
	defer session.Close()

	session.SetSocketTimeout(time.Minute)

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
	}

	pipeline := []bson.M{
		{"$match": matchStage},
		{"$project": projectStage},
		{"$group": groupStage},
		{"$sort": bson.M{task.RevisionOrderNumberKey: -1}},
	}
	agg := database.C(task.Collection).Pipe(pipeline)
	var aggregatedTasks []bson.M
	if err = agg.All(&aggregatedTasks); err != nil {
		return chunk, errors.WithStack(err)
	}
	chunk.Tasks = aggregatedTasks

	failedTests, err := iter.GetFailedTests(agg)
	if err != nil {
		return chunk, errors.WithStack(err)
	}

	chunk.FailedTests = failedTests
	return chunk, nil
}

func (self *taskHistoryIterator) GetDistinctTestNames(numCommits int) ([]string, error) {
	session, mdb, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		return nil, errors.Wrap(err, "problem getting database session")
	}
	defer session.Close()

	session.SetSocketTimeout(time.Minute)

	pipeline := mdb.C(task.Collection).Pipe(
		[]bson.M{
			{
				"$match": bson.M{
					task.BuildVariantKey: bson.M{"$in": self.BuildVariants},
					task.DisplayNameKey:  self.TaskName,
				},
			},
			{"$sort": bson.D{{Key: task.RevisionOrderNumberKey, Value: -1}}},
			{"$limit": numCommits},
			{"$lookup": bson.M{
				"from":         testresult.Collection,
				"localField":   task.IdKey,
				"foreignField": testresult.TaskIDKey,
				"as":           testResultsKey},
			},
			{"$project": bson.M{
				testResultsKey: bson.M{
					"$filter": bson.M{
						// Filter off non-matching executions. This should be replaced once
						// multi-key $lookups are supported in 3.6
						"input": "$" + testResultsKey,
						"as":    "tr",
						"cond": bson.M{
							"$eq": []string{"$$tr.task_execution", "$execution"}},
					},
				},
				task.IdKey:                 1,
				task.TestResultTestFileKey: 1,
			}},
			{"$unwind": fmt.Sprintf("$%v", testResultsKey)},
			{"$group": bson.M{"_id": fmt.Sprintf("$%v.%v", testResultsKey, task.TestResultTestFileKey)}},
		},
	)

	var output []bson.M

	if err = pipeline.All(&output); err != nil {
		return nil, errors.WithStack(err)
	}

	names := make([]string, 0)
	for _, doc := range output {
		names = append(names, doc["_id"].(string))
	}

	return names, nil
}

// GetFailedTests returns a mapping of task id to a slice of failed tasks
// extracted from a pipeline of aggregated tasks
func (self *taskHistoryIterator) GetFailedTests(aggregatedTasks adb.Results) (map[string][]task.TestResult, error) {
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
		return nil, errors.WithStack(err)
	}

	if failedTaskIds == nil {
		// this is an added hack to make tests pass when
		// transitioning between mongodb drivers
		return nil, nil
	}

	// find all the relevant failed tests
	failedTestsMap := make(map[string][]task.TestResult)
	tasks, err := task.Find(task.ByIds(failedTaskIds))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// create the mapping of the task id to the list of failed tasks
	for _, task := range tasks {
		if err := task.MergeNewTestResults(); err != nil {
			return nil, err
		}
		for _, test := range task.LocalTestResults {
			if test.Status == evergreen.TestFailedStatus {
				failedTestsMap[task.Id] = append(failedTestsMap[task.Id], test)
			}
		}
	}

	return failedTestsMap, nil
}

// validate returns a list of validation error messages if there are any validation errors
// and an empty list if there are none.
// It checks that there is not both a date and revision time range,
// checks that sort is either -1 or 1,
// checks that the test statuses and task statuses are valid test or task statuses,
// checks that there is a project id and either a list of test names or task names.
func (t *TestHistoryParameters) validate() []string {
	validationErrors := []string{}
	if t.Project == "" {
		validationErrors = append(validationErrors, "no project id specified")
	}

	if len(t.TestNames) == 0 && len(t.TaskNames) == 0 {
		validationErrors = append(validationErrors, "must include test names or task names")
	}
	// A test can either have failed, silently failed, got skipped, or passed.
	validTestStatuses := []string{
		evergreen.TestFailedStatus,
		evergreen.TestSilentlyFailedStatus,
		evergreen.TestSkippedStatus,
		evergreen.TestSucceededStatus,
	}
	for _, status := range t.TestStatuses {
		if !util.StringSliceContains(validTestStatuses, status) {
			validationErrors = append(validationErrors, fmt.Sprintf("invalid test status in parameters: %v", status))
		}
	}

	// task statuses can be fail, pass, or timeout.
	validTaskStatuses := []string{evergreen.TaskFailed, evergreen.TaskSucceeded, TaskTimeout, TaskSystemFailure}
	for _, status := range t.TaskStatuses {
		if !util.StringSliceContains(validTaskStatuses, status) {
			validationErrors = append(validationErrors, fmt.Sprintf("invalid task status in parameters: %v", status))
		}
	}

	if (!util.IsZeroTime(t.AfterDate) || !util.IsZeroTime(t.BeforeDate)) &&
		(t.AfterRevision != "" || t.BeforeRevision != "") {
		validationErrors = append(validationErrors, "cannot have both date and revision time range parameter")
	}

	if t.Sort != -1 && t.Sort != 1 {
		validationErrors = append(validationErrors, "sort parameter can only be -1 or 1")
	}

	if t.BeforeRevision == "" && t.AfterRevision == "" && t.Limit == 0 {
		validationErrors = append(validationErrors, "must specify a range of revisions *or* a limit")
	}
	return validationErrors
}

// setDefaultsAndValidate sets the default for test history parameters that do not have values
// and validates the test parameters.
func (thp *TestHistoryParameters) SetDefaultsAndValidate() error {
	if len(thp.TestStatuses) == 0 {
		thp.TestStatuses = []string{evergreen.TestFailedStatus}
	}
	if len(thp.TaskStatuses) == 0 {
		thp.TaskStatuses = []string{evergreen.TaskFailed}
	}
	if thp.Sort == 0 {
		thp.Sort = -1
	}

	validationErrors := thp.validate()
	if len(validationErrors) > 0 {
		return errors.Errorf("validation error on test history parameters: %s",
			strings.Join(validationErrors, ", "))
	}
	return nil
}

// mergeResults merges the test results from the old tests and current tests so that all test results with the same
// test file name and task id are adjacent to each other.
// Since the tests results returned in the aggregation are sorted in the same way for both the tasks and old_tasks collection,
// the sorted format should be the same - this is assuming that currentTestHistory and oldTestHistory are both sorted.
func mergeResults(currentTestHistory []TestHistoryResult, oldTestHistory []TestHistoryResult) []TestHistoryResult {
	if len(oldTestHistory) == 0 {
		return currentTestHistory
	}
	if len(currentTestHistory) == 0 {
		return oldTestHistory
	}

	allResults := []TestHistoryResult{}
	oldIndex := 0

	for _, testResult := range currentTestHistory {
		// first add the element of the latest execution
		allResults = append(allResults, testResult)

		// check that there are more test results in oldTestHistory;
		// check if the old task id, is the same as the original task id of the current test result
		// and that the test file is the same.
		for oldIndex < len(oldTestHistory) &&
			oldTestHistory[oldIndex].OldTaskId == testResult.TaskId &&
			oldTestHistory[oldIndex].TestFile == testResult.TestFile {
			allResults = append(allResults, oldTestHistory[oldIndex])

			oldIndex += 1
		}
	}
	return allResults
}

// buildTestHistoryQuery returns the aggregation pipeline that is executed given the test history parameters.
func buildTestHistoryQuery(testHistoryParameters *TestHistoryParameters) ([]bson.M, error) {
	// construct the task match query
	taskMatchQuery := bson.M{
		task.ProjectKey: testHistoryParameters.Project,
	}

	// construct the test match query
	testMatchQuery := bson.M{
		testResultsKey + "." + testresult.StatusKey: bson.M{"$in": testHistoryParameters.TestStatuses},
	}

	// separate out pass/fail from timeouts and system failures
	isTimeout := false
	isSysFail := false
	taskStatuses := []string{}
	for _, status := range testHistoryParameters.TaskStatuses {
		switch status {
		case TaskTimeout:
			isTimeout = true
		case TaskSystemFailure:
			isSysFail = true
		default:
			taskStatuses = append(taskStatuses, status)
		}
	}
	statusQuery := []bson.M{}

	// if there are any pass/fail tasks create a query that isn't a timeout or a system failure.
	if len(taskStatuses) > 0 {
		statusQuery = append(statusQuery,
			bson.M{
				task.StatusKey: bson.M{"$in": taskStatuses},
				task.DetailsKey + "." + task.TaskEndDetailTimedOut: bson.M{
					"$ne": true,
				},
				task.DetailsKey + "." + task.TaskEndDetailType: bson.M{
					"$ne": evergreen.CommandTypeSystem,
				},
			})
	}

	if isTimeout {
		statusQuery = append(statusQuery, bson.M{
			task.StatusKey: evergreen.TaskFailed,
			task.DetailsKey + "." + task.TaskEndDetailTimedOut: true,
		})
	}
	if isSysFail {
		statusQuery = append(statusQuery, bson.M{
			task.StatusKey: evergreen.TaskFailed,
			task.DetailsKey + "." + task.TaskEndDetailType: evergreen.CommandTypeSystem,
		})
	}

	if testHistoryParameters.TaskRequestType != "" {
		taskMatchQuery[task.RequesterKey] = testHistoryParameters.TaskRequestType
	}

	taskMatchQuery["$or"] = statusQuery

	// check task, test, and build variants  and add them to the task query if necessary
	if len(testHistoryParameters.TaskNames) > 0 {
		taskMatchQuery[task.DisplayNameKey] = bson.M{"$in": testHistoryParameters.TaskNames}
	}
	if len(testHistoryParameters.BuildVariants) > 0 {
		taskMatchQuery[task.BuildVariantKey] = bson.M{"$in": testHistoryParameters.BuildVariants}
	}
	if len(testHistoryParameters.TestNames) > 0 {
		testMatchQuery[testResultsKey+"."+testresult.TestFileKey] = bson.M{"$in": testHistoryParameters.TestNames}
	}

	// add in date to  task query if necessary
	if !util.IsZeroTime(testHistoryParameters.BeforeDate) || !util.IsZeroTime(testHistoryParameters.AfterDate) {
		startTimeClause := bson.M{}
		if !util.IsZeroTime(testHistoryParameters.BeforeDate) {
			startTimeClause["$lte"] = testHistoryParameters.BeforeDate
		}
		if !util.IsZeroTime(testHistoryParameters.AfterDate) {
			startTimeClause["$gte"] = testHistoryParameters.AfterDate
		}
		taskMatchQuery[task.StartTimeKey] = startTimeClause
	}

	var pipeline []bson.M

	// we begin to build the pipeline here. This if/else clause
	// builds the initial match and limit. This returns early if
	// you do not specify a revision range or a limit; and issues
	// a warning if you specify only *one* bound without a limit.
	//
	// This operation will return an error if the before or after
	// revision are empty.
	if testHistoryParameters.BeforeRevision == "" && testHistoryParameters.AfterRevision == "" {
		if testHistoryParameters.Limit == 0 {
			return nil, errors.New("must specify a range of revisions *or* a limit")
		}

		pipeline = append(pipeline,
			bson.M{"$match": taskMatchQuery})
	} else {
		//  add in revision to task query if necessary

		revisionOrderNumberClause := bson.M{}
		if testHistoryParameters.BeforeRevision != "" {
			v, err := VersionFindOne(VersionByProjectIdAndRevision(testHistoryParameters.Project,
				testHistoryParameters.BeforeRevision).WithFields(VersionRevisionOrderNumberKey))
			if err != nil {
				return nil, err
			}
			if v == nil {
				return nil, errors.Errorf("invalid revision : %v", testHistoryParameters.BeforeRevision)
			}
			revisionOrderNumberClause["$lte"] = v.RevisionOrderNumber
		}

		if testHistoryParameters.AfterRevision != "" {
			v, err := VersionFindOne(VersionByProjectIdAndRevision(testHistoryParameters.Project,
				testHistoryParameters.AfterRevision).WithFields(VersionRevisionOrderNumberKey))
			if err != nil {
				return nil, err
			}
			if v == nil {
				return nil, errors.Errorf("invalid revision : %v", testHistoryParameters.AfterRevision)
			}
			revisionOrderNumberClause["$gt"] = v.RevisionOrderNumber
		}
		taskMatchQuery[task.RevisionOrderNumberKey] = revisionOrderNumberClause

		pipeline = append(pipeline, bson.M{"$match": taskMatchQuery})

		if testHistoryParameters.Limit == 0 && len(revisionOrderNumberClause) != 2 {
			grip.Notice("task history query contains a potentially unbounded range of revisions")
		}
	}

	pipeline = append(pipeline,
		bson.M{"$lookup": bson.M{
			"from":         testresult.Collection,
			"localField":   task.IdKey,
			"foreignField": testresult.TaskIDKey,
			"as":           testResultsKey},
		},
		bson.M{"$project": bson.M{
			testResultsKey: bson.M{
				"$filter": bson.M{
					// Filter off non-matching executions. This should be replaced once
					// multi-key $lookups are supported in 3.6
					"input": "$" + testResultsKey,
					"as":    "tr",
					"cond": bson.M{
						"$eq": []string{"$$tr.task_execution", "$execution"}},
				},
			},
			task.DisplayNameKey:         1,
			task.BuildVariantKey:        1,
			task.StatusKey:              1,
			task.RevisionKey:            1,
			task.IdKey:                  1,
			task.ExecutionKey:           1,
			task.RevisionOrderNumberKey: 1,
			task.OldTaskIdKey:           1,
			task.StartTimeKey:           1,
			task.ProjectKey:             1,
			task.DetailsKey:             1,
		}},
		bson.M{"$unwind": "$test_results"},
		bson.M{"$match": testMatchQuery},
		bson.M{"$sort": bson.D{
			{Key: task.RevisionOrderNumberKey, Value: testHistoryParameters.Sort},
			{Key: testResultsKey + "." + testresult.TaskIDKey, Value: testHistoryParameters.Sort},
			{Key: testResultsKey + "." + testresult.TestFileKey, Value: testHistoryParameters.Sort},
		}})
	if testHistoryParameters.Limit > 0 {
		pipeline = append(pipeline, bson.M{"$limit": testHistoryParameters.Limit})
	}
	pipeline = append(pipeline,
		bson.M{"$project": bson.M{
			TestFileKey:        "$" + testResultsKey + "." + task.TestResultTestFileKey,
			TaskIdKey:          "$" + task.IdKey,
			TestStatusKey:      "$" + testResultsKey + "." + task.TestResultStatusKey,
			TaskStatusKey:      "$" + task.StatusKey,
			RevisionKey:        "$" + task.RevisionKey,
			ProjectKey:         "$" + task.ProjectKey,
			TaskNameKey:        "$" + task.DisplayNameKey,
			BuildVariantKey:    "$" + task.BuildVariantKey,
			StartTimeKey:       "$" + testResultsKey + "." + task.TestResultStartTimeKey,
			EndTimeKey:         "$" + testResultsKey + "." + task.TestResultEndTimeKey,
			ExecutionKey:       "$" + task.ExecutionKey + "." + task.ExecutionKey,
			OldTaskIdKey:       "$" + task.OldTaskIdKey,
			UrlKey:             "$" + testResultsKey + "." + task.TestResultURLKey,
			UrlRawKey:          "$" + testResultsKey + "." + task.TestResultURLRawKey,
			LogIdKey:           "$" + testResultsKey + "." + task.TestResultLogIdKey,
			TaskTimedOutKey:    "$" + task.DetailsKey + "." + task.TaskEndDetailTimedOut,
			TaskDetailsTypeKey: "$" + task.DetailsKey + "." + task.TaskEndDetailType,
		}})

	return pipeline, nil
}

func testHistoryV2Results(params *TestHistoryParameters) ([]task.Task, error) {
	if params == nil {
		return nil, errors.New("unable to determine parameters for test history query")
	}
	// run just the part of the query on the tasks collection to form our starting result set
	tasksQuery, err := formQueryFromTasks(params)
	if err != nil {
		return nil, err
	}
	projection := bson.M{
		task.DisplayNameKey:         1,
		task.BuildVariantKey:        1,
		task.StatusKey:              1,
		task.RevisionKey:            1,
		task.IdKey:                  1,
		task.ExecutionKey:           1,
		task.RevisionOrderNumberKey: 1,
		task.OldTaskIdKey:           1,
		task.StartTimeKey:           1,
		task.FinishTimeKey:          1,
		task.ProjectKey:             1,
		task.DetailsKey:             1,
	}
	tasks, err := task.Find(db.Query(tasksQuery).Project(projection))
	if err != nil {
		return nil, err
	}
	oldTasks, err := task.FindOld(db.Query(tasksQuery).Project(projection))
	if err != nil {
		return nil, err
	}
	tasks = append(tasks, oldTasks...)
	taskIds := []string{}
	for _, t := range tasks {
		taskIds = append(taskIds, t.Id)
	}

	// to join the test results, merge test results for all the tasks
	testQuery := db.Query(formTestsQuery(params, taskIds))
	out, err := task.MergeTestResultsBulk(tasks, &testQuery)
	if err != nil {
		return nil, errors.Wrap(err, "error merging test results")
	}
	return out, nil
}

func formQueryFromTasks(params *TestHistoryParameters) (bson.M, error) {
	query := bson.M{}
	if len(params.TaskNames) > 0 {
		query[task.DisplayNameKey] = bson.M{"$in": params.TaskNames}
	}
	if len(params.Project) > 0 {
		query[task.ProjectKey] = params.Project
	}
	if len(params.TaskRequestType) > 0 {
		query[task.RequesterKey] = params.TaskRequestType
	}
	if !util.IsZeroTime(params.BeforeDate) || !util.IsZeroTime(params.AfterDate) {
		startTimeClause := bson.M{}
		if !util.IsZeroTime(params.BeforeDate) {
			startTimeClause["$lte"] = params.BeforeDate
		}
		if !util.IsZeroTime(params.AfterDate) {
			startTimeClause["$gte"] = params.AfterDate
		}
		query[task.StartTimeKey] = startTimeClause
	}
	if len(params.BuildVariants) > 0 {
		query[task.BuildVariantKey] = bson.M{"$in": params.BuildVariants}
	}
	statusQuery := formTaskStatusQuery(params)
	if len(statusQuery) > 0 {
		query["$or"] = statusQuery
	}
	revisionQuery, err := formRevisionQuery(params)
	if err != nil {
		return bson.M{}, errors.WithStack(err)
	}
	if revisionQuery != nil {
		query[task.RevisionOrderNumberKey] = *revisionQuery
	}

	return query, nil
}

func formTestsQuery(params *TestHistoryParameters, taskIds []string) bson.M {
	query := bson.M{
		testresult.TaskIDKey: bson.M{
			"$in": taskIds,
		},
	}
	if len(params.TestNames) > 0 {
		query[testresult.TestFileKey] = bson.M{
			"$in": params.TestNames,
		}
	}
	if len(params.TestStatuses) > 0 {
		query[testresult.StatusKey] = bson.M{
			"$in": params.TestStatuses,
		}
	}

	return query
}

func formTaskStatusQuery(params *TestHistoryParameters) []bson.M {
	// separate out pass/fail from timeouts and system failures
	isTimeout := false
	isSysFail := false
	isSuccess := false
	taskStatuses := []string{}
	for _, status := range params.TaskStatuses {
		switch status {
		case TaskTimeout:
			isTimeout = true
		case TaskSystemFailure:
			isSysFail = true
		case evergreen.TaskSucceeded:
			isSuccess = true
		default:
			taskStatuses = append(taskStatuses, status)
		}
	}
	statusQuery := []bson.M{}

	// if there are any pass/fail tasks create a query that isn't a timeout or a system failure.
	if len(taskStatuses) > 0 {
		statusQuery = append(statusQuery,
			bson.M{
				task.StatusKey: bson.M{"$in": taskStatuses},
				task.DetailsKey + "." + task.TaskEndDetailTimedOut: bson.M{
					"$ne": true,
				},
				task.DetailsKey + "." + task.TaskEndDetailType: bson.M{
					"$ne": evergreen.CommandTypeSystem,
				},
			})
	}

	if isTimeout {
		statusQuery = append(statusQuery, bson.M{
			task.StatusKey: evergreen.TaskFailed,
			task.DetailsKey + "." + task.TaskEndDetailTimedOut: true,
		})
	}
	if isSysFail {
		statusQuery = append(statusQuery, bson.M{
			task.StatusKey: evergreen.TaskFailed,
			task.DetailsKey + "." + task.TaskEndDetailType: evergreen.CommandTypeSystem,
		})
	}
	if isSuccess {
		statusQuery = append(statusQuery, bson.M{
			task.StatusKey: evergreen.TaskSucceeded,
		})
	}

	return statusQuery
}

func formRevisionQuery(params *TestHistoryParameters) (*bson.M, error) {
	if params.BeforeRevision == "" && params.AfterRevision == "" {
		return nil, nil
	}
	revisionOrderNumberClause := bson.M{}
	if params.BeforeRevision != "" {
		v, err := VersionFindOne(VersionByProjectIdAndRevision(params.Project,
			params.BeforeRevision).WithFields(VersionRevisionOrderNumberKey))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if v == nil {
			return nil, errors.Errorf("invalid revision : %v", params.BeforeRevision)
		}
		revisionOrderNumberClause["$lte"] = v.RevisionOrderNumber
	}

	if params.AfterRevision != "" {
		v, err := VersionFindOne(VersionByProjectIdAndRevision(params.Project,
			params.AfterRevision).WithFields(VersionRevisionOrderNumberKey))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if v == nil {
			return nil, errors.Errorf("invalid revision : %v", params.AfterRevision)
		}
		revisionOrderNumberClause["$gt"] = v.RevisionOrderNumber
	}
	return &revisionOrderNumberClause, nil
}

// GetTestHistory takes in test history parameters, validates them, and returns the test results according to those parameters.
// It sets tasks failed and tests failed as default statuses if none are provided, and defaults to all tasks, tests,
// and variants if those are not set.
func GetTestHistory(testHistoryParameters *TestHistoryParameters) ([]TestHistoryResult, error) {
	pipeline, err := buildTestHistoryQuery(testHistoryParameters)
	if err != nil {
		return nil, err
	}
	aggTestResults := []TestHistoryResult{}
	err = db.Aggregate(task.Collection, pipeline, &aggTestResults)
	if err != nil {
		return nil, err
	}
	aggOldTestResults := []TestHistoryResult{}
	err = db.Aggregate(task.OldCollection, pipeline, &aggOldTestResults)
	if err != nil {
		return nil, err
	}
	return mergeResults(aggTestResults, aggOldTestResults), nil
}

func GetTestHistoryV2(testHistoryParameters *TestHistoryParameters) ([]TestHistoryResult, error) {
	var results []TestHistoryResult
	tasks, err := testHistoryV2Results(testHistoryParameters)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for _, t := range tasks {
		for _, result := range t.LocalTestResults {
			results = append(results, TestHistoryResult{
				TaskId:          t.Id,
				TaskName:        t.DisplayName,
				TaskStatus:      t.Status,
				Revision:        t.Revision,
				Order:           t.RevisionOrderNumber,
				Project:         t.Project,
				BuildVariant:    t.BuildVariant,
				Execution:       t.Execution,
				OldTaskId:       t.OldTaskId,
				TaskTimedOut:    t.Details.TimedOut,
				TaskDetailsType: t.Details.Type,
				TestFile:        result.TestFile,
				TestStatus:      result.Status,
				Url:             result.URL,
				UrlRaw:          result.URLRaw,
				LogId:           result.LogId,
				StartTime:       result.StartTime,
				EndTime:         result.EndTime,
			})
		}
	}

	sort.Sort(historyResultSorter(results))
	limit := testHistoryParameters.Limit
	var out []TestHistoryResult
	if limit == 0 {
		limit = len(results)
	}
	for i := range results {
		index := i
		if testHistoryParameters.Sort < 0 {
			index = len(results) - i - 1
		}
		out = append(out, results[index])
		if i >= limit-1 {
			break
		}
	}

	return out, nil
}

type historyResultSorter []TestHistoryResult

func (h historyResultSorter) Len() int      { return len(h) }
func (h historyResultSorter) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historyResultSorter) Less(i, j int) bool {
	if h[i].Order == h[j].Order {
		if h[i].TaskId == h[j].TaskId {
			return h[i].TestFile < h[j].TestFile
		}
		return h[i].TaskId < h[j].TaskId
	}
	return h[i].Order < h[j].Order
}

type PickaxeParams struct {
	Project           *Project
	TaskName          string
	NewestOrder       int64
	OldestOrder       int64
	BuildVariants     []string
	Tests             map[string]string
	OnlyMatchingTasks bool
}

func TaskHistoryPickaxe(params PickaxeParams) ([]task.Task, error) {
	// If there are no build variants, use all of them for the given task name.
	// Need this because without the build_variant specified, no amount of hinting
	// will get sort to use the proper index
	repo, err := FindRepository(params.Project.Identifier)
	if err != nil {
		return nil, errors.Wrap(err, "error finding repository")
	}
	if repo == nil {
		return nil, errors.New("unable to find repository")
	}
	grip.Info(repo)
	buildVariants, err := task.FindVariantsWithTask(params.TaskName, params.Project.Identifier, repo.RevisionOrderNumber-50, repo.RevisionOrderNumber)
	if err != nil {
		return nil, errors.Wrap(err, "error finding build variants")
	}
	grip.Notice(buildVariants)
	query := bson.M{
		"build_variant": bson.M{
			"$in": buildVariants,
		},
		"display_name": params.TaskName,
		"order": bson.M{
			"$gte": params.OldestOrder,
			"$lte": params.NewestOrder,
		},
		"branch": params.Project.Identifier,
	}
	// If there are build variants in the filter, use them instead
	if len(params.BuildVariants) > 0 {
		query["build_variant"] = bson.M{
			"$in": params.BuildVariants,
		}
	}
	projection := bson.M{
		"_id":           1,
		"status":        1,
		"activated":     1,
		"time_taken":    1,
		"build_variant": 1,
	}
	last, err := task.Find(db.Query(query).Project(projection))
	if err != nil {
		return nil, errors.Wrap(err, "Error querying tasks")
	}

	taskIds := []string{}
	for _, t := range last {
		taskIds = append(taskIds, t.Id)
	}

	elemMatchOr := []bson.M{}
	for test, result := range params.Tests {
		regexp := fmt.Sprintf(testMatchRegex, test, test)
		if result == "ran" {
			// Special case: if asking for tasks where the test ran, don't care
			// about the test status
			elemMatchOr = append(elemMatchOr, bson.M{
				"test_file": primitive.Regex{Pattern: regexp},
			})
		} else {
			elemMatchOr = append(elemMatchOr, bson.M{
				"test_file": primitive.Regex{Pattern: regexp},
				"status":    result,
			})
		}
	}
	testQuery := db.Query(bson.M{
		"$or": elemMatchOr,
		testresult.TaskIDKey: bson.M{
			"$in": taskIds,
		},
	})
	last, err = task.MergeTestResultsBulk(last, &testQuery)
	if err != nil {
		return nil, errors.Wrap(err, "Error merging test results")
	}
	// if only want matching tasks, remove any tasks that have no test results merged
	if params.OnlyMatchingTasks {
		for i := len(last) - 1; i >= 0; i-- {
			t := last[i]
			if len(t.LocalTestResults) == 0 {
				// if only want matching tasks and didn't find a match, remove the task
				last = append(last[:i], last[i+1:]...)
			}
		}
	}

	return last, nil
}
