package data

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

type DBCommitQueueConnector struct{}

func (pc *DBCommitQueueConnector) GetGitHubPR(ctx context.Context, owner, repo string, PRNum int) (*github.PullRequest, error) {
	conf, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "can't get evergreen configuration")
	}
	ghToken, err := conf.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "can't get Github OAuth token from configuration")
	}

	ctxWithCancel, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pr, err := thirdparty.GetGithubPullRequest(ctxWithCancel, ghToken, owner, repo, PRNum)
	if err != nil {
		return nil, errors.Wrap(err, "call to Github API failed")
	}

	return pr, nil
}

func (pc *DBCommitQueueConnector) EnqueueItem(projectID string, item restModel.APICommitQueueItem, enqueueNext bool) (int, error) {
	q, err := commitqueue.FindOneId(projectID)
	if err != nil {
		return 0, errors.Wrapf(err, "can't query for queue id '%s'", projectID)
	}
	if q == nil {
		return 0, errors.Errorf("no commit queue found for '%s'", projectID)
	}

	itemInterface, err := item.ToService()
	if err != nil {
		return 0, errors.Wrap(err, "item cannot be converted to DB model")
	}

	itemService := itemInterface.(commitqueue.CommitQueueItem)
	if enqueueNext {
		var position int
		position, err = q.EnqueueAtFront(itemService)
		if err != nil {
			return 0, errors.Wrapf(err, "can't force enqueue item to queue '%s'", projectID)
		}
		return position, nil
	}

	position, err := q.Enqueue(itemService)
	if err != nil {
		return 0, errors.Wrapf(err, "can't enqueue item to queue '%s'", projectID)
	}

	return position, nil
}

func (pc *DBCommitQueueConnector) FindCommitQueueForProject(name string) (*restModel.APICommitQueue, error) {
	id, err := model.FindIdForProject(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cqService, err := commitqueue.FindOneId(id)
	if err != nil {
		return nil, errors.Wrap(err, "can't get commit queue from database")
	}
	if cqService == nil {
		return nil, errors.Errorf("no commit queue found for '%s'", id)
	}

	apiCommitQueue := &restModel.APICommitQueue{}
	if err = apiCommitQueue.BuildFromService(*cqService); err != nil {
		return nil, errors.Wrap(err, "can't read commit queue into API model")
	}

	return apiCommitQueue, nil
}

func (pc *DBCommitQueueConnector) CommitQueueRemoveItem(id, issue, user string) (*restModel.APICommitQueueItem, error) {
	projectRef, err := model.FindOneProjectRef(id)
	if err != nil {
		return nil, errors.Wrapf(err, "can't find projectRef for '%s'", id)
	}
	if projectRef == nil {
		return nil, errors.Errorf("can't find project ref for '%s'", id)
	}
	cq, err := commitqueue.FindOneId(projectRef.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get commit queue for id '%s'", id)
	}
	if cq == nil {
		return nil, errors.Errorf("no commit queue found for '%s'", id)
	}
	version, err := model.GetVersionForCommitQueueItem(cq, issue)
	if err != nil {
		return nil, errors.Wrapf(err, "error verifying if version exists for issue '%s'", issue)
	}
	removed, err := cq.RemoveItemAndPreventMerge(issue, version != nil, user)
	if err != nil {
		return nil, errors.Wrap(err, "unable to remove item")
	}
	if removed == nil {
		return nil, errors.Errorf("item %s not found in queue", issue)
	}
	apiRemovedItem := restModel.APICommitQueueItem{}
	if err = apiRemovedItem.BuildFromService(*removed); err != nil {
		return nil, err
	}
	return &apiRemovedItem, nil
}

func (pc *DBCommitQueueConnector) IsItemOnCommitQueue(id, item string) (bool, error) {
	cq, err := commitqueue.FindOneId(id)
	if err != nil {
		return false, errors.Wrapf(err, "can't get commit queue for id '%s'", id)
	}
	if cq == nil {
		return false, errors.Errorf("no commit queue found for '%s'", id)
	}

	pos := cq.FindItem(item)
	if pos >= 0 {
		return true, nil
	}
	return false, nil
}

func (pc *DBCommitQueueConnector) CommitQueueClearAll() (int, error) {
	return commitqueue.ClearAllCommitQueues()
}

type UserRepoInfo struct {
	Username string
	Owner    string
	Repo     string
}

func (pc *DBCommitQueueConnector) IsAuthorizedToPatchAndMerge(ctx context.Context, settings *evergreen.Settings, args UserRepoInfo) (bool, error) {
	// In the org
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return false, errors.Wrap(err, "can't get Github OAuth token from configuration")
	}

	requiredOrganization := settings.GithubPRCreatorOrg
	if requiredOrganization == "" {
		return false, errors.New("no GitHub PR creator organization configured")
	}

	ctxWithCancel, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	inOrg, err := thirdparty.GithubUserInOrganization(ctxWithCancel, token, requiredOrganization, args.Username)
	if err != nil {
		return false, errors.Wrap(err, "call to Github API failed")
	}

	// Has repository merge permission
	// See: https://help.github.com/articles/repository-permission-levels-for-an-organization/
	ctxWithCancel, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	permission, err := thirdparty.GitHubUserPermissionLevel(ctxWithCancel, token, args.Owner, args.Repo, args.Username)
	if err != nil {
		return false, errors.Wrap(err, "call to Github API failed")
	}
	mergePermissions := []string{"admin", "write"}
	hasPermission := utility.StringSliceContains(mergePermissions, permission)

	return inOrg && hasPermission, nil
}

func (pc *DBCommitQueueConnector) CreatePatchForMerge(ctx context.Context, existingPatchID string) (*restModel.APIPatch, error) {
	existingPatch, err := patch.FindOneId(existingPatchID)
	if err != nil {
		return nil, errors.Wrap(err, "can't get patch")
	}
	if existingPatch == nil {
		return nil, errors.Errorf("no patch found for id '%s'", existingPatchID)
	}

	newPatch, err := model.MakeMergePatchFromExisting(existingPatch)
	if err != nil {
		return nil, errors.Wrap(err, "can't create new patch")
	}

	apiPatch := &restModel.APIPatch{}
	if err = apiPatch.BuildFromService(*newPatch); err != nil {
		return nil, errors.Wrap(err, "problem building API patch")
	}
	return apiPatch, nil
}

func (pc *DBCommitQueueConnector) GetMessageForPatch(patchID string) (string, error) {
	requestedPatch, err := patch.FindOneId(patchID)
	if err != nil {
		return "", errors.Wrap(err, "error finding patch")
	}
	if requestedPatch == nil {
		return "", errors.New("no patch found")
	}
	project, err := model.FindOneProjectRef(requestedPatch.Project)
	if err != nil {
		return "", errors.Wrap(err, "unable to find project for patch")
	}
	if project == nil {
		return "", errors.New("patch has nonexistent project")
	}

	return project.CommitQueue.Message, nil
}

type MockCommitQueueConnector struct {
	Queue map[string][]restModel.APICommitQueueItem
}

func (pc *MockCommitQueueConnector) GetGitHubPR(ctx context.Context, owner, repo string, PRNum int) (*github.PullRequest, error) {
	return &github.PullRequest{
		User: &github.User{
			ID:    github.Int64(1234),
			Login: github.String("github.user"),
		},
		Base: &github.PullRequestBranch{
			Ref: github.String("master"),
		},
		Head: &github.PullRequestBranch{
			SHA: github.String("abcdef1234"),
		},
	}, nil
}

func (pc *MockCommitQueueConnector) EnqueueItem(projectID string, item restModel.APICommitQueueItem, enqueueNext bool) (int, error) {
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

func (pc *MockCommitQueueConnector) FindCommitQueueForProject(id string) (*restModel.APICommitQueue, error) {
	if _, ok := pc.Queue[id]; !ok {
		return nil, nil
	}

	return &restModel.APICommitQueue{ProjectID: utility.ToStringPtr(id), Queue: pc.Queue[id]}, nil
}

func (pc *MockCommitQueueConnector) CommitQueueRemoveItem(id, itemId, user string) (*restModel.APICommitQueueItem, error) {
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

func (pc *MockCommitQueueConnector) IsItemOnCommitQueue(id, item string) (bool, error) {
	queue, ok := pc.Queue[id]
	if !ok {
		return false, errors.Errorf("can't get commit queue for id '%s'", id)
	}
	for _, queueItem := range queue {
		if utility.FromStringPtr(queueItem.Issue) == item {
			return true, nil
		}
	}
	return false, nil
}

func (pc *MockCommitQueueConnector) CommitQueueClearAll() (int, error) {
	var count int
	for k, v := range pc.Queue {
		if len(v) > 0 {
			count++
		}
		pc.Queue[k] = []restModel.APICommitQueueItem{}
	}

	return count, nil
}

func (pc *MockCommitQueueConnector) IsAuthorizedToPatchAndMerge(context.Context, *evergreen.Settings, UserRepoInfo) (bool, error) {
	return true, nil
}

func (pc *MockCommitQueueConnector) CreatePatchForMerge(ctx context.Context, existingPatchID string) (*restModel.APIPatch, error) {
	return nil, nil
}
func (pc *MockCommitQueueConnector) GetMessageForPatch(patchID string) (string, error) {
	return "", nil
}
