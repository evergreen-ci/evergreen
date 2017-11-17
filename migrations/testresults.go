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

func makeLegacyTaskMigrationFunction(db, collection string) db.MigrationOperation {
	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		var id string
		for _, raw := range rawD {
			switch raw.Name {
			case "_id":
				if err := raw.Value.Unmarshal(&id); err != nil {
					return errors.Wrap(err, "error unmarshaling task id")
				}
			case "execution":
				// sanity check
				return nil
			}
		}

		return session.DB(db).C(collection).Update(bson.M{"_id": id}, bson.M{"$set": bson.M{"execution": 0}})
	}
}

func makeTaskMigrationFunction(db, collection string) db.MigrationOperation {
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

			if err := session.DB(db).C(collection).Insert(test); err != nil {
				return errors.Wrap(err, "error saving testresult")
			}
		}

		return session.DB(db).C(collection).Update(bson.M{"_id": id, "execution": execution}, bson.M{"$unset": bson.M{"test_results": 0}})
	}
}

// testResultsGeneratorFactory returns generators for the tasks and old_tasks collections.
func testResultsGeneratorFactory(env anser.Environment, db string, limit int) []anser.Generator {
	noExecutionOpts := model.GeneratorOptions{
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

	tasksOpts := model.GeneratorOptions{
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

	oldTasksopts := model.GeneratorOptions{
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

	return []anser.Generator{
		anser.NewManualMigrationGenerator(env, noExecutionOpts, "tasks_testresults_noexecution"),
		anser.NewManualMigrationGenerator(env, tasksOpts, "tasks_testresults"),
		anser.NewManualMigrationGenerator(env, oldTasksopts, "old_tasks_testresults"),
	}
}

func registerTestResultsMigrationOperations(env anser.Environment, db string) error {
	if err := env.RegisterManualMigrationOperation("tasks_testresults_noexecution", makeLegacyTaskMigrationFunction(db, tasksCollection)); err != nil {
		return err
	}
	if err := env.RegisterManualMigrationOperation("tasks_testresults", makeTaskMigrationFunction(db, tasksCollection)); err != nil {
		return err
	}
	if err := env.RegisterManualMigrationOperation("old_tasks_testresults", makeTaskMigrationFunction(db, oldTasksCollection)); err != nil {
		return err
	}
	return nil
}
