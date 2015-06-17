package grid

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/version"
	"gopkg.in/mgo.v2/bson"
)

// CellId represents a unique identifier for each cell on
// the Grid page.
type CellId struct {
	Task    string `bson:"d" json:"task"`
	Variant string `bson:"v" json:"variant"`
}

// CellHistory holds historical data for each cell on the
// Grid page.
type CellHistory struct {
	Id       string `bson:"d" json:"id"`
	Revision string `bson:"v" json:"revision"`
	Status   string `bson:"s" json:"status"`
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

// Failures holds failures.
type Failures []Failure

// FetchCells returns a Grid of Cells - grouped by variant and display name.
// current is the most recent version and from which to fetch prior Cells
// going back as far as depth versions.
func FetchCells(current version.Version, depth int) (Grid, error) {
	cells := Grid{}
	pipeline := []bson.M{
		// Stage 1: Get all builds from the current version going back
		// as far as depth versions.
		{"$match": bson.M{
			build.RequesterKey: evergreen.RepotrackerVersionRequester,
			build.RevisionOrderNumberKey: bson.M{
				"$lte": current.RevisionOrderNumber,
				"$gte": (current.RevisionOrderNumber - depth),
			},
			build.ProjectKey: current.Project,
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
func FetchFailures(current version.Version, depth int) (Failures, error) {
	pipeline := []bson.M{
		// Stage 1: Get the most recent completed tasks - looking back as far as
		// depth versions - on this project.
		{"$match": bson.M{
			model.TaskRevisionOrderNumberKey: bson.M{
				"$lte": current.RevisionOrderNumber,
				"$gte": (current.RevisionOrderNumber - depth),
			},
			model.TaskProjectKey:   current.Project,
			model.TaskRequesterKey: evergreen.RepotrackerVersionRequester,
			model.TaskStatusKey: bson.M{
				"$in": []string{
					evergreen.TaskFailed,
					evergreen.TaskSucceeded,
				},
			},
		}},
		// Stage 2: Sort the tasks by the most recently completed.
		{"$sort": bson.M{
			model.TaskRevisionOrderNumberKey: -1,
		}},
		// Stage 3: Project only relevant fields.
		{"$project": bson.M{
			model.TaskDisplayNameKey:  1,
			model.TaskBuildVariantKey: 1,
			model.TaskTestResultsKey:  1,
			model.TaskIdKey:           1,
		}},
		// Stage 4: Group these tasks by display name and buildvariant -
		// this returns the most recently completed grouped by task display name
		// and by variant. We take only the first test results (adding its task
		// id) for each task/variant group.
		{"$group": bson.M{
			"_id": bson.M{
				"t": "$" + model.TaskDisplayNameKey,
				"v": "$" + model.TaskBuildVariantKey,
			},
			"l": bson.M{
				"$first": "$" + model.TaskTestResultsKey,
			},
			"tid": bson.M{
				"$first": "$" + model.TaskIdKey,
			},
		}},
		// Stage 5: For each group, filter out those task/variant combinations
		// that don't have at least one test failure in them.
		{"$match": bson.M{
			"l." + model.TestResultStatusKey: evergreen.TestFailedStatus,
		}},
		// Stage 6: Rewrite each task/variant combination from the _id into the
		// top-level. Project only the test name and status for all tests, and
		// add a 'status' literal string to each group. This sets up the
		// documents for redacting in next stage.
		{"$project": bson.M{
			"tid": 1,
			"t":   "$_id.t",
			"v":   "$_id.v",
			"l." + model.TestResultStatusKey:   1,
			"l." + model.TestResultTestFileKey: 1,
			"status": bson.M{
				"$literal": evergreen.TestFailedStatus,
			},
		}},
		// Stage 7: While each test result contains at least one failed test,
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
		// Stage 8: We no longer need the status fields so project only fields
		// we want to return.
		{"$project": bson.M{
			"f":   "$l." + model.TestResultTestFileKey,
			"_id": 0,
			"tid": 1,
			"t":   1,
			"v":   1,
		}},
		// Stage 9: Flatten each failing test so we can group them by all the
		// variants on which they are failing.
		{"$unwind": "$f"},
		// Stage 10: Group individual test failure. For each, add the variants
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
	return failures, db.Aggregate(model.TasksCollection, pipeline, &failures)
}
