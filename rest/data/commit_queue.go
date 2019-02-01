package data

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
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

func (pc *DBCommitQueueConnector) EnqueueItem(owner, repo, baseBranch, item string) error {
	proj, err := model.FindOneProjectRefWithCommitQByOwnerRepoAndBranch(owner, repo, baseBranch)
	if err != nil {
		return errors.Wrapf(err, "can't query for matching project with commit queue enabled. owner: %s, repo: %s, branch: %s", owner, repo, baseBranch)
	}
	if proj == nil {
		return errors.Wrapf(err, "no matching project with commit queue enabled. owner: %s, repo: %s, branch: %s", owner, repo, baseBranch)
	}

	projectID := proj.Identifier
	q, err := commitqueue.FindOneId(projectID)
	if err != nil {
		return errors.Wrapf(err, "can't query for queue id %s", projectID)
	}

	if err := q.Enqueue(item); err != nil {
		return errors.Wrapf(err, "can't enqueue item to queue %s", projectID)
	}

	return nil
}

func (pc *DBCommitQueueConnector) FindCommitQueueByID(id string) (*restModel.APICommitQueue, error) {
	cqService, err := commitqueue.FindOneId(id)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, nil
		}
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

	return cq.Remove(item)
}

type MockCommitQueueConnector struct {
	Queue map[string][]restModel.APIString
}

func (pc *MockCommitQueueConnector) GetGitHubPR(ctx context.Context, owner, repo string, PRNum int) (*github.PullRequest, error) {
	userID := 1234
	label := "master"
	return &github.PullRequest{
		User: &github.User{
			ID: &userID,
		},
		Base: &github.PullRequestBranch{
			Label: &label,
		},
	}, nil
}

func (pc *MockCommitQueueConnector) EnqueueItem(owner, repo, baseBranch, item string) error {
	if pc.Queue == nil {
		pc.Queue = make(map[string][]restModel.APIString)
	}

	queueID := owner + "." + repo + "." + baseBranch
	pc.Queue[queueID] = append(pc.Queue[queueID], restModel.ToAPIString(item))

	return nil
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
		if restModel.FromAPIString(pc.Queue[id][i]) == item {
			pc.Queue[id] = append(pc.Queue[id][:i], pc.Queue[id][i+1:]...)
			return true, nil
		}
	}

	return false, nil
}
