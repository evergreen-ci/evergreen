package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
)

type APIGithubHook struct {
	HookID int       `json:"hook_id"`
	Owner  APIString `json:"owner"`
	Repo   APIString `json:"repo"`
}

func (a *APIGithubHook) BuildFromService(h interface{}) error {
	v, ok := h.(model.GithubHook)
	if !ok {
		return fmt.Errorf("incorrect type when converting github hook")
	}
	if v.HookID == 0 {
		return errors.New("HookID cannot be 0")
	}
	if v.Owner == "" {
		return errors.New("Owner cannot be empty string")
	}
	if v.Repo == "" {
		return errors.New("Repo cannot be empty string")
	}

	a.HookID = v.HookID
	a.Owner = APIString(v.Owner)
	a.Repo = APIString(v.Repo)
	return nil
}

// ToService returns a service layer build using the data from the APIBuild.
func (*APIGithubHook) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}
