package migrations

import (
	"context"
	"testing"

	evgdb "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/anser"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type perfCopyVariantMigration struct {
	migrationSuite
}

func TestPerfCopyVariantMigration(t *testing.T) {
	s := &perfCopyVariantMigration{}
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
		perfTagKey:         "a_tag",
		perfProjectIDKey:   "a_project",
		perfFromVariantKey: "a_variant",
		perfToVariantKey:   "to_variant",
	}
	factory := perfCopyVariantFactoryFactory(copyArgs)
	generator, err := factory(anser.GetEnvironment(), args)
	s.NoError(err)
	generator.Run(ctx)
	s.NoError(generator.Error())
	for j := range generator.Jobs() {
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
