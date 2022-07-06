package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
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
		Owner:  utility.ToStringPtr("evergreen-ci"),
		Repo:   utility.ToStringPtr("evergreen"),
	}, apiHook)

	apiHook = APIGithubHook{}
	assert.Error(apiHook.BuildFromService(hook))
	assert.Zero(apiHook)

	apiHook = APIGithubHook{}
	hook.HookID = 0
	hook.Owner = "owner"
	hook.Repo = "repo"
	assert.Error(apiHook.BuildFromService(hook))
	assert.Zero(apiHook)

	apiHook = APIGithubHook{}
	hook.HookID = 1
	hook.Owner = "owner"
	hook.Repo = ""
	assert.Error(apiHook.BuildFromService(hook))
	assert.Zero(apiHook)

	apiHook = APIGithubHook{}
	hook.HookID = 1
	hook.Owner = ""
	hook.Repo = "repo"
	assert.Error(apiHook.BuildFromService(hook))
	assert.Zero(apiHook)
}
