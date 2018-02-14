package migrations

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser"
	anserdb "github.com/mongodb/anser/db"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type githubHookMigrationSuite struct {
	env      *mock.Environment
	database string
	session  anserdb.Session
	cancel   func()

	projectRefs []model.ProjectRef
	projectVars []bson.M

	suite.Suite
}

func TestGithubHookMigration(t *testing.T) {
	require := require.New(t)

	mgoSession, database, err := db.GetGlobalSessionFactory().GetSession()
	require.NoError(err)
	defer mgoSession.Close()

	session := anserdb.WrapSession(mgoSession.Copy())
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())
	s := &githubHookMigrationSuite{
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

func (s *githubHookMigrationSuite) SetupTest() {
	s.NoError(db.ClearCollections(model.ProjectVarsCollection, model.ProjectRefCollection, model.GithubHooksCollection))
	s.projectRefs = []model.ProjectRef{
		{
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Identifier: "mci",
		},
		{
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Identifier: "mci2",
		},
		{
			Owner:      "mongodb",
			Repo:       "grip",
			Identifier: "grip",
		},
		{
			Owner:      "mongodb",
			Repo:       "grip",
			Identifier: "grip2",
		},
		{
			Owner:      "mongodb",
			Repo:       "grip",
			Identifier: "grip3",
		},
		{
			Owner:      "mongodb",
			Repo:       "grip",
			Identifier: "grip4",
		},
	}
	for _, ref := range s.projectRefs {
		s.NoError(ref.Insert())
	}

	s.projectVars = []bson.M{
		{
			"_id":            "mci",
			"github_hook_id": 9001,
		},
		{
			"_id": "mci2",
			"vars": bson.M{
				"hi": "bye",
			},
		},
		// no vars for grip
		{
			"_id":            "grip2",
			"github_hook_id": 9002,
		},
		{
			"_id":            "grip3",
			"github_hook_id": 9003,
		},
		// a deliberately duplicate hook id #
		{
			"_id":            "grip4",
			"github_hook_id": 9002,
		},
	}
	for _, vars := range s.projectVars {
		s.NoError(db.Insert(model.ProjectVarsCollection, vars))
	}
}

func (s *githubHookMigrationSuite) TestMigration() {
	gen, err := githubHooksToCollectionGenerator(anser.GetEnvironment(), s.database, 50)
	s.NoError(err)
	gen.Run()
	s.NoError(gen.Error())

	for j := range gen.Jobs() {
		j.Run()
		s.NoError(j.Error())
	}

	out := []bson.M{}
	s.NoError(db.FindAllQ(model.ProjectVarsCollection, db.Q{}, &out))

	saw := 0
	for _, vars := range out {
		if vars["_id"] == "mci" {
			s.Empty(vars["vars"])

		} else if vars["_id"] == "mci2" {
			s.Len(vars["vars"], 1)

		} else if vars["_id"] == "grip2" {
			s.Empty(vars["vars"])

		} else if vars["_id"] == "grip3" {
			s.Empty(vars["vars"])

		} else if vars["_id"] == "grip4" {
			s.Empty(vars["vars"])

		} else {
			s.T().Errorf("Unknown ID: %s", vars["_id"])
			continue
		}
		_, ok := vars["github_hook_id"]
		s.False(ok)
		s.Empty(vars["private_vars"])
		saw += 1
	}
	s.Equal(5, saw)

	hook, err := model.FindGithubHook("evergreen-ci", "evergreen")
	s.NoError(err)
	s.Require().NotNil(hook)
	s.Equal(9001, hook.HookID)

	hook, err = model.FindGithubHook("mongodb", "grip")
	s.NoError(err)
	s.Require().NotNil(hook)
	s.Equal(9002, hook.HookID)
}

func (s *githubHookMigrationSuite) TearDownSuite() {
	s.cancel()
}
