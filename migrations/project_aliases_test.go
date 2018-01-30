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
)

type projectAliasMigration struct {
	env      *mock.Environment
	session  db.Session
	database string
	cancel   func()
	suite.Suite
}

func TestProjectAliasMigration(t *testing.T) {
	require := require.New(t) // nolint

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

	require.NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings)))
	require.NoError(s.env.LocalQueue().Start(ctx))

	anser.ResetEnvironment()
	require.NoError(anser.GetEnvironment().Setup(s.env.LocalQueue(), s.session))
	anser.GetEnvironment().RegisterCloser(func() error { cancel(); return nil })

	suite.Run(t, s)
}

func (s *projectAliasMigration) SetupTest() {
	s.NoError(evgdb.ClearCollections(projectVarsCollection, projectAliasCollection))

	vars := &model.ProjectVars{
		Id: "test",
		PatchDefinitions: []model.PatchDefinition{
			{
				Alias:   "__github",
				Variant: "meh",
				Task:    "something",
			},
			{
				Alias:   "__github",
				Variant: "meh",
				Tags:    []string{"lint"},
			},
		},
	}
	s.NoError(vars.Insert())
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
	s.Empty(v.PatchDefinitions)

	a, err := model.FindAliasInProject("test", "__github")
	s.NoError(err)
	s.Len(a, 2)
}

func (s *projectAliasMigration) TearDownTest() {
	s.cancel()
}
