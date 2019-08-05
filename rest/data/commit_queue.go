package data

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
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

func (pc *DBCommitQueueConnector) EnqueueItem(projectID string, item restModel.APICommitQueueItem) (int, error) {
	q, err := commitqueue.FindOneId(projectID)
	if err != nil {
		return 0, errors.Wrapf(err, "can't query for queue id %s", projectID)
	}

	itemInterface, err := item.ToService()
	if err != nil {
		return 0, errors.Wrap(err, "item cannot be converted to DB model")
	}

	itemService := itemInterface.(commitqueue.CommitQueueItem)
	position, err := q.Enqueue(itemService)
	if err != nil {
		return 0, errors.Wrapf(err, "can't enqueue item to queue %s", projectID)
	}

	return position, nil
}

func (pc *DBCommitQueueConnector) FindCommitQueueByID(id string) (*restModel.APICommitQueue, error) {
	cqService, err := commitqueue.FindOneId(id)
	if err != nil {
		return nil, errors.Wrap(err, "can't get commit queue from database")
	}

	apiCommitQueue := &restModel.APICommitQueue{}
	if err = apiCommitQueue.BuildFromService(*cqService); err != nil {
		return nil, errors.Wrap(err, "can't read commit queue into API model")
	}

	return apiCommitQueue, nil
}

func (pc *DBCommitQueueConnector) CommitQueueRemoveItem(id, item string) (bool, error) {
	cq, err := commitqueue.FindOneId(id)
	if err != nil {
		return false, errors.Wrapf(err, "can't get commit queue for id '%s'", id)
	}

	head := cq.Next()
	removed, err := cq.Remove(item)
	if err != nil {
		return removed, errors.Wrapf(err, "can't remove item '%s' from queue '%s'", item, id)
	}

	if removed && head.Issue == item {
		if err = preventMergeForItem(id, head); err != nil {
			return removed, errors.Wrapf(err, "can't prevent merge for item '%s' on queue '%s'", item, id)
		}
	}

	return removed, nil
}

func (pc *DBCommitQueueConnector) CommitQueueClearAll() (int, error) {
	return commitqueue.ClearAllCommitQueues()
}

func preventMergeForItem(projectID string, item *commitqueue.CommitQueueItem) error {
	projectRef, err := model.FindOneProjectRef(projectID)
	if err != nil {
		return errors.Wrapf(err, "can't find projectRef for '%s'", projectID)
	}
	if projectRef == nil {
		return errors.Errorf("can't find project ref for '%s'", projectID)
	}

	if projectRef.CommitQueue.PatchType == commitqueue.PRPatchType && item.Version != "" {
		if err = clearVersionPatchSubscriber(item.Version, event.GithubMergeSubscriberType); err != nil {
			return errors.Wrap(err, "can't clear subscriptions")
		}
	}

	if projectRef.CommitQueue.PatchType == commitqueue.CLIPatchType {
		version, err := model.VersionFindOneId(item.Issue)
		if err != nil {
			return errors.Wrapf(err, "can't find patch '%s'", item.Issue)
		}
		if version != nil {
			if err = clearVersionPatchSubscriber(version.Id, event.CommitQueueDequeueSubscriberType); err != nil {
				return errors.Wrap(err, "can't clear subscriptions")
			}

			// Blacklist the merge task
			mergeTask, err := task.FindMergeTaskForVersion(version.Id)
			if err != nil {
				return errors.Wrapf(err, "can't find merge task for '%s'", version.Id)
			}
			err = mergeTask.SetPriority(-1, evergreen.User)
			if err != nil {
				return errors.Wrap(err, "can't blacklist merge task")
			}
		}
	}

	return nil
}

func clearVersionPatchSubscriber(versionID, subscriberType string) error {
	subscriptions, err := event.FindSubscriptions(event.ResourceTypePatch, []event.Selector{{Type: event.SelectorID, Data: versionID}})
	if err != nil {
		return errors.Wrapf(err, "can't find subscription to patch '%s'", versionID)
	}
	for _, subscription := range subscriptions {
		if subscription.Subscriber.Type == subscriberType {
			err = event.RemoveSubscription(subscription.ID)
			if err != nil {
				return errors.Wrapf(err, "can't remove subscription for '%s', type '%s'", versionID, subscriberType)
			}
		}
	}

	return nil
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
	hasPermission := util.StringSliceContains(mergePermissions, permission)

	return inOrg && hasPermission, nil
}

type MockCommitQueueConnector struct {
	Queue map[string][]restModel.APICommitQueueItem
}

func (pc *MockCommitQueueConnector) GetGitHubPR(ctx context.Context, owner, repo string, PRNum int) (*github.PullRequest, error) {
	userID := 1234
	ref := "master"
	sha := "abcdef1234"
	return &github.PullRequest{
		User: &github.User{
			ID: &userID,
		},
		Base: &github.PullRequestBranch{
			Ref: &ref,
		},
		Head: &github.PullRequestBranch{
			SHA: &sha,
		},
	}, nil
}

func (pc *MockCommitQueueConnector) EnqueueItem(projectID string, item restModel.APICommitQueueItem) (int, error) {
	if pc.Queue == nil {
		pc.Queue = make(map[string][]restModel.APICommitQueueItem)
	}

	pc.Queue[projectID] = append(pc.Queue[projectID], item)

	return len(pc.Queue[projectID]), nil
}

func (pc *MockCommitQueueConnector) FindCommitQueueByID(id string) (*restModel.APICommitQueue, error) {
	if _, ok := pc.Queue[id]; !ok {
		return nil, nil
	}

	return &restModel.APICommitQueue{ProjectID: restModel.ToAPIString(id), Queue: pc.Queue[id]}, nil
}

func (pc *MockCommitQueueConnector) CommitQueueRemoveItem(id, item string) (bool, error) {
	if _, ok := pc.Queue[id]; !ok {
		return false, nil
	}

	for i := range pc.Queue[id] {
		if restModel.FromAPIString(pc.Queue[id][i].Issue) == item {
			pc.Queue[id] = append(pc.Queue[id][:i], pc.Queue[id][i+1:]...)
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
