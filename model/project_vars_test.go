package model

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"testing"

	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"

	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindOneProjectVar(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert := assert.New(t)

	require.NoError(t, db.ClearCollections(ProjectVarsCollection, ProjectRefCollection))
	pRef := ProjectRef{
		Id:                    "mongodb",
		ParameterStoreEnabled: true,
	}
	require.NoError(t, pRef.Insert())
	vars := map[string]string{
		"a": "b",
		"c": "d",
	}
	projectVars := ProjectVars{
		Id:   pRef.Id,
		Vars: vars,
	}
	change, err := projectVars.Upsert()
	assert.NotNil(change)
	assert.NoError(err)
	assert.Equal(1, change.Updated, "%+v", change)

	projectVarsFromDB, err := FindOneProjectVars("mongodb")
	assert.NoError(err)

	assert.Equal("mongodb", projectVarsFromDB.Id)
	assert.Equal(vars, projectVarsFromDB.Vars)

	checkParametersMatchVars(ctx, t, projectVarsFromDB.Parameters, vars)

	dbProjRef, err := FindBranchProjectRef(pRef.Id)
	require.NoError(t, err)
	require.NotZero(t, dbProjRef)
	assert.True(dbProjRef.ParameterStoreVarsSynced)
}

func TestFindMergedProjectVars(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(ProjectVarsCollection, ProjectRefCollection, RepoRefCollection))

	repo := RepoRef{ProjectRef{
		Id:    "repo_ref",
		Owner: "mongodb",
		Repo:  "test_repo",
	}}
	require.NoError(t, repo.Upsert())

	project0 := ProjectRef{
		Id:        "project_0",
		Owner:     "mongodb",
		Branch:    "branch_0",
		Repo:      "test_repo",
		RepoRefId: "repo_ref",
	}
	project1 := ProjectRef{
		Id:        "project_1",
		Owner:     "mongodb",
		Branch:    "branch_1",
		Repo:      "test_repo",
		RepoRefId: "repo_ref",
	}
	require.NoError(t, project0.Insert())
	require.NoError(t, project1.Insert())

	repoVars := ProjectVars{
		Id:            repo.Id,
		Vars:          map[string]string{"hello": "world", "world": "hello", "beep": "boop", "admin": "only"},
		PrivateVars:   map[string]bool{"world": true},
		AdminOnlyVars: map[string]bool{"admin": true},
	}
	project0Vars := ProjectVars{
		Id:   project0.Id,
		Vars: map[string]string{"world": "goodbye", "new": "var"},
	}
	require.NoError(t, repoVars.Insert())
	require.NoError(t, project0Vars.Insert())

	// Testing merging of project vars and repo vars
	expectedMergedVars := ProjectVars{
		Id:            project0.Id,
		Vars:          map[string]string{"hello": "world", "world": "goodbye", "beep": "boop", "new": "var", "admin": "only"},
		PrivateVars:   map[string]bool{},
		AdminOnlyVars: map[string]bool{"admin": true},
	}
	mergedVars, err := FindMergedProjectVars(project0.Id)
	assert.NoError(err)
	assert.Equal(expectedMergedVars, *mergedVars)

	// Testing existing repo vars but no project vars
	expectedMergedVars = repoVars
	expectedMergedVars.Id = project1.Id
	mergedVars, err = FindMergedProjectVars(project1.Id)
	assert.NoError(err)
	assert.Equal(expectedMergedVars, *mergedVars)

	// Testing existing project vars but no repo vars
	require.NoError(t, db.Clear(ProjectVarsCollection))
	require.NoError(t, project0Vars.Insert())
	mergedVars, err = FindMergedProjectVars(project0.Id)
	assert.NoError(err)
	assert.Equal(project0Vars.Vars, mergedVars.Vars)
	assert.Equal(0, len(mergedVars.PrivateVars))

	// Testing ProjectRef.RepoRefId == ""
	project0.RepoRefId = ""
	require.NoError(t, project0.Upsert())
	mergedVars, err = FindMergedProjectVars(project0.Id)
	assert.NoError(err)
	assert.Equal(project0Vars, *mergedVars)

	// Testing no project vars and no repo vars
	require.NoError(t, db.Clear(ProjectVarsCollection))
	mergedVars, err = FindMergedProjectVars(project1.Id)
	assert.NoError(err)
	assert.Nil(mergedVars)

	// Testing non-existent project
	mergedVars, err = FindMergedProjectVars("bad_project")
	assert.Error(err)
	assert.Nil(mergedVars)
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
	defer func() {
		assert.NoError(t, db.ClearCollections(ProjectVarsCollection, ProjectRefCollection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T){
		"ShouldModifyExistingVars": func(ctx context.Context, t *testing.T) {
			pRef := ProjectRef{
				Id:                    "123",
				ParameterStoreEnabled: true,
			}
			require.NoError(t, pRef.Insert())

			vars := &ProjectVars{
				Id:          pRef.Id,
				Vars:        map[string]string{"a": "1", "b": "3", "d": "4"},
				PrivateVars: map[string]bool{"b": true, "d": true},
			}
			assert.NoError(t, vars.Insert())

			dbVars, err := FindOneProjectVars(pRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVars)

			assert.Len(t, dbVars.Vars, 3)
			assert.Equal(t, "1", dbVars.Vars["a"])
			assert.Equal(t, "3", dbVars.Vars["b"])
			assert.Equal(t, "4", dbVars.Vars["d"])

			assert.True(t, dbVars.PrivateVars["b"])
			assert.True(t, dbVars.PrivateVars["d"])

			checkParametersMatchVars(ctx, t, dbVars.Parameters, dbVars.Vars)

			dbProjRef, err := FindBranchProjectRef(pRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			assert.True(t, dbProjRef.ParameterStoreVarsSynced, "project vars should be synced to Parameter Store after Insert")

			// want to "fix" b, add c, delete d
			newVars := &ProjectVars{
				Id:          pRef.Id,
				Vars:        map[string]string{"b": "2", "c": "3"},
				PrivateVars: map[string]bool{"a": true, "b": false},
			}
			varsToDelete := []string{"d"}

			info, err := newVars.FindAndModify(varsToDelete)
			assert.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, info.Updated, 1)

			dbVars, err = FindOneProjectVars(pRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVars)

			assert.Len(t, dbVars.Vars, 3)
			assert.Equal(t, "1", dbVars.Vars["a"])
			assert.Equal(t, "2", dbVars.Vars["b"])
			assert.Equal(t, "3", dbVars.Vars["c"])
			_, ok := dbVars.Vars["d"]
			assert.False(t, ok)

			assert.True(t, dbVars.PrivateVars["a"])
			assert.False(t, dbVars.PrivateVars["b"])

			checkParametersMatchVars(ctx, t, dbVars.Parameters, dbVars.Vars)
		},
		"ShouldUpsertNewVars": func(ctx context.Context, t *testing.T) {
			pRef := ProjectRef{
				Id:                    "234",
				ParameterStoreEnabled: true,
			}
			require.NoError(t, pRef.Insert())

			vars := &ProjectVars{
				Id:          pRef.Id,
				Vars:        map[string]string{"b": "2", "c": "3"},
				PrivateVars: map[string]bool{"b": false, "a": true},
			}
			varsToDelete := []string{"d"}
			vars.Id = pRef.Id
			_, err := vars.FindAndModify(varsToDelete)
			assert.NoError(t, err)

			dbVars, err := FindOneProjectVars(pRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVars)

			assert.Len(t, dbVars.Vars, 2)
			assert.Equal(t, "2", dbVars.Vars["b"])
			assert.Equal(t, "3", dbVars.Vars["c"])
			_, ok := dbVars.Vars["d"]
			assert.False(t, ok)

			assert.True(t, dbVars.PrivateVars["a"])
			assert.False(t, dbVars.PrivateVars["b"])

			checkParametersMatchVars(ctx, t, dbVars.Parameters, dbVars.Vars)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			require.NoError(t, db.ClearCollections(ProjectVarsCollection, ProjectRefCollection))

			tCase(ctx, t)
		})
	}

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
	projectVars := &ProjectVars{
		Id:          "mongodb",
		Vars:        vars,
		PrivateVars: privateVars,
	}
	newVars := projectVars.RedactPrivateVars()
	assert.Equal("", newVars.Vars["a"], "redacted variables should be empty strings")
	assert.NotEqual("", projectVars.Vars["a"], "original vars should not be modified")
}

func TestGetVarsByValue(t *testing.T) {
	assert := assert.New(t)

	require.NoError(t, db.Clear(ProjectVarsCollection),
		"Error clearing collection")

	projectVars1 := &ProjectVars{
		Id:   "mongodb1",
		Vars: map[string]string{"a": "1", "b": "2"},
	}

	projectVars2 := &ProjectVars{
		Id:   "mongodb2",
		Vars: map[string]string{"c": "1", "d": "2"},
	}

	projectVars3 := &ProjectVars{
		Id:   "mongodb3",
		Vars: map[string]string{"e": "2", "f": "3"},
	}

	require.NoError(t, projectVars1.Insert())
	require.NoError(t, projectVars2.Insert())
	require.NoError(t, projectVars3.Insert())

	newVars, err := getVarsByValue("1")
	assert.NoError(err)
	assert.Equal(2, len(newVars))

	newVars, err = getVarsByValue("2")
	assert.NoError(err)
	assert.Equal(3, len(newVars))

	newVars, err = getVarsByValue("3")
	assert.NoError(err)
	assert.Equal(1, len(newVars))

	newVars, err = getVarsByValue("0")
	assert.NoError(err)
	assert.Equal(0, len(newVars))
}

func TestAWSVars(t *testing.T) {
	require := require.New(t)
	require.NoError(db.ClearCollections(ProjectVarsCollection, ProjectRefCollection))
	assert := assert.New(t)
	project := ProjectRef{
		Id: "mci",
	}
	assert.NoError(project.Insert())

	// empty vars
	newVars := &ProjectVars{
		Id: project.Id,
	}
	require.NoError(newVars.Insert())
	k, err := GetAWSKeyForProject(project.Id)
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
		Id:          project.Id,
		Vars:        vars,
		PrivateVars: privateVars,
	}
	_, err = projectVars.Upsert()
	assert.NoError(err)

	// canaries
	found, err := FindOneProjectVars(project.Id)
	assert.NoError(err)
	assert.Equal("foo", found.Vars["a"])
	assert.Equal("bar", found.Vars["b"])
	assert.Equal(true, found.PrivateVars["a"])
	assert.Equal(false, found.PrivateVars["b"])

	// empty aws values
	k, err = GetAWSKeyForProject(project.Id)
	assert.NoError(err)
	assert.Empty(k.Name)
	assert.Empty(k.Value)

	// insert and retrieve aws values
	k = &AWSSSHKey{
		Name:  "aws_key_name",
		Value: "aws_key_value",
	}
	assert.NoError(SetAWSKeyForProject(project.Id, k))
	k, err = GetAWSKeyForProject(project.Id)
	assert.NoError(err)
	assert.Equal("aws_key_name", k.Name)
	assert.Equal("aws_key_value", k.Value)

	// canaries, again
	found, err = FindOneProjectVars(project.Id)
	assert.NoError(err)
	assert.Equal("foo", found.Vars["a"])
	assert.Equal("bar", found.Vars["b"])
	assert.Equal(true, found.PrivateVars["a"])
	assert.Equal(false, found.PrivateVars["b"])

	// hidden aws values
	assert.Equal(false, found.PrivateVars[ProjectAWSSSHKeyName])
	assert.Equal(true, found.PrivateVars[ProjectAWSSSHKeyValue])
}

func TestConvertVarToParam(t *testing.T) {
	t.Run("ReturnsNewParamNameForValidVarNameAndValue", func(t *testing.T) {
		const (
			varName  = "var_name"
			varValue = "var_value"
		)
		paramName, paramValue, err := convertVarToParam("project_id", ParameterMappings{}, varName, varValue)
		require.NoError(t, err)
		assert.Equal(t, "project_id/var_name", paramName, "new parameter name should include project ID prefix")
		assert.Equal(t, varValue, paramValue, "variable value is valid and should be unchanged")
	})
	t.Run("ReturnsNewParamNameAndEmptyValueForValidVarNameAndValue", func(t *testing.T) {
		const (
			varName  = "var_name"
			varValue = ""
		)
		paramName, paramValue, err := convertVarToParam("project_id", ParameterMappings{}, varName, varValue)
		require.NoError(t, err)
		assert.Equal(t, "project_id/var_name", paramName, "new parameter name should include project ID prefix")
		assert.Equal(t, varValue, paramValue, "variable value is empty, which is valid, so parameter value should also be empty")
	})
	t.Run("ReturnsValidParamNameForVarContainingDisallowedAWSPrefix", func(t *testing.T) {
		const (
			varName  = "aws_secret"
			varValue = "super_secret"
		)
		paramName, paramValue, err := convertVarToParam("project_id", ParameterMappings{}, varName, varValue)
		require.NoError(t, err)
		assert.Equal(t, "project_id/_aws_secret", paramName, "new parameter name should prevent invalid 'aws' prefix from appearing in basename")
		assert.Equal(t, varValue, paramValue, "parameter value should match variable value because variable value is valid")
	})
	t.Run("ReturnsValidParamNameForVarContainingDisallowedSSMPrefix", func(t *testing.T) {
		const (
			varName  = "ssm_secret"
			varValue = "super_secret"
		)
		paramName, paramValue, err := convertVarToParam("project_id", ParameterMappings{}, varName, varValue)
		require.NoError(t, err)
		assert.Equal(t, "project_id/_ssm_secret", paramName, "new parameter name should prevent invalid 'ssm' prefix from appearing in basename")
		assert.Equal(t, varValue, paramValue, "parameter value should match variable value because variable value is valid")
	})
	t.Run("ReturnsExistingParameterNameAndNewVarValueForVarWithExistingParameter", func(t *testing.T) {
		const (
			varName           = "var_name"
			varValue          = "var_value"
			existingParamName = "/prefix/project_id/var_name"
		)
		pm := ParameterMappings{
			{
				Name:          varName,
				ParameterName: existingParamName,
			},
		}
		paramName, paramValue, err := convertVarToParam("project_id", pm, varName, varValue)
		require.NoError(t, err)
		assert.Equal(t, existingParamName, paramName, "should return already-existing parameter name")
		assert.Equal(t, varValue, paramValue, "parameter value should match variable value")
	})
	t.Run("ReturnsNewParameterNameAndCompressedParameterValueWhenVarValueExceedsLimit", func(t *testing.T) {
		const varName = "var_name"

		longVarValue := strings.Repeat("abc", parameterstore.ParamValueMaxLength)
		assert.Greater(t, len(longVarValue), parameterstore.ParamValueMaxLength)

		paramName, paramValue, err := convertVarToParam("project_id", ParameterMappings{}, varName, longVarValue)
		require.NoError(t, err)
		assert.Equal(t, "project_id/var_name.gz", paramName, "should include project ID prefix and gzip extension to indicate the value was compressed")
		assert.NotEqual(t, longVarValue, paramValue, "compressed value should not match original variable value")

		gzr, err := gzip.NewReader(strings.NewReader(paramValue))
		require.NoError(t, err)
		decompressed, err := io.ReadAll(gzr)
		require.NoError(t, err)
		assert.Equal(t, longVarValue, string(decompressed), "decompressed value should match original value")
	})
	t.Run("ReturnsNewParameterNameAndCompressedParameterValueWhenLengthOfExistingVarIncreasesBeyondLimit", func(t *testing.T) {
		const (
			varName           = "var_name"
			existingParamName = "/prefix/project_id/var_name"
		)
		pm := ParameterMappings{
			{
				Name:          varName,
				ParameterName: existingParamName,
			},
		}

		longVarValue := strings.Repeat("abc", parameterstore.ParamValueMaxLength)
		assert.Greater(t, len(longVarValue), parameterstore.ParamValueMaxLength)

		paramName, paramValue, err := convertVarToParam("project_id", pm, varName, longVarValue)
		require.NoError(t, err)
		assert.NotEqual(t, existingParamName, paramName)
		assert.Equal(t, existingParamName+gzipCompressedParamExtension, paramName, "project variable that was previously short but now is long enoguh to require compression should have its parameter name changed")
		assert.NotEqual(t, longVarValue, paramValue)

		gzr, err := gzip.NewReader(strings.NewReader(paramValue))
		require.NoError(t, err)
		decompressed, err := io.ReadAll(gzr)
		require.NoError(t, err)
		assert.Equal(t, longVarValue, string(decompressed))
	})
	t.Run("ReturnsErrorForEmptyVariableName", func(t *testing.T) {
		const (
			varName  = ""
			varValue = "var_value"
		)
		_, _, err := convertVarToParam("project_id", ParameterMappings{}, varName, varValue)
		assert.Error(t, err, "should not allow variable with empty name")
	})
	t.Run("ReturnsErrorForVariableNameEndingInGzipExtension", func(t *testing.T) {
		const (
			varName  = "var_name.gz"
			varValue = "var_value"
		)
		_, _, err := convertVarToParam("project_id", ParameterMappings{}, varName, varValue)
		assert.Error(t, err, "should not allow variable with gzip extension")
	})
	t.Run("ReturnsErrorForVarValueThatExceedsMaxLengthAfterCompression", func(t *testing.T) {
		const varName = "var_name"
		// Since this is a purely random string, there's no realistic way to
		// compress it to fit within the max length limit.
		longVarValue := utility.MakeRandomString(10 * parameterstore.ParamValueMaxLength)
		assert.Greater(t, len(longVarValue), parameterstore.ParamValueMaxLength)

		_, _, err := convertVarToParam("project_id", ParameterMappings{}, varName, longVarValue)
		assert.Error(t, err)
	})
	t.Run("ReturnsErrorForNewParameterThatWouldConflictWithExistingParameter", func(t *testing.T) {
		const (
			varName  = "aws_secret"
			varValue = "super_secret"
		)
		pm := ParameterMappings{
			{
				Name:          "_aws_secret",
				ParameterName: "/prefix/project_id/_aws_secret",
			},
		}

		_, _, err := convertVarToParam("project_id", pm, varName, varValue)
		assert.Error(t, err, "should not allow creation of new parameter whose name conflicts with an already-existing parameter")
	})
}

func TestConvertParamToVar(t *testing.T) {
	t.Run("ReturnsOriginalVariableNameAndValue", func(t *testing.T) {
		const (
			varName   = "var_name"
			varValue  = "var_value"
			paramName = "/prefix/project_id/var_name"
		)
		pm := ParameterMappings{
			{
				Name:          varName,
				ParameterName: paramName,
			},
		}
		varNameFromParam, varValueFromParam, err := convertParamToVar(pm, paramName, varValue)
		require.NoError(t, err)
		assert.Equal(t, varName, varNameFromParam, "should return original variable name")
		assert.Equal(t, varValue, varValueFromParam, "should return original variable value")
	})
	t.Run("ReturnsOriginalVariableNameAndValueForVariablePrefixedWithAWS", func(t *testing.T) {
		const (
			varName   = "aws_secret"
			varValue  = "super_secret"
			paramName = "/prefix/project_id/_aws_secret"
		)
		pm := ParameterMappings{
			{
				Name:          varName,
				ParameterName: paramName,
			},
		}
		varNameFromParam, varValueFromParam, err := convertParamToVar(pm, paramName, varValue)
		require.NoError(t, err)
		assert.Equal(t, varName, varNameFromParam, "should return original variable name")
		assert.Equal(t, varValue, varValueFromParam, "should return original variable value")
	})
	t.Run("ReturnsErrorForVariableMissingParameterMapping", func(t *testing.T) {
		pm := ParameterMappings{
			{
				Name:          "var_name",
				ParameterName: "/prefix/project_id/var_name",
			},
		}
		varNameFromParam, varValueFromParam, err := convertParamToVar(pm, "some_other_var_name", "some_other_var_value")
		assert.Error(t, err, "should return error if there's no parameter mapping entry associated with the given parameter")
		assert.Zero(t, varNameFromParam)
		assert.Zero(t, varValueFromParam)
	})
	t.Run("ReturnsErrorForParameterThatMapsToEmptyVariable", func(t *testing.T) {
		const (
			paramName = "/prefix/project_id/var_name"
		)
		pm := ParameterMappings{
			{
				ParameterName: paramName,
			},
		}
		varNameFromParam, varValueFromParam, err := convertParamToVar(pm, paramName, "var_value")
		assert.Error(t, err, "should error if parameter mapping entry exists but maps to empty variable name")
		assert.Zero(t, varNameFromParam)
		assert.Zero(t, varValueFromParam)
	})
	t.Run("DecompressesParameterValueToOriginalVariableValue", func(t *testing.T) {
		const (
			varName   = "var_name"
			paramName = "/prefix/project_id/var_name.gz"
		)

		longVarValue := strings.Repeat("abc", parameterstore.ParamValueMaxLength)
		assert.Greater(t, len(longVarValue), parameterstore.ParamValueMaxLength)

		_, compressedParamValue, err := getCompressedParamForVar(varName, longVarValue)
		require.NoError(t, err)

		pm := ParameterMappings{
			{
				Name:          varName,
				ParameterName: paramName,
			},
		}

		varNameFromParam, varValueFromParam, err := convertParamToVar(pm, paramName, compressedParamValue)
		require.NoError(t, err)
		assert.Equal(t, varName, varNameFromParam)
		assert.Equal(t, longVarValue, varValueFromParam, "should return original decompressed variable value")
	})
	t.Run("RoundTripReturnsOriginalVarNameAndValue", func(t *testing.T) {
		const (
			varName   = "var_name"
			varValue  = "var_value"
			projectID = "project_id"
		)
		pm := ParameterMappings{}
		paramName, paramValue, err := convertVarToParam(projectID, pm, varName, varValue)
		require.NoError(t, err)
		pm = append(pm, ParameterMapping{
			Name:          varName,
			ParameterName: paramName,
		})

		varNameFromParam, varValueFromParam, err := convertParamToVar(pm, paramName, paramValue)
		require.NoError(t, err)
		assert.Equal(t, varName, varNameFromParam, "should return original variable name")
		assert.Equal(t, varValue, varValueFromParam, "should return original variable value")
	})
}

func TestShouldGetAdminOnlyVars(t *testing.T) {
	type testCase struct {
		requester          string
		usrId              string
		shouldGetAdminVars bool
	}

	usrId := "not_admin"
	adminUsrId := "admin"
	testCases := map[string]testCase{
		"repotrackerShouldSucceed": {
			requester:          evergreen.RepotrackerVersionRequester,
			usrId:              usrId,
			shouldGetAdminVars: true,
		},
		"triggerShouldSucceed": {
			requester:          evergreen.TriggerRequester,
			usrId:              usrId,
			shouldGetAdminVars: true,
		},
		"gitTagShouldSucceed": {
			requester:          evergreen.GitTagRequester,
			usrId:              usrId,
			shouldGetAdminVars: true,
		},
		"adHocShouldSucceed": {
			requester:          evergreen.AdHocRequester,
			usrId:              usrId,
			shouldGetAdminVars: true,
		},
		"patchVersionShouldFail": {
			requester:          evergreen.PatchVersionRequester,
			usrId:              usrId,
			shouldGetAdminVars: false,
		},
		"githubPRShouldFail": {
			requester:          evergreen.GithubPRRequester,
			usrId:              usrId,
			shouldGetAdminVars: false,
		},
		"mergeTestShouldFail": {
			requester:          evergreen.MergeTestRequester,
			usrId:              usrId,
			shouldGetAdminVars: false,
		},
		"mergeRequestShouldFail": {
			requester:          evergreen.GithubMergeRequester,
			usrId:              usrId,
			shouldGetAdminVars: false,
		},
		"githubPRWithAdminShouldSucceed": {
			requester:          evergreen.GithubPRRequester,
			usrId:              adminUsrId,
			shouldGetAdminVars: true,
		},
		"patchVersionWithAdminShouldSucceed": {
			requester:          evergreen.PatchVersionRequester,
			usrId:              adminUsrId,
			shouldGetAdminVars: true,
		},
	}

	for name, testCase := range testCases {
		assert.NoError(t, db.ClearCollections(user.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))

		usr := user.DBUser{
			Id: usrId,
		}

		adminUsr := user.DBUser{
			Id: adminUsrId,
		}
		assert.NoError(t, usr.Insert())
		assert.NoError(t, adminUsr.Insert())
		env := evergreen.GetEnvironment()
		roleManager := env.RoleManager()
		projectScope := gimlet.Scope{
			ID:        "projectScopeID",
			Type:      evergreen.ProjectResourceType,
			Resources: []string{"myProject"},
		}
		require.NoError(t, roleManager.AddScope(projectScope))

		role := gimlet.Role{
			ID:          "admin_role",
			Scope:       projectScope.ID,
			Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value},
		}
		require.NoError(t, roleManager.UpdateRole(role))
		require.NoError(t, adminUsr.AddRole(role.ID))
		tsk := &task.Task{
			Id:      "t1",
			Project: "myProject",
		}

		t.Run(name, func(t *testing.T) {
			tsk.Requester = testCase.requester
			tsk.ActivatedBy = testCase.usrId

			assert.Equal(t, testCase.shouldGetAdminVars, shouldGetAdminOnlyVars(tsk))
		})
	}

	// Verify that all requesters are tested on the non-admin.
	for _, requester := range evergreen.AllRequesterTypes {
		tested := false
		for _, testCase := range testCases {
			if testCase.usrId == usrId && requester == testCase.requester {
				tested = true
				break
			}
		}
		assert.True(t, tested, fmt.Sprintf("requester '%s' not tested with non-admin", requester))
	}
}

func TestFullSyncToParameterStore(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, ProjectVarsCollection, fakeparameter.Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T){
		"InitiallySyncsAllParametersWithNewProjectVars": func(ctx context.Context, t *testing.T) {
			projRef := ProjectRef{
				Id:                    "project_id",
				ParameterStoreEnabled: true,
			}
			require.NoError(t, projRef.Insert())
			vars := map[string]string{
				"var1": "value1",
				"var2": "value2",
			}
			projVars := ProjectVars{
				Id:   "project_id",
				Vars: vars,
			}

			require.NoError(t, fullSyncToParameterStore(ctx, &projVars, &projRef, false))

			dbProjVars, err := FindOneProjectVars(projVars.Id)
			require.NoError(t, err)
			require.NotZero(t, dbProjVars)

			checkParametersMatchVars(ctx, t, dbProjVars.Parameters, vars)

			dbProjRef, err := FindBranchProjectRef(projRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			assert.True(t, dbProjRef.ParameterStoreVarsSynced)
		},
		"InitiallySyncsAllParametersWithPreexistingProjectVars": func(ctx context.Context, t *testing.T) {
			projRef := ProjectRef{
				Id:                    "project_id",
				ParameterStoreEnabled: true,
			}
			require.NoError(t, projRef.Insert())
			vars := map[string]string{
				"var1": "value1",
				"var2": "value2",
			}
			projVars := ProjectVars{
				Id:   "project_id",
				Vars: vars,
			}
			require.NoError(t, projVars.Insert())

			require.NoError(t, fullSyncToParameterStore(ctx, &projVars, &projRef, false))

			dbProjVars, err := FindOneProjectVars(projVars.Id)
			require.NoError(t, err)
			require.NotZero(t, dbProjVars)

			checkParametersMatchVars(ctx, t, dbProjVars.Parameters, vars)

			dbProjRef, err := FindBranchProjectRef(projRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			assert.True(t, dbProjRef.ParameterStoreVarsSynced)
		},
		"InitiallySyncsAllParametersForRepoVars": func(ctx context.Context, t *testing.T) {
			repoRef := RepoRef{
				ProjectRef: ProjectRef{
					Id:                    "repo_id",
					ParameterStoreEnabled: true,
				},
			}
			require.NoError(t, repoRef.Upsert())
			vars := map[string]string{
				"var1": "value1",
				"var2": "value2",
			}
			repoVars := ProjectVars{
				Id:   "repo_id",
				Vars: vars,
			}

			require.NoError(t, fullSyncToParameterStore(ctx, &repoVars, &repoRef.ProjectRef, true))

			dbRepoVars, err := FindOneProjectVars(repoVars.Id)
			require.NoError(t, err)
			require.NotZero(t, dbRepoVars)

			checkParametersMatchVars(ctx, t, dbRepoVars.Parameters, vars)

			dbRepoRef, err := FindOneRepoRef(repoRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbRepoRef)
			assert.True(t, dbRepoRef.ParameterStoreVarsSynced)
		},
		"DeletesExistingDesyncedParametersAndResyncs": func(ctx context.Context, t *testing.T) {
			projRef := ProjectRef{
				Id:                    "project_id",
				ParameterStoreEnabled: true,
			}
			require.NoError(t, projRef.Insert())
			vars := map[string]string{
				"var1": "value1",
				"var2": "value2",
				"var3": "value3",
			}
			projVars := ProjectVars{
				Id:   "project_id",
				Vars: vars,
			}

			require.NoError(t, fullSyncToParameterStore(ctx, &projVars, &projRef, false))

			dbProjVars, err := FindOneProjectVars(projVars.Id)
			require.NoError(t, err)
			require.NotZero(t, dbProjVars)

			checkParametersMatchVars(ctx, t, dbProjVars.Parameters, vars)

			dbProjRef, err := FindBranchProjectRef(projRef.Id)
			require.NoError(t, err)
			require.NotZero(t, dbProjRef)
			assert.True(t, dbProjRef.ParameterStoreVarsSynced)

			newProjRef := *dbProjRef
			newProjRef.ParameterStoreVarsSynced = false
			newVars := map[string]string{
				"var1": "value1",
				"var3": "new_value3",
				"var4": "value4",
			}
			newProjVars := ProjectVars{
				Id:   projVars.Id,
				Vars: newVars,
			}

			require.NoError(t, fullSyncToParameterStore(ctx, &newProjVars, &newProjRef, false))

			newDBProjVars, err := FindOneProjectVars(projVars.Id)
			require.NoError(t, err)
			require.NotZero(t, newDBProjVars)

			checkParametersMatchVars(ctx, t, newDBProjVars.Parameters, newVars)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, ProjectVarsCollection, fakeparameter.Collection))
			tCase(ctx, t)
		})
	}
}
