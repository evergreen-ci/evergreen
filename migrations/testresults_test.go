package migrations

import (
	"fmt"
	"testing"

	evg "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type TestResultsMigrationSuite struct {
	suite.Suite
	dbName        string
	session       db.Session
	task          bson.M
	invariantTask bson.M
	testResults   []bson.M
	migration     db.MigrationOperation
	collection    string
	database      *mgo.Database
	taskID        string
	oldTaskID     string
}

func TestTestResultsMigration(t *testing.T) {
	s := &TestResultsMigrationSuite{
		migration:  makeTaskMigrationFunction(tasksCollection),
		collection: tasksCollection,
		task: bson.M{
			"_id":       "taskid-1",
			"secret":    "secret-1",
			"version":   "version-1",
			"branch":    "project-1",
			"gitspec":   "revision-1",
			"execution": 1,
		},
		taskID: "taskid-1",
	}
	suite.Run(t, s)

	s = &TestResultsMigrationSuite{
		migration:  makeTaskMigrationFunction(oldTasksCollection),
		collection: oldTasksCollection,
		task: bson.M{
			"_id":         "taskid-1_1",
			"secret":      "secret-1",
			"version":     "version-1",
			"branch":      "project-1",
			"gitspec":     "revision-1",
			"execution":   1,
			"old_task_id": "taskid-1",
		},
		taskID:    "taskid-1_1",
		oldTaskID: "taskid-1",
	}
	suite.Run(t, s)
}

func (s *TestResultsMigrationSuite) SetupSuite() {
	evg.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	mgoSession, database, err := evg.GetGlobalSessionFactory().GetSession()
	s.database = database
	s.Require().NoError(err)
	s.dbName = database.Name
	s.session = db.WrapSession(mgoSession)

	s.invariantTask = bson.M{
		"_id":       "taskid-2",
		"secret":    "secret-2",
		"version":   "version-2",
		"branch":    "project-2",
		"gitspec":   "revision-2",
		"execution": 1,
		"test_results": bson.M{
			"status":    "pass",
			"test_file": "file-1",
			"url":       "url-1",
			"url_raw":   "urlraw-1",
			"log_id":    "logid-1",
			"line_num":  1,
			"exit_code": 1,
			"start":     float64(1),
			"end":       float64(1),
		},
	}

	s.testResults = []bson.M{
		bson.M{
			"status":    "pass",
			"test_file": "file-1",
			"url":       "url-1",
			"url_raw":   "urlraw-1",
			"log_id":    "logid-1",
			"line_num":  1,
			"exit_code": 1,
			"start":     float64(1),
			"end":       float64(1),
		},
		bson.M{
			"status":    "pass",
			"test_file": "file-2",
			"url":       "url-2",
			"url_raw":   "urlraw-2",
			"log_id":    "logid-2",
			"line_num":  2,
			"exit_code": 2,
			"start":     float64(2),
			"end":       float64(2),
		},
	}
}

func (s *TestResultsMigrationSuite) SetupTest() {
	s.Require().NoError(evg.Clear(s.collection))
	s.Require().NoError(evg.Clear(testResultsCollection))
	s.Require().NoError(evg.Insert(s.collection, s.invariantTask))
}

func (s *TestResultsMigrationSuite) TestNoTestResults() {
	s.Require().NoError(evg.Insert(s.collection, s.task))

	var doc bson.RawD
	coll := s.session.DB(s.dbName).C(s.collection)
	s.Require().NoError(coll.FindId(s.taskID).One(&doc))
	s.Assert().NoError(s.migration(s.session, doc))

	count, err := evg.Count(s.collection, bson.M{})
	s.NoError(err)
	s.Equal(2, count)

	var task bson.M
	s.NoError(s.database.C(s.collection).Find(bson.M{"_id": s.taskID}).One(&task))
	s.NotContains(task, "test_results")

	count, err = evg.Count(testResultsCollection, bson.M{})
	s.NoError(err)
	s.Equal(0, count)
}

func (s *TestResultsMigrationSuite) TestWithTestResults() {
	s.task["test_results"] = s.testResults
	s.Require().NoError(evg.Insert(s.collection, s.task))

	// the task has test_results
	var task bson.M
	s.NoError(s.database.C(s.collection).Find(bson.M{"_id": s.taskID}).One(&task))
	s.Contains(task, "test_results")

	// run the migration
	var doc bson.RawD
	coll := s.session.DB(s.dbName).C(s.collection)
	s.Require().NoError(coll.FindId(s.taskID).One(&doc))
	s.Assert().NoError(s.migration(s.session, doc))

	// there are still 2 tasks
	count, err := evg.Count(s.collection, bson.M{})
	s.NoError(err)
	s.Equal(2, count)

	// the task no longer contains test results
	s.NoError(s.database.C(s.collection).Find(bson.M{"_id": s.taskID}).One(&task))
	s.NotContains(task, "test_results")

	// the test results collection has the correct items
	count, err = evg.Count(s.collection, bson.M{})
	s.NoError(err)
	s.Equal(2, count)
	var testresults []bson.M
	s.NoError(s.database.C(testResultsCollection).Find(bson.M{}).All(&testresults))
	for i, test := range testresults {
		s.Equal("pass", test["status"])
		s.Equal(fmt.Sprintf("file-%d", i+1), test["test_file"])
		s.Equal(fmt.Sprintf("url-%d", i+1), test["url"])
		s.Equal(fmt.Sprintf("urlraw-%d", i+1), test["url_raw"])
		s.Equal(fmt.Sprintf("logid-%d", i+1), test["log_id"])
		s.Equal(i+1, test["line_num"])
		s.Equal(i+1, test["exit_code"])
		s.Equal(float64(i+1), test["start"])
		s.Equal(float64(i+1), test["end"])
		s.Equal(1, test["task_execution"])

		// for a task in the tasks collection, testresult.task_id should equal task._id, but
		// for a task in the old_tasks collection, testresult.task_id should equal old_task.old_task_id
		if s.oldTaskID == "" {
			s.Equal(s.taskID, test["task_id"])
		} else {
			s.Equal(s.oldTaskID, test["task_id"])
		}
	}
}

func TestTestResultsLegacyTask(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	evg.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	mgoSession, database, err := evg.GetGlobalSessionFactory().GetSession()
	require.NoError(err)
	dbName := database.Name
	session := db.WrapSession(mgoSession)
	require.NoError(evg.Clear(tasksCollection))

	legacyTask := bson.M{
		"_id":     "taskid-1",
		"secret":  "secret-1",
		"version": "version-1",
		"branch":  "project-1",
		"gitspec": "revision-1",
		"test_results": bson.M{
			"status":    "pass",
			"test_file": "file-1",
			"url":       "url-1",
			"url_raw":   "urlraw-1",
			"log_id":    "logid-1",
			"line_num":  1,
			"exit_code": 1,
			"start":     float64(1),
			"end":       float64(1),
		},
	}
	require.NoError(evg.Insert(tasksCollection, legacyTask))

	// the task has test_results and no execution field
	var task bson.M
	assert.NoError(database.C(tasksCollection).Find(bson.M{"_id": "taskid-1"}).One(&task))
	assert.Contains(task, "test_results")
	assert.NotContains(task, "execution")

	// run the migration
	var doc bson.RawD
	coll := session.DB(dbName).C(tasksCollection)
	assert.NoError(coll.FindId("taskid-1").One(&doc))
	assert.NoError(makeLegacyTaskMigrationFunction()(session, doc))

	// the task still contains test results, and now contains an execution field
	assert.NoError(database.C(tasksCollection).Find(bson.M{"_id": "taskid-1"}).One(&task))
	fmt.Printf("\nbson.M: %+v\n", task)
	assert.Contains(task, "test_results")
	assert.Contains(task, "execution")
	assert.Equal(0, task["execution"].(int))
}
