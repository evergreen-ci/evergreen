package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/stretchr/testify/assert"
)

func TestAPIGithubHook(t *testing.T) {
	assert := assert.New(t)

	hook := model.GithubHook{
		HookID: 1,
		Owner:  "evergreen-ci",
		Repo:   "evergreen",
	}

	apiHook := APIGithubHook{}
	err := apiHook.BuildFromService(hook)
	assert.NoError(err)
	assert.Equal(APIGithubHook{
		HookID: 1,
		Owner:  APIString("evergreen-ci"),
		Repo:   APIString("evergreen"),
	}, apiHook)

	apiHook = APIGithubHook{}
	err = apiHook.BuildFromService(&hook)
	assert.Error(err)
	assert.Equal("incorrect type when converting github hook", err.Error())
	assert.Zero(apiHook)

	apiHook = APIGithubHook{}
	hook.HookID = 0
	err = apiHook.BuildFromService(hook)
	assert.Error(err)
	assert.Equal("HookID cannot be 0", err.Error())
	assert.Zero(apiHook)

	apiHook = APIGithubHook{}
	hook.HookID = 1
	hook.Repo = ""
	err = apiHook.BuildFromService(hook)
	assert.Error(err)
	assert.Equal("Repo cannot be empty string", err.Error())
	assert.Zero(apiHook)

	apiHook = APIGithubHook{}
	hook.HookID = 1
	hook.Owner = ""
	err = apiHook.BuildFromService(hook)
	assert.Error(err)
	assert.Equal("Owner cannot be empty string", err.Error())
	assert.Zero(apiHook)
}
