package data

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/v34/github"
	"github.com/pkg/errors"
)

type DBGithubConnector struct{}

// GetGitHubPR takes the owner, repo, and PR number, and returns the associated GitHub PR.
func (gc *DBGithubConnector) GetGitHubPR(ctx context.Context, owner, repo string, prNum int) (*github.PullRequest, error) {
	conf, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "getting admin settings")
	}
	ghToken, err := conf.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "getting GitHub OAuth token from admin settings")
	}

	ctxWithCancel, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pr, err := thirdparty.GetGithubPullRequest(ctxWithCancel, ghToken, owner, repo, prNum)
	if err != nil {
		return nil, errors.Wrap(err, "getting GitHub PR from GitHub API")
	}

	return pr, nil
}

// AddCommentToPR adds the given comment to the associated PR.
func (gc *DBGithubConnector) AddCommentToPR(ctx context.Context, owner, repo, comment string, PRNum int) error {
	conf, err := evergreen.GetConfig()
	if err != nil {
		return errors.Wrap(err, "getting admin settings")
	}
	ghToken, err := conf.GetGithubOauthToken()
	if err != nil {
		return errors.Wrap(err, "getting GitHub OAuth token from admin settings")
	}

	ctxWithCancel, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err = thirdparty.PostCommentToPullRequest(ctxWithCancel, ghToken, owner, repo, comment, PRNum)
	return errors.Wrap(err, "posting GitHub comment with GitHub API")
}
