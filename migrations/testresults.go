package migrations

import (
	evg "github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	// tasksCollection is the name of the tasks collection in the database.
	tasksCollection = "tasks"

	// OldTasksCollection is the name of the old_tasks collection in the database.
	oldTasksCollection = "old_tasks"

	// testResultsCollection is the name of the testresults collection in the database.
	testResultsCollection = "testresults"
)

// testResult is an element of the embedded test_results array in a task.
type testResult struct {
	Status    string  `bson:"status"`
	TestFile  string  `bson:"test_file"`
	URL       string  `bson:"url,omitempty"`
	URLRaw    string  `bson:"url_raw,omitempty"`
	LogID     string  `bson:"log_id,omitempty"`
	LineNum   int     `bson:"line_num,omitempty"`
	ExitCode  int     `bson:"exit_code"`
	StartTime float64 `bson:"start"`
	EndTime   float64 `bson:"end"`

	// Together, TaskID and Execution identify the task which created this testResult
	TaskID    string `bson:"task_id"`
	Execution int    `bson:"task_execution"`
}

func makeTaskMigrationFunction(collection string) db.MigrationOperation {
	return func(session db.Session, rawD bson.RawD) error {
		testresults := []testResult{}
		var id string
		var execution int
		for _, raw := range rawD {
			switch raw.Name {
			case "test_results":
				if err := raw.Value.Unmarshal(&testresults); err != nil {
					return errors.Wrap(err, "error unmarshaling testresults")
				}
				if len(testresults) == 0 {
					return nil
				}
			case "_id":
				if err := raw.Value.Unmarshal(&id); err != nil {
					return errors.Wrap(err, "error unmarshaling task id")
				}
			case "execution":
				if err := raw.Value.Unmarshal(&execution); err != nil {
					return errors.Wrap(err, "error unmarshaling task execution")
				}
			}
		}

		for _, test := range testresults {
			test.TaskID = id
			test.Execution = execution
			if err := evg.Insert(testResultsCollection, test); err != nil {
				return errors.Wrap(err, "error saving testresult")
			}
		}

		return evg.Update(collection, bson.M{"_id": id, "execution": execution}, bson.M{"$unset": bson.M{"test_results": 0}})
	}
}

// testResultsGeneratorFactory returns generators for the tasks and old_tasks collections.
func testResultsGeneratorFactory(env anser.Environment, db string) []anser.Generator {
	tasksOpts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: tasksCollection,
		},
		Query: bson.M{"testresults.0": bson.M{"$exists": true}},
	}

	oldTasksopts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: oldTasksCollection,
		},
		Query: bson.M{"testresults.0": bson.M{"$exists": true}},
	}

	return []anser.Generator{
		anser.NewManualMigrationGenerator(env, tasksOpts, "tasksTestResultsMigration"),
		anser.NewManualMigrationGenerator(env, oldTasksopts, "oldTasksTestResultsMigration"),
	}
}

func registerTestResultsMigrationOperations(env anser.Environment) error {
	if err := env.RegisterManualMigrationOperation("tasks_testresults", makeTaskMigrationFunction(tasksCollection)); err != nil {
		return err
	}
	if err := env.RegisterManualMigrationOperation("old_tasks_testresults", makeTaskMigrationFunction(oldTasksCollection)); err != nil {
		return err
	}
	return nil
}
