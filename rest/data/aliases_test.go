package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindProjectAliases(t *testing.T) {
	assert := assert.New(t)
	session, _, _ := db.GetGlobalSessionFactory().GetSession()
	require.NoError(t, session.DB(testConfig.Database.DB).DropDatabase(), "Error dropping database")

	sc := &DBConnector{}

	aliases := []model.ProjectAlias{
		{
			ProjectID: "project_id",
			Alias:     "foo",
			Variant:   "variant",
			Task:      "task",
		},
		{
			ProjectID: "project_id",
			Alias:     "bar",
			Variant:   "not_this_variant",
			Task:      "not_this_task",
		},
		{
			ProjectID: "project_id",
			Alias:     "foo",
			Variant:   "another_variant",
			Task:      "another_task",
		},
		{
			ProjectID: "other_project_id",
			Alias:     "baz",
			Variant:   "variant",
			Task:      "task",
		},
	}
	for _, v := range aliases {
		assert.NoError(v.Upsert())
	}
	found, err := sc.FindProjectAliases("project_id")
	assert.Nil(err)
	assert.Len(found, 3)
}
