package model

import (
	"fmt"
	"testing"

	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"

	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
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
	assert.NoError(err) // should upsert
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

func TestGetParamNameForVar(t *testing.T) {
	t.Run("ReturnsNewlyGeneratedParameterNameBasedOnProjectVarName", func(t *testing.T) {
		paramName, err := getParamNameForVar([]ParameterMapping{
			{
				Name:          "my_var",
				ParameterName: "my_parameter_name",
			},
		}, "my_new_var")
		require.NoError(t, err)
		assert.Equal(t, "my_new_var", paramName)
	})
	t.Run("ReturnsAlreadyGeneratedParamName", func(t *testing.T) {
		paramName, err := getParamNameForVar([]ParameterMapping{
			{
				Name:          "my_var",
				ParameterName: "my_parameter_name",
			},
		}, "my_var")
		require.NoError(t, err)
		assert.Equal(t, "my_parameter_name", paramName)
	})
	t.Run("ReturnsNewlyGeneratedParameterNameThatDoesNotStartWithAWS", func(t *testing.T) {
		paramName, err := getParamNameForVar([]ParameterMapping{
			{
				Name:          "my_var",
				ParameterName: "my_parameter_name",
			},
			{
				Name: "aws_key",
			},
		}, "aws_key")
		require.NoError(t, err)
		assert.Equal(t, "_aws_key", paramName)
	})
	t.Run("ReturnsNewlyGeneratedParameterNameThatDoesNotStartWithSSM", func(t *testing.T) {
		paramName, err := getParamNameForVar([]ParameterMapping{
			{
				Name:          "my_var",
				ParameterName: "my_parameter_name",
			},
			{
				Name: "ssm_key",
			},
		}, "ssm_key")
		require.NoError(t, err)
		assert.Equal(t, "_ssm_key", paramName)
	})
	t.Run("ReturnsNewlyGeneratedParameterNameIfParameterMappingDoesNotYetHaveParameterName", func(t *testing.T) {
		paramName, err := getParamNameForVar([]ParameterMapping{
			{
				Name: "aws_key",
			},
		}, "aws_key")
		require.NoError(t, err)
		assert.Equal(t, "_aws_key", paramName)
	})
	t.Run("ErrorsIfConflictingParameterNameWouldBeGenerated", func(t *testing.T) {
		paramName, err := getParamNameForVar([]ParameterMapping{
			{
				Name:          "_aws_key",
				ParameterName: "_aws_key",
			},
			{
				Name: "aws_key",
			},
		}, "aws_key")
		assert.Error(t, err)
		assert.Zero(t, paramName)
	})
	t.Run("DoesNotAllowEmptyParameterName", func(t *testing.T) {
		paramName, err := getParamNameForVar(nil, "")
		assert.Error(t, err)
		assert.Zero(t, paramName)
	})
}

func TestGetParamValueForVar(t *testing.T) {
	t.Run("ReturnsUnmodifiedVarForShortVarValue", func(t *testing.T) {
		const (
			varName  = "var_name"
			varValue = "var_value"
		)
		paramName, paramValue, err := getCompressedParamValueForVar(varName, varValue)
		assert.NoError(t, err)
		assert.Equal(t, varName, paramName)
		assert.Equal(t, varValue, paramValue)
	})
	t.Run("ReturnsUnmodifiedVarForEmptyVarValue", func(t *testing.T) {
		const (
			varName  = "var_name"
			varValue = ""
		)
		paramName, paramValue, err := getCompressedParamValueForVar(varName, varValue)
		assert.NoError(t, err)
		assert.Equal(t, varName, paramName)
		assert.Equal(t, varValue, paramValue)
	})
	t.Run("ReturnsCompressedVarForLongVarValue", func(t *testing.T) {
		const varName = "var_name"
		varValue := strings.Repeat("abc", parameterstore.ParamValueMaxLength)
		assert.Greater(t, len(varValue), parameterstore.ParamValueMaxLength)

		paramName, paramValue, err := getCompressedParamValueForVar(varName, varValue)
		assert.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s%s", varName, gzipCompressedParamExtension), paramName)
		assert.Less(t, len(paramValue), parameterstore.ParamValueMaxLength)
	})
	t.Run("ReturnsErrorForVarValueThatExceedsMaxLengthAfterCompression", func(t *testing.T) {
		const varName = "var_name"
		varValue := utility.MakeRandomString(10 * parameterstore.ParamValueMaxLength)
		assert.Greater(t, len(varValue), parameterstore.ParamValueMaxLength)

		_, _, err := getCompressedParamValueForVar(varName, varValue)
		assert.Error(t, err)
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
