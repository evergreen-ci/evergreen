package migrations

import (
	"context"
	"testing"
	"time"

	"gopkg.in/mgo.v2/bson"

	"github.com/evergreen-ci/evergreen"
	evgdb "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/stretchr/testify/suite"
)

type taskCreateTimeMigrationSuite struct {
	migrationSuite
}

func TestTaskCreateTimeMigration(t *testing.T) {
	suite.Run(t, &taskCreateTimeMigrationSuite{})
}

// Test values used in SetupTest and TestMigration.
var patchTime = time.Now()
var commitTime1 = time.Date(1998, 7, 12, 20, 45, 0, 0, time.UTC)
var commitTime2 = time.Date(2018, 7, 15, 16, 45, 0, 0, time.UTC)

const (
	project   = "mongodb-mongo-test"
	revision1 = "aaaaffff"
	revision2 = "abcd1234"
)

func versionDocument(id string, project string, revision string, createTime time.Time) db.Document {
	return db.Document{
		model.VersionIdKey:         id,
		model.VersionCreateTimeKey: createTime,
		model.VersionRevisionKey:   revision,
		model.VersionIdentifierKey: project,
		model.VersionRequesterKey:  evergreen.RepotrackerVersionRequester,
	}
}

func taskDocument(id string, execution int, project string, revision string, requester string, createTime time.Time) db.Document {
	return db.Document{
		task.IdKey:         id,
		task.ExecutionKey:  execution,
		task.ProjectKey:    project,
		task.RevisionKey:   revision,
		task.RequesterKey:  requester,
		task.CreateTimeKey: createTime,
	}
}

func oldTaskDocument(id string, taskId string, execution int, project string, revision string, requester string, createTime time.Time) db.Document {
	return db.Document{
		task.IdKey:         id,
		task.OldTaskIdKey:  taskId,
		task.ExecutionKey:  execution,
		task.ProjectKey:    project,
		task.RevisionKey:   revision,
		task.RequesterKey:  requester,
		task.CreateTimeKey: createTime,
	}
}

func (s *taskCreateTimeMigrationSuite) SetupTest() {
	require := s.Require()
	require.NoError(evgdb.ClearCollections(tasksCollection, model.VersionCollection, oldTasksCollection))

	// Insert 2 base versions.
	doc := versionDocument("version1", project, revision1, commitTime1)
	require.NoError(evgdb.Insert(model.VersionCollection, doc))
	doc = versionDocument("version2", project, revision2, commitTime2)
	require.NoError(evgdb.Insert(model.VersionCollection, doc))
	// Insert 1 patch task for version 1 without old task.
	doc = taskDocument("task1", 0, project, revision1, evergreen.PatchVersionRequester, patchTime)
	require.NoError(evgdb.Insert(tasksCollection, doc))
	// Insert 1 patch task for version 2 with 2 old tasks.
	doc = taskDocument("task2", 2, project, revision2, evergreen.GithubPRRequester, patchTime)
	require.NoError(evgdb.Insert(tasksCollection, doc))
	doc = oldTaskDocument("old_task1", "task2", 0, project, revision2, evergreen.GithubPRRequester, patchTime)
	require.NoError(evgdb.Insert(oldTasksCollection, doc))
	doc = oldTaskDocument("old_task2", "task2", 1, project, revision2, evergreen.GithubPRRequester, patchTime)
	require.NoError(evgdb.Insert(oldTasksCollection, doc))
	// Insert 1 mainline task for version 1 with 2 old tasks.
	doc = taskDocument("task3", 2, project, revision1, evergreen.RepotrackerVersionRequester, commitTime1)
	require.NoError(evgdb.Insert(tasksCollection, doc))
	doc = oldTaskDocument("old_task3", "task3", 0, project, revision1, evergreen.RepotrackerVersionRequester, commitTime1)
	require.NoError(evgdb.Insert(oldTasksCollection, doc))
	doc = oldTaskDocument("old_task4", "task3", 1, project, revision1, evergreen.RepotrackerVersionRequester, commitTime1)
	require.NoError(evgdb.Insert(oldTasksCollection, doc))
}

func (s *taskCreateTimeMigrationSuite) TestMigration() {
	require := s.Require()
	args := migrationGeneratorFactoryOptions{
		db:    s.database,
		limit: 50,
		id:    "migration-task-create-time",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gen, err := taskCreateTimeGenerator(anser.GetEnvironment(), args)
	require.NoError(err)
	gen.Run(ctx)
	require.NoError(gen.Error())

	for j := range gen.Jobs() {
		j.Run(ctx)
		require.NoError(j.Error())
	}

	// Check that all the tasks in the DB have a create_time that match the commit time for their revision.
	tasks, err := task.Find(evgdb.Query(bson.M{}))
	require.NoError(err)
	require.Len(tasks, 3)
	s.checkTasks(tasks)
	oldTasks, err := task.FindOld(evgdb.Query(bson.M{}))
	require.NoError(err)
	require.Len(oldTasks, 4)
	s.checkTasks(oldTasks)
}

func (s *taskCreateTimeMigrationSuite) checkTasks(tasks []task.Task) {
	require := s.Require()
	for _, task := range tasks {
		if task.Revision == revision1 {
			require.Equal(commitTime1.UTC(), task.CreateTime.UTC())
		} else if task.Revision == revision2 {
			require.Equal(commitTime2.UTC(), task.CreateTime.UTC())
		} else {
			s.T().FailNow()
		}
	}
}

func (s *taskCreateTimeMigrationSuite) TearDownSuite() {
	s.cancel()
}
