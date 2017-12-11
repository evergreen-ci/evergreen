package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindOneProjectVar(t *testing.T) {
	assert := assert.New(t) //nolint

	testutil.HandleTestingErr(db.Clear(ProjectVarsCollection), t,
		"Error clearing collection")
	vars := map[string]string{
		"a": "b",
		"c": "d",
	}
	projectVars := ProjectVars{
		Id:   "mongodb",
		Vars: vars,
	}
	change, err := projectVars.Upsert()
	assert.NotNil(change)
	assert.NoError(err)
	assert.Equal(0, change.Updated)

	projectVarsFromDB, err := FindOneProjectVars("mongodb")
	assert.NoError(err)

	assert.Equal("mongodb", projectVarsFromDB.Id)
	assert.Equal(vars, projectVarsFromDB.Vars)
}

func TestProjectVarsInsert(t *testing.T) {
	assert := assert.New(t) //nolint

	testutil.HandleTestingErr(db.Clear(ProjectVarsCollection), t,
		"Error clearing collection")

	vars := &ProjectVars{
		Id:   "mongodb",
		Vars: map[string]string{"a": "1"},
		PatchDefinitions: []PatchDefinition{
			{
				Alias:   "alias",
				Variant: "variant",
				Task:    "task",
			},
		},
	}
	assert.NoError(vars.Insert())

	projectVarsFromDB, err := FindOneProjectVars("mongodb")
	assert.NoError(err)
	assert.Equal("mongodb", projectVarsFromDB.Id)
	assert.NotEmpty(projectVarsFromDB.Vars)
	assert.Equal("1", projectVarsFromDB.Vars["a"])
	assert.NotEmpty(projectVarsFromDB.PatchDefinitions)
	assert.Equal("alias", projectVarsFromDB.PatchDefinitions[0].Alias)
	assert.Equal("variant", projectVarsFromDB.PatchDefinitions[0].Variant)
	assert.Equal("task", projectVarsFromDB.PatchDefinitions[0].Task)
}

func TestRedactPrivateVars(t *testing.T) {
	assert := assert.New(t) //nolint

	vars := map[string]string{
		"a": "a",
		"b": "b",
	}
	privateVars := map[string]bool{
		"a": true,
	}
	projectVars := ProjectVars{
		Id:          "mongodb",
		Vars:        vars,
		PrivateVars: privateVars,
	}
	projectVars.RedactPrivateVars()
	assert.Equal("", projectVars.Vars["a"], "redacted variables should be empty strings")
}

func TestFindProjectAliases(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	require.NoError(db.Clear(ProjectVarsCollection))

	// with an empty project_vars collection
	aliases, err := FindOneProjectAlias("project_id", "foo")
	assert.NoError(err)
	assert.Nil(aliases)
	aliases, err = FindAllProjectAliases("project_id")
	assert.NoError(err)
	assert.Nil(aliases)

	// with a project without patch definitions
	vars := &ProjectVars{
		Id: "project_id",
	}
	assert.NoError(vars.Insert())
	aliases, err = FindOneProjectAlias("project_id", "foo")
	assert.NoError(err)
	assert.Nil(aliases)
	aliases, err = FindAllProjectAliases("project_id")
	assert.Nil(err)
	assert.Len(aliases, 0)

	// with a project with project definitions
	require.NoError(db.Clear(ProjectVarsCollection))
	vars = &ProjectVars{
		Id: "project_id",
		PatchDefinitions: []PatchDefinition{
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
	}
	assert.NoError(vars.Insert())
	aliases, err = FindOneProjectAlias("project_id", "foo")
	assert.NoError(err)
	assert.Len(aliases, 2)
	aliases, err = FindAllProjectAliases("project_id")
	assert.NoError(err)
	assert.Len(aliases, 3)
}
