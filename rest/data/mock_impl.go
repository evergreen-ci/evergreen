package data

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
)

type MockGitHubConnectorImpl struct {
	Aliases []restModel.APIProjectAlias
}

func (pc *MockGitHubConnectorImpl) GetGitHubPR(ctx context.Context, owner, repo string, prNum int) (*github.PullRequest, error) {
	return &github.PullRequest{
		User: &github.User{
			ID:    github.Int64(1234),
			Login: github.String("github.user"),
		},
		Base: &github.PullRequestBranch{
			Ref: github.String("main"),
		},
		Head: &github.PullRequestBranch{
			SHA: github.String("abcdef1234"),
		},
		MergeableState: utility.ToStringPtr("clean"),
	}, nil
}

func (pc *MockGitHubConnectorImpl) AddCommentToPR(ctx context.Context, owner, repo string, prNum int, comment string) error {
	return nil
}

func (pc *MockGitHubConnectorImpl) GetProjectFromFile(ctx context.Context, pRef model.ProjectRef, file string) (model.ProjectInfo, error) {
	config := `
buildvariants:
- name: v1
  run_on: d
  tasks:
  - name: t1
tasks:
- name: t1
`
	p := &model.Project{}
	opts := &model.GetProjectOpts{
		Ref:          &pRef,
		RemotePath:   file,
		ReadFileFrom: model.ReadFromLocal,
	}
	pp, err := model.LoadProjectInto(ctx, []byte(config), opts, pRef.Id, p)
	return model.ProjectInfo{
		Project:             p,
		IntermediateProject: pp,
		Config:              nil,
	}, err
}

func (mvc *MockGitHubConnectorImpl) CreateVersionFromConfig(ctx context.Context, projectInfo *model.ProjectInfo, metadata model.VersionMetadata) (*model.Version, error) {
	return &model.Version{
		Requester:         evergreen.GitTagRequester,
		TriggeredByGitTag: metadata.GitTag,
	}, nil
}
