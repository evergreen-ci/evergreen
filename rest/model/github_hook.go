package model

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
)

type APIGithubHook struct {
	HookID int     `json:"hook_id"`
	Owner  *string `json:"owner"`
	Repo   *string `json:"repo"`
}

func (a *APIGithubHook) BuildFromService(hook model.GithubHook) error {
	a.HookID = hook.HookID
	a.Owner = utility.ToStringPtr(hook.Owner)
	a.Repo = utility.ToStringPtr(hook.Repo)
	return nil
}
