package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindOneProjectVar(t *testing.T) {
	assert := assert.New(t)

	require.NoError(t, db.Clear(ProjectVarsCollection),
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
	assert.NotNil(change.UpsertedId)
	assert.Equal(1, change.Updated, "%+v", change)

	projectVarsFromDB, err := FindOneProjectVars("mongodb")
	assert.NoError(err)

	assert.Equal("mongodb", projectVarsFromDB.Id)
	assert.Equal(vars, projectVarsFromDB.Vars)
}

func TestProjectVarsInsert(t *testing.T) {
	assert := assert.New(t)

	require.NoError(t, db.Clear(ProjectVarsCollection),
		"Error clearing collection")

	vars := &ProjectVars{
		Id:   "mongodb",
		Vars: map[string]string{"a": "1", "b": "2"},
	}
	assert.NoError(vars.Insert())

	projectVarsFromDB, err := FindOneProjectVars("mongodb")
	assert.NoError(err)
	assert.Equal("mongodb", projectVarsFromDB.Id)
	assert.NotEmpty(projectVarsFromDB.Vars)
	assert.Equal("1", projectVarsFromDB.Vars["a"])
}

func TestProjectVarsFindAndModify(t *testing.T) {
	assert := assert.New(t)

	require.NoError(t, db.Clear(ProjectVarsCollection),
		"Error clearing collection")

	vars := &ProjectVars{
		Id:          "123",
		Vars:        map[string]string{"a": "1", "b": "3", "d": "4"},
		PrivateVars: map[string]bool{"b": true, "d": true},
	}
	assert.NoError(vars.Insert())

	// want to "fix" b, add c, delete d
	newVars := &ProjectVars{
		Id:          "123",
		Vars:        map[string]string{"b": "2", "c": "3"},
		PrivateVars: map[string]bool{"b": false, "a": true},
	}
	varsToDelete := []string{"d"}

	info, err := newVars.FindAndModify(varsToDelete)
	assert.NoError(err)
	assert.NotNil(info)
	assert.Equal(info.Updated, 1)

	assert.Equal(newVars.Vars["a"], "1")
	assert.Equal(newVars.Vars["b"], "2")
	assert.Equal(newVars.Vars["c"], "3")
	_, ok := newVars.Vars["d"]
	assert.False(ok)

	assert.Equal(newVars.PrivateVars["b"], false)
	assert.Equal(newVars.PrivateVars["a"], true)
	_, ok = newVars.Vars["d"]
	assert.False(ok)

	newVars.Id = "234"
	info, err = newVars.FindAndModify(varsToDelete)
	assert.Error(err)
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

func TestAWSVars(t *testing.T) {
	require := require.New(t)
	require.NoError(db.ClearCollections(ProjectVarsCollection))
	assert := assert.New(t)
	project := "mci"

	// empty vars
	newVars := &ProjectVars{
		Id: project,
	}
	require.NoError(newVars.Insert())
	k, err := GetAWSKeyForProject(project)
	assert.NoError(err)
	assert.Empty(k.Name)
	assert.Empty(k.Value)

	vars := map[string]string{
		"a": "foo",
		"b": "bar",
	}
	privateVars := map[string]bool{
		"a": true,
	}
	projectVars := ProjectVars{
		Id:          project,
		Vars:        vars,
		PrivateVars: privateVars,
	}
	_, err = projectVars.Upsert()
	assert.NoError(err)

	// canaries
	found, err := FindOneProjectVars(project)
	assert.NoError(err)
	assert.Equal("foo", found.Vars["a"])
	assert.Equal("bar", found.Vars["b"])
	assert.Equal(true, found.PrivateVars["a"])
	assert.Equal(false, found.PrivateVars["b"])

	// empty aws values
	k, err = GetAWSKeyForProject(project)
	assert.NoError(err)
	assert.Empty(k.Name)
	assert.Empty(k.Value)

	// insert and retrieve aws values
	k = &AWSSSHKey{
		Name:  "aws_key_name",
		Value: "aws_key_value",
	}
	assert.NoError(SetAWSKeyForProject(project, k))
	k, err = GetAWSKeyForProject(project)
	assert.NoError(err)
	assert.Equal("aws_key_name", k.Name)
	assert.Equal("aws_key_value", k.Value)

	// canaries, again
	found, err = FindOneProjectVars(project)
	assert.NoError(err)
	assert.Equal("foo", found.Vars["a"])
	assert.Equal("bar", found.Vars["b"])
	assert.Equal(true, found.PrivateVars["a"])
	assert.Equal(false, found.PrivateVars["b"])

	// hidden aws values
	assert.Equal(false, found.PrivateVars[ProjectAWSSSHKeyName])
	assert.Equal(true, found.PrivateVars[ProjectAWSSSHKeyValue])
}
