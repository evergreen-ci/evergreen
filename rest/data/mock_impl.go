package data

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v34/github"
	"github.com/pkg/errors"
)

type MockDBConnectorImpl struct {
	Queue           map[string][]restModel.APICommitQueueItem
	UserPermissions map[UserRepoInfo]string // map user to permission level in lieu of the Github API
	CachedPatches   []restModel.APIPatch
	Aliases         []restModel.APIProjectAlias
}

func (pc *MockDBConnectorImpl) GetGitHubPR(ctx context.Context, owner, repo string, PRNum int) (*github.PullRequest, error) {
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
	}, nil
}

func (pc *MockDBConnectorImpl) AddPatchForPr(ctx context.Context, projectRef model.ProjectRef, prNum int, modules []restModel.APIModule, messageOverride string) (string, error) {
	return "", nil
}

func (pc *MockDBConnectorImpl) EnqueueItem(projectID string, item restModel.APICommitQueueItem, enqueueNext bool) (int, error) {
	if pc.Queue == nil {
		pc.Queue = make(map[string][]restModel.APICommitQueueItem)
	}
	if enqueueNext && len(pc.Queue[projectID]) > 0 {
		q := pc.Queue[projectID]
		pc.Queue[projectID] = append([]restModel.APICommitQueueItem{q[0], item}, q[1:]...)
		return 1, nil
	}
	pc.Queue[projectID] = append(pc.Queue[projectID], item)
	return len(pc.Queue[projectID]) - 1, nil
}

func (pc *MockDBConnectorImpl) FindCommitQueueForProject(id string) (*restModel.APICommitQueue, error) {
	if _, ok := pc.Queue[id]; !ok {
		return nil, nil
	}

	return &restModel.APICommitQueue{ProjectID: utility.ToStringPtr(id), Queue: pc.Queue[id]}, nil
}

func (pc *MockDBConnectorImpl) CommitQueueRemoveItem(id, itemId, user string) (*restModel.APICommitQueueItem, error) {
	if _, ok := pc.Queue[id]; !ok {
		return nil, nil
	}

	for i := range pc.Queue[id] {
		if utility.FromStringPtr(pc.Queue[id][i].Issue) == itemId {
			item := pc.Queue[id][i]
			pc.Queue[id] = append(pc.Queue[id][:i], pc.Queue[id][i+1:]...)
			return &item, nil
		}
	}

	return nil, nil
}

func (pc *MockDBConnectorImpl) IsAuthorizedToPatchAndMerge(ctx context.Context, settings *evergreen.Settings, args UserRepoInfo) (bool, error) {
	_, err := settings.GetGithubOauthToken()
	if err != nil {
		return false, errors.Wrap(err, "can't get Github OAuth token from configuration")
	}

	permission, ok := pc.UserPermissions[args]
	if !ok {
		return false, nil
	}
	mergePermissions := []string{"admin", "write"}
	hasPermission := utility.StringSliceContains(mergePermissions, permission)
	return hasPermission, nil
}

func (pc *MockDBConnectorImpl) CreatePatchForMerge(ctx context.Context, existingPatchID, commitMessage string) (*restModel.APIPatch, error) {
	return nil, nil
}

func (ctx *MockDBConnectorImpl) HasMatchingGitTagAliasAndRemotePath(projectId, tag string) (bool, string, error) {
	if len(ctx.Aliases) == 1 && utility.FromStringPtr(ctx.Aliases[0].RemotePath) != "" {
		return true, utility.FromStringPtr(ctx.Aliases[0].RemotePath), nil
	}
	return len(ctx.Aliases) > 0, "", nil
}

func (pc *MockDBConnectorImpl) GetProjectFromFile(ctx context.Context, pRef model.ProjectRef, file string, token string) (model.ProjectInfo, error) {
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
		Token:        token,
		ReadFileFrom: model.ReadFromLocal,
	}
	pp, pConfig, err := model.LoadProjectInto(ctx, []byte(config), opts, pRef.Id, p)
	return model.ProjectInfo{
		Project:             p,
		IntermediateProject: pp,
		Config:              pConfig,
	}, err
}

func (mvc *MockDBConnectorImpl) CreateVersionFromConfig(ctx context.Context, projectInfo *model.ProjectInfo, metadata model.VersionMetadata, active bool) (*model.Version, error) {
	return &model.Version{
		Requester:         evergreen.GitTagRequester,
		TriggeredByGitTag: metadata.GitTag,
	}, nil
}
