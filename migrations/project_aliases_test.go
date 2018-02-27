package migrations

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	evgdb "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type projectAliasMigration struct {
	env      *mock.Environment
	session  db.Session
	database string
	cancel   func()
	suite.Suite
}

func TestProjectAliasMigration(t *testing.T) {
	require := require.New(t)

	mgoSession, database, err := evgdb.GetGlobalSessionFactory().GetSession()
	require.NoError(err)
	defer mgoSession.Close()

	session := db.WrapSession(mgoSession.Copy())
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())
	s := &projectAliasMigration{
		env:      &mock.Environment{},
		session:  session,
		database: database.Name,
		cancel:   cancel,
	}

	require.NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
	require.NoError(s.env.LocalQueue().Start(ctx))

	anser.ResetEnvironment()
	require.NoError(anser.GetEnvironment().Setup(s.env.LocalQueue(), s.session))
	anser.GetEnvironment().RegisterCloser(func() error { cancel(); return nil })

	suite.Run(t, s)
}

func (s *projectAliasMigration) SetupTest() {
	s.NoError(evgdb.ClearCollections(projectVarsCollection, projectAliasCollection))

	data := bson.M{
		"_id": "test",
		"patch_definitions": []bson.M{
			{
				"alias":   "__github",
				"variant": "meh",
				"task":    "something",
			},
			{
				"alias":   "__github",
				"variant": "meh",
				"tags":    []string{"lint"},
			},
		},
	}
	s.NoError(evgdb.Insert(projectVarsCollection, data))
}

func (s *projectAliasMigration) TestMigration() {
	gen, err := projectAliasesToCollectionGenerator(anser.GetEnvironment(), s.database, 50)
	s.NoError(err)
	gen.Run()
	s.NoError(gen.Error())

	for j := range gen.Jobs() {
		j.Run()
		s.NoError(j.Error())
	}

	v, err := model.FindOneProjectVars("test")
	s.NoError(err)
	s.Require().NotNil(v)

	a, err := model.FindAliasInProject("test", "__github")
	s.NoError(err)
	s.Len(a, 2)

	s.Equal("test", a[0].ProjectID)
	s.Equal("__github", a[0].Alias)
	s.Equal("meh", a[0].Variant)
	s.Equal("something", a[0].Task)
	s.Empty(a[0].Tags)

	s.Equal("test", a[1].ProjectID)
	s.Equal("__github", a[1].Alias)
	s.Equal("meh", a[1].Variant)
	s.Empty(a[1].Task)
	s.Equal("lint", a[1].Tags[0])
}

func (s *projectAliasMigration) TearDownSuite() {
	s.cancel()
}
