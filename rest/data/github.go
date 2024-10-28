package data

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/v52/github"
	"github.com/pkg/errors"
)

type DBGithubConnector struct{}

// GetGitHubPR takes the owner, repo, and PR number, and returns the associated GitHub PR.
func (gc *DBGithubConnector) GetGitHubPR(ctx context.Context, owner, repo string, prNum int) (*github.PullRequest, error) {
	ctxWithCancel, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pr, err := thirdparty.GetGithubPullRequest(ctxWithCancel, owner, repo, prNum)
	if err != nil {
		return nil, errors.Wrap(err, "getting GitHub PR from GitHub API")
	}

	return pr, nil
}

// AddCommentToPR adds the given comment to the associated PR.
func (gc *DBGithubConnector) AddCommentToPR(ctx context.Context, owner, repo string, prNum int, comment string) error {
	ctxWithCancel, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err := thirdparty.PostCommentToPullRequest(ctxWithCancel, owner, repo, prNum, comment)
	return errors.Wrap(err, "posting GitHub comment with GitHub API")
}
