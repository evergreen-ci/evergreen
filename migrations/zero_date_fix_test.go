package migrations

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser"
	anserdb "github.com/mongodb/anser/db"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type fixZeroDateSuite struct {
	env      *mock.Environment
	database string
	session  anserdb.Session
	cancel   func()

	utc          *time.Location
	expectedTime time.Time
	nowTime      time.Time
	patches      []patch.Patch
	versions     []version.Version
	builds       []build.Build
	tasks        []task.Task

	suite.Suite
}

func TestFixZeroDateMigration(t *testing.T) {
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFixZeroDateMigration")

	require := require.New(t)

	mgoSession, database, err := db.GetGlobalSessionFactory().GetSession()
	require.NoError(err)
	defer mgoSession.Close()

	session := anserdb.WrapSession(mgoSession.Copy())
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())
	s := &fixZeroDateSuite{
		env:      &mock.Environment{},
		session:  session,
		database: database.Name,
		cancel:   cancel,
	}

	require.NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings)))
	require.NoError(s.env.LocalQueue().Start(ctx))
	s.env.Settings().Credentials = testConfig.Credentials

	anser.ResetEnvironment()
	require.NoError(anser.GetEnvironment().Setup(s.env.LocalQueue(), s.session))
	anser.GetEnvironment().RegisterCloser(func() error { cancel(); return nil })

	suite.Run(t, s)
}

func (s *fixZeroDateSuite) SetupTest() {
	s.NoError(db.ClearCollections(model.ProjectRefCollection, patch.Collection, version.Collection, build.Collection, task.Collection))

	ref := model.ProjectRef{
		Identifier: "mci",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "master",
	}
	s.NoError(ref.Insert())

	var err error
	s.utc, err = time.LoadLocation("UTC")
	s.NoError(err)
	s.NotNil(s.utc)

	s.expectedTime, err = time.ParseInLocation(time.RFC3339Nano, "2018-02-15T20:35:24-05:00", s.utc)
	s.NoError(err)

	s.nowTime = time.Now().Truncate(0).Round(time.Millisecond).In(s.utc)
	zeroTime := time.Time{}.In(s.utc)

	s.patches = []patch.Patch{
		{
			Id:         bson.NewObjectId(),
			Version:    "no-errors",
			CreateTime: s.nowTime,
		},
		{
			Id:         bson.NewObjectId(),
			Version:    "v1",
			CreateTime: zeroTime,
		},
	}
	for i := range s.patches {
		s.NoError(s.patches[i].Insert())
	}

	s.versions = []version.Version{
		{
			Id:         "no-errors",
			Identifier: "mci",
			CreateTime: s.nowTime,
			Revision:   "d3eac475f28d638e83c30863a2dce30986055d8a",
		},
		{
			Id:         "v1",
			Identifier: "mci",
			CreateTime: zeroTime,
			Revision:   "d3eac475f28d638e83c30863a2dce30986055d8a",
		},
	}
	for i := range s.versions {
		s.NoError(s.versions[i].Insert())
	}

	s.builds = []build.Build{
		{
			Id:         "b-no-errors",
			Version:    "no-errors",
			CreateTime: s.nowTime,
		},
		{
			Id:         "b-v1",
			Version:    "v1",
			CreateTime: zeroTime,
		},
	}
	for i := range s.builds {
		s.NoError(s.builds[i].Insert())
	}

	s.tasks = []task.Task{
		{
			Id:         "t-no-errors",
			Version:    "no-errors",
			CreateTime: s.nowTime,
		},
		{
			Id:         "t-v1",
			Version:    "v1",
			CreateTime: zeroTime,
		},
	}
	for i := range s.tasks {
		s.NoError(s.tasks[i].Insert())
	}
}

func (s *fixZeroDateSuite) TestMigration() {
	token, err := s.env.Settings().GetGithubOauthToken()
	s.NoError(err)
	s.NotEmpty(token)

	gen, err := zeroDateFixGenerator(token)(anser.GetEnvironment(), s.database, 50)
	s.NoError(err)
	gen.Run()
	s.NoError(gen.Error())

	for j := range gen.Jobs() {
		j.Run()
		s.NoError(j.Error())
	}

	patches, err := patch.Find(db.Q{})
	s.NoError(err)
	s.Len(patches, len(s.patches))

	for i := range patches {
		if strings.HasSuffix(patches[i].Version, "no-errors") {
			s.True(s.nowTime.Equal(patches[i].CreateTime))

		} else if strings.HasSuffix(patches[i].Version, "v1") {
			s.True(s.expectedTime.Equal(patches[i].CreateTime))

		} else {
			s.T().Errorf("patch with unknown version id: %s", patches[i].Version)
		}
	}

	versions, err := version.Find(db.Q{})
	s.NoError(err)
	s.Len(versions, len(s.versions))

	for i := range versions {
		if strings.HasSuffix(versions[i].Id, "no-errors") {
			s.True(s.nowTime.Equal(versions[i].CreateTime))

		} else if strings.HasSuffix(versions[i].Id, "v1") {
			s.True(s.expectedTime.Equal(versions[i].CreateTime))

		} else {
			s.T().Errorf("version with unknown version id: %s", versions[i].Id)
		}
	}

	builds, err := build.Find(db.Q{})
	s.NoError(err)
	s.Len(builds, len(s.builds))

	for i := range builds {
		if strings.HasSuffix(builds[i].Id, "no-errors") {
			s.True(s.nowTime.Equal(builds[i].CreateTime))

		} else if strings.HasSuffix(builds[i].Id, "v1") {
			s.True(s.expectedTime.Equal(builds[i].CreateTime))

		} else {
			s.T().Errorf("unknown build id: %s", builds[i].Id)
		}
	}

	tasks, err := task.Find(db.Q{})
	s.NoError(err)
	s.Len(tasks, len(s.tasks))

	for i := range tasks {
		if strings.HasSuffix(tasks[i].Id, "no-errors") {
			s.True(s.nowTime.Equal(tasks[i].CreateTime))

		} else if strings.HasSuffix(tasks[i].Version, "v1") {
			s.True(s.expectedTime.Equal(tasks[i].CreateTime))

		} else {
			s.T().Errorf("unknown task id: %s", tasks[i].Id)
		}
	}
}

func (s *fixZeroDateSuite) TearDownSuite() {
	s.cancel()
}
