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

type perfCopyVariantMigration struct {
	migrationSuite
}

func TestPerfCopyVariantMigration(t *testing.T) {
	require := require.New(t)

	mgoSession, database, err := evgdb.GetGlobalSessionFactory().GetSession()
	require.NoError(err)
	defer mgoSession.Close()

	session := db.WrapSession(mgoSession.Copy())
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())
	s := &perfCopyVariantMigration{
		migrationSuite{
			env:      &mock.Environment{},
			session:  session,
			database: database.Name,
			cancel:   cancel,
		},
	}

	require.NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
	require.NoError(s.env.LocalQueue().Start(ctx))

	anser.ResetEnvironment()
	require.NoError(anser.GetEnvironment().Setup(s.env.LocalQueue(), s.session))
	anser.GetEnvironment().RegisterCloser(func() error { cancel(); return nil })

	suite.Run(t, s)
}

func (s *perfCopyVariantMigration) SetupTest() {
	s.NoError(evgdb.ClearCollections(jsonCollection))
	data := []bson.M{
		{
			"task_id":    "task_1",
			"project_id": "a_project",
			"variant":    "a_variant",
			"tag":        "a_tag",
			"name":       "a_name",
		},
		{
			"task_id":    "task_2",
			"project_id": "a_different_project",
			"variant":    "a_variant",
			"tag":        "a_tag",
			"name":       "a_name",
		},
		{
			"task_id":    "task_3",
			"project_id": "a_project",
			"variant":    "a_variant",
			"tag":        "a_tag",
			"name":       "a_name",
		},
		{
			"task_id":    "task_4",
			"project_id": "a_project",
			"variant":    "a_different_variant",
			"tag":        "a_tag",
			"name":       "a_name",
		},
	}
	for _, d := range data {
		s.NoError(evgdb.Insert(jsonCollection, d))
	}
}

func (s *perfCopyVariantMigration) TestMigration() {
	args := migrationGeneratorFactoryOptions{
		db:    s.database,
		limit: 50,
		id:    perfCopyVariantMigrationName,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out []model.TaskJSON
	err := evgdb.FindAllQ(jsonCollection, evgdb.Query(bson.M{}), &out)
	s.NoError(err)
	s.Len(out, 4)

	copyArgs := map[string]string{
		tagKey:         "a_tag",
		projectIDKey:   "a_project",
		fromVariantKey: "a_variant",
		toVariantKey:   "to_variant",
	}
	factoryFactory := perfCopyVariantFactoryFactory(copyArgs)
	factory, err := factoryFactory(anser.GetEnvironment(), args)
	s.NoError(err)
	factory.Run(ctx)
	s.NoError(factory.Error())
	for j := range factory.Jobs() {
		j.Run(ctx)
		s.NoError(j.Error())
	}
	err = evgdb.FindAllQ(jsonCollection, evgdb.Query(bson.M{}), &out)
	s.NoError(err)
	s.Len(out, 6)
	expected := []model.TaskJSON{
		model.TaskJSON{
			TaskId:    "task_1",
			ProjectId: "a_project",
			Variant:   "a_variant",
			Tag:       "a_tag",
			Name:      "a_name",
		},
		model.TaskJSON{
			TaskId:    "task_2",
			ProjectId: "a_different_project",
			Variant:   "a_variant",
			Tag:       "a_tag",
			Name:      "a_name",
		},
		model.TaskJSON{
			TaskId:    "task_3",
			ProjectId: "a_project",
			Variant:   "a_variant",
			Tag:       "a_tag",
			Name:      "a_name",
		},
		model.TaskJSON{
			TaskId:    "task_4",
			ProjectId: "a_project",
			Variant:   "a_different_variant",
			Tag:       "a_tag",
			Name:      "a_name",
		},
		model.TaskJSON{
			TaskId:    "task_1",
			ProjectId: "a_project",
			Variant:   "to_variant",
			Tag:       "a_tag",
			Name:      "a_name",
		},
		model.TaskJSON{
			TaskId:    "task_3",
			ProjectId: "a_project",
			Variant:   "to_variant",
			Tag:       "a_tag",
			Name:      "a_name",
		},
	}
	found := 0
	for _, o := range out {
		for _, e := range expected {
			if o.TaskId == e.TaskId &&
				o.ProjectId == e.ProjectId &&
				o.Variant == e.Variant &&
				o.Tag == e.Tag &&
				o.Name == e.Name {
				found += 1
			}
		}
	}
	s.Equal(found, 6)
}

func (s *perfCopyVariantMigration) TearDownSuite() {
	s.cancel()
}
