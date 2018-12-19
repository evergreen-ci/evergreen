package data

import (
	"context"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitq"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/pkg/errors"
)

type DBCommitQConnector struct{}

func (pc *DBCommitQConnector) GithubPREnqueueItem(owner, repo string, PRNum int) error {
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

func (pc *DBCommitQConnector) EnqueueItem(owner, repo, baseBranch, item string) error {
	proj, err := model.FindOneProjectRefWithCommitQByOwnerRepoAndBranch(owner, repo, baseBranch)
	if err != nil {
		return errors.Wrapf(err, "can't query for matching project with commit queue enabled. owner: %s, repo: %s, branch: %s", owner, repo, baseBranch)
	}
	if proj == nil {
		return errors.Wrapf(err, "no matching project with commit queue enabled. owner: %s, repo: %s, branch: %s", owner, repo, baseBranch)
	}

	projectID := proj.Identifier
	q, err := commitq.FindOneId(projectID)
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

type MockCommitQConnector struct {
	Queue map[string][]string
}

func (pc *MockCommitQConnector) GithubPREnqueueItem(owner, repo string, PRNum int) error {
	return pc.EnqueueItem(owner, repo, "master", strconv.Itoa(PRNum))
}

func (pc *MockCommitQConnector) EnqueueItem(owner, repo, baseBranch, item string) error {
	if pc.Queue == nil {
		pc.Queue = make(map[string][]string)
	}

	queueID := owner + "." + repo + "." + baseBranch
	pc.Queue[queueID] = append(pc.Queue[queueID], item)

	return nil
}
