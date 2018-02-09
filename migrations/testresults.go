package migrations

import (
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

func makeTaskMigrationFunction(database, collection string) db.MigrationOperation {
	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		testresults := []testResult{}
		var id string
		var oldTaskID string
		var execution int
		var hasExecution bool
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
			case "old_task_id":
				if err := raw.Value.Unmarshal(&oldTaskID); err != nil {
					return errors.Wrap(err, "error unmarshaling task id")
				}
			case "execution":
				if err := raw.Value.Unmarshal(&execution); err != nil {
					return errors.Wrap(err, "error unmarshaling task execution")
				}
				hasExecution = true
			}
		}

		// Very old tasks may not have an execution field. We should ignore those tasks, as they also have odd shapes.
		if !hasExecution {
			return nil
		}

		for _, test := range testresults {
			if oldTaskID == "" {
				test.TaskID = id
			} else {
				test.TaskID = oldTaskID
			}
			test.Execution = execution

			if err := session.DB(database).C(testResultsCollection).Insert(test); err != nil {
				return errors.Wrap(err, "error saving testresult")
			}
		}

		return session.DB(database).C(collection).Update(bson.M{"_id": id, "execution": execution}, bson.M{"$unset": bson.M{"test_results": 0}})
	}
}

func addExecutionToTasksGenerator(env anser.Environment, db string, limit int) (anser.Generator, error) { //nolint
	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: tasksCollection,
		},
		Limit: limit,
		Query: bson.M{
			"execution": bson.M{"$exists": false},
		},
		JobID: "migration-testresults-legacy-no-execution",
	}

	return anser.NewSimpleMigrationGenerator(env, opts, bson.M{"$set": bson.M{"execution": 0}}), nil
}

func testResultsGenerator(env anser.Environment, db string, limit int) (anser.Generator, error) { // nolint: deadcode, megacheck
	const migrationName = "tasks_testresults"

	if err := env.RegisterManualMigrationOperation(migrationName, makeTaskMigrationFunction(db, tasksCollection)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: tasksCollection,
		},
		Limit: limit,
		Query: bson.M{
			"test_results.0": bson.M{"$exists": true},
			"execution":      bson.M{"$exists": true},
		},
		JobID:     "migration-testresults-tasks",
		DependsOn: []string{"migration-testresults-legacy-no-execution"},
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

func oldTestResultsGenerator(env anser.Environment, db string, limit int) (anser.Generator, error) { // nolint: deadcode, megacheck
	const migrationName = "old_tasks_testresults"

	if err := env.RegisterManualMigrationOperation(migrationName, makeTaskMigrationFunction(db, oldTasksCollection)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: oldTasksCollection,
		},
		Limit: limit,
		Query: bson.M{
			"test_results.0": bson.M{"$exists": true},
			"execution":      bson.M{"$exists": true},
		},
		JobID:     "migration-testresults-oldtasks",
		DependsOn: []string{"migration-testresults-legacy-no-execution"},
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}
