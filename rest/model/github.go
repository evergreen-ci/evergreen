package model

import (
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

type APIGithubPullRequestEvent github.PullRequestEvent

func (p *APIGithubPullRequestEvent) BuildFromService(interface{}) error {
	return errors.New("(p *APIPullRequestEvent) BuildFromService() not implemented")
}

func (p *APIGithubPullRequestEvent) ToService() (interface{}, error) {
	return nil, errors.New("(p *APIPullRequestEvent) ToService() not implemented")

}

type APIGithubPullRequest github.PullRequest

func (p *APIGithubPullRequest) BuildFromService(interface{}) error {
	return errors.New("(p *APIPullRequest) BuildFromService() not implemented")
}

func (p *APIGithubPullRequest) ToService() (interface{}, error) {
	return nil, errors.New("(p *APIPullRequest) ToService() not implemented")

}
