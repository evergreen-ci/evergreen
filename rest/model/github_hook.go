package model

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type APIGithubHook struct {
	HookID int     `json:"hook_id"`
	Owner  *string `json:"owner"`
	Repo   *string `json:"repo"`
}

func (a *APIGithubHook) BuildFromService(hook model.GithubHook) error {
	if hook.HookID == 0 {
		return errors.New("hook ID cannot be 0")
	}
	if hook.Owner == "" {
		return errors.New("owner cannot be empty")
	}
	if hook.Repo == "" {
		return errors.New("repo cannot be empty")
	}

	a.HookID = hook.HookID
	a.Owner = utility.ToStringPtr(hook.Owner)
	a.Repo = utility.ToStringPtr(hook.Repo)
	return nil
}
