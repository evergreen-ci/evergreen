package model

import (
	"reflect"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoBuildFromService(t *testing.T) {

	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id:                  "project",
		Owner:               "my_owner",
		Repo:                "my_repo",
		GithubChecksEnabled: utility.TruePtr(),
		PRTestingEnabled:    utility.FalsePtr(),
		CommitQueue: model.CommitQueueParams{
			MergeMethod: "Squash", // being partially populated shouldn't prevent enabled from being defaulted
		}},
	}
	apiRef := &APIProjectRef{}
	assert.NoError(t, apiRef.BuildFromService(repoRef.ProjectRef))
	// not defaulted yet
	require.NotNil(t, apiRef)
	assert.NotNil(t, apiRef.TaskSync)
	assert.Nil(t, apiRef.GitTagVersionsEnabled)
	assert.Nil(t, apiRef.TaskSync.ConfigEnabled)

	apiRef.DefaultUnsetBooleans()
	assert.True(t, *apiRef.GithubChecksEnabled)
	assert.False(t, *apiRef.PRTestingEnabled)
	require.NotNil(t, apiRef.GitTagVersionsEnabled) // should default
	assert.False(t, *apiRef.GitTagVersionsEnabled)

	assert.NotNil(t, apiRef.TaskSync)
	require.NotNil(t, apiRef.TaskSync.ConfigEnabled) // should default
	assert.False(t, *apiRef.TaskSync.ConfigEnabled)

	require.NotNil(t, apiRef.CommitQueue.Enabled)
	assert.False(t, *apiRef.CommitQueue.Enabled)
}

func TestRecursivelyDefaultBooleans(t *testing.T) {
	type insideStruct struct {
		InsideBool *bool
	}
	type testStruct struct {
		EmptyBool *bool
		TrueBool  *bool
		Strct     insideStruct
		PtrStruct *insideStruct
	}

	myStruct := testStruct{TrueBool: utility.TruePtr()}
	reflected := reflect.ValueOf(&myStruct).Elem()
	recursivelyDefaultBooleans(reflected)

	require.NotNil(t, myStruct.EmptyBool)
	assert.False(t, *myStruct.EmptyBool)
	assert.True(t, *myStruct.TrueBool)
	require.NotNil(t, myStruct.Strct.InsideBool)
	assert.False(t, *myStruct.EmptyBool)
	assert.Nil(t, myStruct.PtrStruct) // shouldn't be affected

}
