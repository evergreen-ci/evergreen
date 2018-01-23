package migrations

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	evgdb "github.com/evergreen-ci/evergreen/db"
	evgmock "github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	evgdb.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

type testOrphanDeletion struct {
	suite.Suite

	env       *evgmock.Environment
	dbName    string
	migration db.MigrationOperation
	session   db.Session
	cancel    func()
}

func TestCleanupOrphans(t *testing.T) {
	require := require.New(t) // nolint

	mgoSession, database, err := evgdb.GetGlobalSessionFactory().GetSession()
	require.NoError(err)
	defer mgoSession.Close()

	session := db.WrapSession(mgoSession.Copy())
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())

	s := &testOrphanDeletion{
		env:       &evgmock.Environment{},
		dbName:    database.Name,
		migration: orphanedVersionCleanup,
		session:   session,
		cancel:    cancel,
	}

	require.NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings)))
	require.NoError(s.env.LocalQueue().Start(ctx))

	anser.ResetEnvironment()
	require.NoError(anser.GetEnvironment().Setup(s.env.LocalQueue(), s.session))
	anser.GetEnvironment().RegisterCloser(func() error { cancel(); return nil })

	suite.Run(t, s)
}

func (s *testOrphanDeletion) TestOrphanedBuildCleanupGenerator() {
	gen, err := orphanedBuildCleanupGenerator(anser.GetEnvironment(), s.dbName, 50)
	s.Require().NoError(err)
	gen.Run()
	s.NoError(gen.Error())

	for j := range gen.Jobs() {
		j.Run()
		s.NoError(j.Error())
	}

	b, err := build.Find(evgdb.Query(bson.M{}))
	s.NoError(err)
	s.Require().Len(b, 2)
	s.Equal("b1", b[0].Id)
	s.Len(b[0].Tasks, 1)
	s.Equal("t1", b[0].Tasks[0].Id)

	s.Equal("b5", b[1].Id)
	s.Require().Len(b[1].Tasks, 1)
	s.Equal("t3", b[1].Tasks[0].Id)

	t, err := build.Find(evgdb.Query(bson.M{}))
	s.NoError(err)
	s.Require().Len(t, 2)
}

func (s *testOrphanDeletion) TestOrphanedVersionCleanupGenerator() {
	gen, err := orphanedVersionCleanupGenerator(anser.GetEnvironment(), s.dbName, 50)
	s.Require().NoError(err)
	gen.Run()
	s.NoError(gen.Error())

	for j := range gen.Jobs() {
		j.Run()
		s.NoError(j.Error())
	}

	v, err := version.Find(evgdb.Q{})
	s.NoError(err)
	s.Require().Len(v, 2)
	s.Equal("v1", v[0].Id)
	s.Require().Len(v[0].BuildVariants, 1)
	s.Equal("b1", v[0].BuildVariants[0].BuildId)
	s.Require().Len(v[0].BuildIds, 1)
	s.Equal("b1", v[0].BuildIds[0])

	s.Equal("v3", v[1].Id)
	s.Require().Len(v[1].BuildVariants, 1)
	s.Equal("b5", v[1].BuildVariants[0].BuildId)
	s.Require().Len(v[1].BuildIds, 1)
	s.Equal("b5", v[1].BuildIds[0])
}

func (s *testOrphanDeletion) SetupTest() {
	s.NoError(evgdb.ClearCollections(version.Collection, build.Collection, task.Collection))

	versions := []version.Version{
		{
			Id:        "v1",
			Requester: evergreen.RepotrackerVersionRequester,
			Status:    evergreen.VersionCreated,
			BuildIds:  []string{"b1", "o-b2"},
			BuildVariants: []version.BuildStatus{
				{
					BuildVariant: "test1",
					BuildId:      "b1",
				},
				{
					BuildVariant: "test2",
					BuildId:      "o-b2",
				},
			},
		},
		{
			Id:        "v2",
			Requester: evergreen.RepotrackerVersionRequester,
			Status:    evergreen.VersionCreated,
			BuildIds:  []string{"o-b3"},
			BuildVariants: []version.BuildStatus{
				{
					BuildVariant: "test",
					BuildId:      "o-b3",
				},
			},
		},
		{
			Id:        "v3",
			Requester: evergreen.RepotrackerVersionRequester,
			Status:    evergreen.VersionCreated,
			BuildIds:  []string{"b5"},
			BuildVariants: []version.BuildStatus{
				{
					BuildVariant: "test",
					Activated:    true,
					ActivateAt:   time.Now(),
					BuildId:      "b5",
				},
			},
		},
	}

	builds := []build.Build{
		{
			Id:        "b1",
			Requester: evergreen.RepotrackerVersionRequester,
			Status:    evergreen.BuildCreated,
			Version:   "v1",
			Tasks: []build.TaskCache{
				{
					Id: "t1",
				},
				{
					Id: "o-t2",
				},
			},
			Project: "test",
		},
		{
			Id:        "b4",
			Requester: evergreen.RepotrackerVersionRequester,
			Status:    evergreen.BuildCreated,
			Version:   "o-v5",
			Tasks: []build.TaskCache{
				{
					Id: "t4",
				},
			},
			Project: "test",
		},
		{
			Id:        "b5",
			Requester: evergreen.RepotrackerVersionRequester,
			Status:    evergreen.BuildCreated,
			Version:   "v3",
			Tasks: []build.TaskCache{
				{
					Id: "t3",
				},
			},
			Project: "test",
		},
	}

	tasks := []task.Task{
		{
			Id:        "t1",
			Requester: evergreen.RepotrackerVersionRequester,
			BuildId:   "b1",
			Version:   "v1",
		},
		{
			Id:        "t3",
			Requester: evergreen.RepotrackerVersionRequester,
			BuildId:   "b5",
			Version:   "v4",
		},
		{
			Id:        "t4",
			Requester: evergreen.RepotrackerVersionRequester,
			BuildId:   "b4",
			Version:   "o-v5",
		},
	}

	for _, v := range versions {
		s.NoError(v.Insert())
	}
	for _, b := range builds {
		s.NoError(b.Insert())
	}
	for _, t := range tasks {
		s.NoError(t.Insert())
	}
}

func (s *testOrphanDeletion) TearDownSuite() {
	s.cancel()
}
