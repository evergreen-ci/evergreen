package data

import (
	"context"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/pkg/errors"
)

type DBCommitQueueConnector struct{}

func (pc *DBCommitQueueConnector) GithubPREnqueueItem(owner, repo string, PRNum int) error {
	// Retrieve base branch for the PR from the Github API
	conf, err := evergreen.GetConfig()
	if err != nil {
		return errors.Wrap(err, "can't get evergreen configuration")
	}
	ghToken, err := conf.GetGithubOauthToken()
	if err != nil {
		return errors.Wrap(err, "can't get Github OAuth token from configuration")
	}
	ctx := context.Background()
	pr, err := thirdparty.GetGithubPullRequest(ctx, ghToken, owner, repo, PRNum)
	if err != nil {
		return errors.Wrap(err, "call to Github API failed")
	}

	baseBranch := *pr.Base.Label
	err = pc.EnqueueItem(owner, repo, baseBranch, strconv.Itoa(PRNum))
	return errors.Wrap(err, "enqueue failed")
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
	if q == nil {
		return errors.Errorf("commit queue not found for id %s", projectID)
	}

	if err := q.Enqueue(item); err != nil {
		return errors.Wrapf(err, "can't enqueue item to queue %s", projectID)
	}

	return nil
}

func (pc *DBCommitQueueConnector) FindCommitQueueByID(id string) (*restModel.APICommitQueue, error) {
	cqService, err := commitqueue.FindOneId(id)
	if err != nil {
		return nil, errors.Wrap(err, "can't get commit queue from database")
	}
	if cqService == nil {
		return nil, nil
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
	if cq == nil {
		return false, nil
	}

	found, err := cq.Remove(item)
	return found, errors.Wrapf(err, "can't remove item '%s'", item)
}

type MockCommitQueueConnector struct {
	Queue map[string][]restModel.APIString
}

func (pc *MockCommitQueueConnector) GithubPREnqueueItem(owner, repo string, PRNum int) error {
	return pc.EnqueueItem(owner, repo, "master", strconv.Itoa(PRNum))
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
