package data

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/testresult"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
)

type MockGitHubConnectorImpl struct {
	Queue           map[string][]restModel.APICommitQueueItem
	UserPermissions map[UserRepoInfo]string // map user to permission level in lieu of the Github API
	CachedPatches   []restModel.APIPatch
	Aliases         []restModel.APIProjectAlias
	CachedTests     []testresult.TestResult
	StoredError     error
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

func (pc *MockGitHubConnectorImpl) AddPatchForPR(ctx context.Context, projectRef model.ProjectRef, prNum int, modules []restModel.APIModule, messageOverride string) (*patch.Patch, error) {
	return &patch.Patch{}, nil
}

func (pc *MockGitHubConnectorImpl) AddCommentToPR(ctx context.Context, owner, repo string, prNum int, comment string) error {
	return nil
}

func (pc *MockGitHubConnectorImpl) IsAuthorizedToPatchAndMerge(ctx context.Context, settings *evergreen.Settings, args UserRepoInfo) (bool, error) {

	permission, ok := pc.UserPermissions[args]
	if !ok {
		return false, nil
	}
	mergePermissions := []string{"admin", "write"}
	hasPermission := utility.StringSliceContains(mergePermissions, permission)
	return hasPermission, nil
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
