package grid

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"go.mongodb.org/mongo-driver/bson"
)

const testResultsKey = "test_results"

// CellId represents a unique identifier for each cell on
// the Grid page.
type CellId struct {
	Task    string `bson:"d" json:"task"`
	Variant string `bson:"v" json:"variant"`
}

// CellHistory holds historical data for each cell on the
// Grid page.
type CellHistory struct {
	Id            string                  `bson:"d" json:"id"`
	Revision      string                  `bson:"v" json:"revision"`
	Status        string                  `bson:"s" json:"status"`
	StatusDetails apimodels.TaskEndDetail `bson:"e" json:"task_end_details"`
}

// Cell contains information for each cell on the Grid page.
// It includes both the cell's identifier and its history.
type Cell struct {
	Id      CellId        `bson:"_id" json:"cellId"`
	History []CellHistory `bson:"h" json:"history"`
}

// Grid is a slice of cells.
type Grid []Cell

type FailureId struct {
	Test string `bson:"t" json:"test"`
	Task string `bson:"d" json:"task"`
}

// VariantInfo holds information for each variant a test fails on.
type VariantInfo struct {
	Name   string `bson:"n" json:"name"`
	TaskId string `bson:"i" json:"task_id"`
}

// Failure contains data for a particular test failure - including
// the test's task display name, and the variants its failing on.
type Failure struct {
	Id       FailureId     `bson:"_id" json:"identifier"`
	Variants []VariantInfo `bson:"a" json:"variants"`
}

// Revision failure contains the revision and a list of the
// task failures that exist on that Version
type RevisionFailure struct {
	Id       string        `bson:"_id" json:"revision"`
	Failures []TaskFailure `bson:"a" json:"failures"`
}

// TaskFailure has the information needed to be displayed on
// the revision failures tab.
type TaskFailure struct {
	BuildVariant string `bson:"n" json:"variant"`
	TestName     string `bson:"i" json:"test"`
	TaskName     string `bson:"t" json:"task"`
	TaskId       string `bson:"tid" json:"task_id"`
}

// Failures holds failures.
type Failures []Failure

// RevisionFailures holds revision failures
type RevisionFailures []RevisionFailure

// FetchCells returns a Grid of Cells - grouped by variant and display name.
// current is the most recent version and from which to fetch prior Cells
// going back as far as depth versions.
func FetchCells(current model.Version, depth int) (Grid, error) {
	cells := Grid{}
	pipeline := []bson.M{
		// Stage 1: Get all builds from the current version going back
		// as far as depth versions.
		{"$match": bson.M{
			build.RequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			build.RevisionOrderNumberKey: bson.M{
				"$lte": current.RevisionOrderNumber,
				"$gte": (current.RevisionOrderNumber - depth),
			},
			build.ProjectKey: current.Identifier,
		}},
		// Stage 2: Sort the builds by the most recently completed.
		{"$sort": bson.M{
			build.RevisionOrderNumberKey: -1,
		}},
		// Stage 3: Project only the relevant fields.
		{"$project": bson.M{
			build.TasksKey:    1,
			build.RevisionKey: 1,
			"v":               "$" + build.BuildVariantKey,
		}},
		// Stage 4: Flatten the task cache for easier grouping.
		{"$unwind": "$tasks"},
		// Stage 5: Rewrite and project out only the relevant fields.
		{"$project": bson.M{
			"_id": 0,
			"v":   1,
			"r":   "$" + build.RevisionKey,
			"d":   "$" + build.TasksKey + "." + build.TaskCacheDisplayNameKey,
			"st":  "$" + build.TasksKey + "." + build.TaskCacheStatusKey,
			"ed":  "$" + build.TasksKey + "." + build.TaskCacheStatusDetailsKey,
			"id":  "$" + build.TasksKey + "." + build.TaskCacheIdKey,
		}},
		// Stage 6: Group the tasks by variant and display name. For each group,
		// add the history - all prior versioned tasks along with their status,
		// id, and revision identifier.
		{"$group": bson.M{
			"_id": bson.M{
				"v": "$v",
				"d": "$d",
			},
			"h": bson.M{
				"$push": bson.M{
					"s": "$st",
					"e": "$ed",
					"d": "$id",
					"v": "$r",
				},
			},
		}},
	}
	return cells, db.Aggregate(build.Collection, pipeline, &cells)
}

// FetchFailures returns the most recent test failures that have occurred at or
// before the current version - looking back as far as depth versions.
func FetchFailures(current model.Version, depth int) (Failures, error) {
	pipeline := []bson.M{
		// Get the most recent completed tasks - looking back as far as
		// depth versions - on this project.
		{"$match": bson.M{
			task.RevisionOrderNumberKey: bson.M{
				"$lte": current.RevisionOrderNumber,
				"$gte": (current.RevisionOrderNumber - depth),
			},
			task.ProjectKey: current.Identifier,
			task.RequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			task.StatusKey: bson.M{
				"$in": []string{
					evergreen.TaskFailed,
					evergreen.TaskSucceeded,
				},
			},
		}},
		// Sort the tasks by the most recently completed.
		{"$sort": bson.M{
			task.RevisionOrderNumberKey: -1,
		}},
		// Join test results to this task
		{"$lookup": bson.M{
			"from": testresult.Collection,
			"as":   testResultsKey,
			"let": bson.M{
				"task_id":   "$" + task.IdKey,
				"execution": "$" + task.ExecutionKey,
			},
			"pipeline": []bson.M{{
				"$match": bson.M{
					"$expr": bson.M{
						"$and": []bson.M{
							{"$eq": []string{"$" + testresult.TaskIDKey, "$$task_id"}},
							{"$eq": []string{"$" + testresult.ExecutionKey, "$$execution"}},
						},
					},
				},
			}},
		}},
		// Project only relevant fields.
		{"$project": bson.M{
			task.DisplayNameKey:  1,
			task.BuildVariantKey: 1,
			testResultsKey:       1,
			task.IdKey:           1,
		}},
		// Group these tasks by display name and buildvariant -
		// this returns the most recently completed grouped by task display name
		// and by variant. We take only the first test results (adding its task
		// id) for each task/variant group.
		{"$group": bson.M{
			"_id": bson.M{
				"t": "$" + task.DisplayNameKey,
				"v": "$" + task.BuildVariantKey,
			},
			"l": bson.M{
				"$first": "$" + testResultsKey,
			},
			"tid": bson.M{
				"$first": "$" + task.IdKey,
			},
		}},
		// For each group, filter out those task/variant combinations
		// that don't have at least one test failure in them.
		{"$match": bson.M{
			"l." + task.TestResultStatusKey: evergreen.TestFailedStatus,
		}},
		// Rewrite each task/variant combination from the _id into the
		// top-level. Project only the test name and status for all tests, and
		// add a 'status' literal string to each group. This sets up the
		// documents for redacting in next stage.
		{"$project": bson.M{
			"tid":                             1,
			"t":                               "$_id.t",
			"v":                               "$_id.v",
			"l." + task.TestResultStatusKey:   1,
			"l." + task.TestResultTestFileKey: 1,
			"status": bson.M{
				"$literal": evergreen.TestFailedStatus,
			},
		}},
		// While each test result contains at least one failed test,
		// some other tests may have passed. Prune individual tests that did
		// not fail.
		{"$redact": bson.M{
			"$cond": bson.M{
				"if": bson.M{
					"$eq": []string{
						"$status", evergreen.TestFailedStatus,
					},
				},
				"then": "$$DESCEND",
				"else": "$$PRUNE",
			},
		}},
		// We no longer need the status fields so project only fields
		// we want to return.
		{"$project": bson.M{
			"f":   "$l." + task.TestResultTestFileKey,
			"_id": 0,
			"tid": 1,
			"t":   1,
			"v":   1,
		}},
		// Flatten each failing test so we can group them by all the
		// variants on which they are failing.
		{"$unwind": "$f"},
		// Group individual test failure. For each, add the variants
		// it's failing on (and the accompanying task id) and include task's
		// display name.
		{"$group": bson.M{
			"_id": bson.M{
				"t": "$f",
				"d": "$t",
			},
			"a": bson.M{
				"$push": bson.M{
					"n": "$v",
					"i": "$tid",
				},
			},
		}},
	}
	failures := Failures{}
	return failures, db.Aggregate(task.Collection, pipeline, &failures)
}

// FetchRevisionOrderFailures returns the most recent test failures
// grouped by revision - looking as far back as depth revisions
func FetchRevisionOrderFailures(current model.Version, depth int) (RevisionFailures, error) {
	pipeline := []bson.M{
		// Get the most recent completed tasks - looking back as far as
		// depth versions - on this project.
		{"$match": bson.M{
			task.RevisionOrderNumberKey: bson.M{
				"$lte": current.RevisionOrderNumber,
				"$gte": (current.RevisionOrderNumber - depth),
			},
			task.ProjectKey: current.Identifier,
			task.RequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
		}},
		// Join test results to this task
		{"$lookup": bson.M{
			"from": testresult.Collection,
			"as":   testResultsKey,
			"let": bson.M{
				"task_id":   "$" + task.IdKey,
				"execution": "$" + task.ExecutionKey,
			},
			"pipeline": []bson.M{{
				"$match": bson.M{
					"$expr": bson.M{
						"$and": []bson.M{
							{"$eq": []string{"$" + testresult.TaskIDKey, "$$task_id"}},
							{"$eq": []string{"$" + testresult.ExecutionKey, "$$execution"}},
						},
					},
				},
			}},
		}},
		// Project only relevant fields.
		{"$project": bson.M{
			task.DisplayNameKey:  1,
			task.RevisionKey:     1,
			task.BuildVariantKey: 1,
			task.IdKey:           1,
			"l":                  "$" + testResultsKey,
		}},
		// Flatten out the test results
		{"$unwind": "$l"},
		// Take only failed test results
		{"$match": bson.M{
			"l." + task.TestResultStatusKey: evergreen.TestFailedStatus,
		}},
		// Project only relevant fields including just the test file key
		{"$project": bson.M{
			task.RevisionKey:     1,
			task.BuildVariantKey: 1,
			task.DisplayNameKey:  1,
			task.IdKey:           1,
			"f":                  "$l." + task.TestResultTestFileKey,
		}},
		// Group by revision. For each one include the
		// variant name, task name, task id and test name
		{"$group": bson.M{
			"_id": "$" + task.RevisionKey,
			"a": bson.M{
				"$push": bson.M{
					"n":   "$" + task.BuildVariantKey,
					"i":   "$f",
					"t":   "$" + task.DisplayNameKey,
					"tid": "$" + task.IdKey,
				},
			},
		}},
	}
	taskFailures := RevisionFailures{}
	return taskFailures, db.Aggregate(task.Collection, pipeline, &taskFailures)
}
