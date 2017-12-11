package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestFindProjectAliases(t *testing.T) {
	assert := assert.New(t)
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindAllAliases")
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	session, _, _ := db.GetGlobalSessionFactory().GetSession()
	testutil.HandleTestingErr(session.DB(testConfig.Database.DB).DropDatabase(), t, "Error dropping database")

	sc := &DBConnector{}

	vars := []model.ProjectVars{
		model.ProjectVars{
			Id: "project_id",
			PatchDefinitions: []model.PatchDefinition{
				{
					Alias:   "foo",
					Variant: "variant",
					Task:    "task",
				},
				{
					Alias:   "bar",
					Variant: "not_this_variant",
					Task:    "not_this_task",
				},
				{
					Alias:   "foo",
					Variant: "another_variant",
					Task:    "another_task",
				},
			},
		},
		model.ProjectVars{
			Id: "other_project_id",
			PatchDefinitions: []model.PatchDefinition{
				{
					Alias:   "baz",
					Variant: "variant",
					Task:    "task",
				},
			},
		},
	}
	for _, v := range vars {
		assert.NoError(v.Insert())
	}
	found, err := sc.FindProjectAliases("project_id")
	assert.Nil(err)
	assert.Len(found, 3)
}
