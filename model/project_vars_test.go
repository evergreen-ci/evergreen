package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestFindOneProjectVar(t *testing.T) {
	assert := assert.New(t)

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
	assert := assert.New(t)

	testutil.HandleTestingErr(db.Clear(ProjectVarsCollection), t,
		"Error clearing collection")

	vars := &ProjectVars{
		Id:   "mongodb",
		Vars: map[string]string{"a": "1"},
		PatchDefinitions: []PatchDefinition{
			{
				Variant: "test",
				Task:    "test",
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
	assert.Equal("test", projectVarsFromDB.PatchDefinitions[0].Variant)
	assert.Equal("test", projectVarsFromDB.PatchDefinitions[0].Task)
}

func TestRedactPrivateVars(t *testing.T) {
	assert := assert.New(t)

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
