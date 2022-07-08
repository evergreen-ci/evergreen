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
	assert.NoError(apiHook.BuildFromService(hook))
	assert.Equal(APIGithubHook{
		HookID: 1,
		Owner:  utility.ToStringPtr("evergreen-ci"),
		Repo:   utility.ToStringPtr("evergreen"),
	}, apiHook)
}
