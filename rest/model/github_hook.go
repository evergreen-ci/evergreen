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

func (a *APIGithubHook) BuildFromService(h interface{}) error {
	v, ok := h.(model.GithubHook)
	if !ok {
		return errors.Errorf("programmatic error: expected GitHub hook but got type %T", h)
	}
	if v.HookID == 0 {
		return errors.New("hook ID cannot be 0")
	}
	if v.Owner == "" {
		return errors.New("owner cannot be empty")
	}
	if v.Repo == "" {
		return errors.New("repo cannot be empty")
	}

	a.HookID = v.HookID
	a.Owner = utility.ToStringPtr(v.Owner)
	a.Repo = utility.ToStringPtr(v.Repo)
	return nil
}

// ToService returns a service layer build using the data from the APIBuild.
func (*APIGithubHook) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}
