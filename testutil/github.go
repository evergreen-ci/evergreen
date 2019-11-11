package testutil

import (
	"time"

	"github.com/google/go-github/github"
)

func NewGithubPR(prNumber int, baseRepoName, headRepoName, headHash, user, title string) *github.PullRequest {
	return &github.PullRequest{
		Title:  github.String(title),
		Number: github.Int(prNumber),
		Head: &github.PullRequestBranch{
			SHA: github.String(headHash),
			Repo: &github.Repository{
				FullName: github.String(headRepoName),
				PushedAt: &github.Timestamp{Time: time.Now().Truncate(time.Millisecond)},
			},
		},
		Base: &github.PullRequestBranch{
			Ref: github.String("master"),
			Repo: &github.Repository{
				FullName: github.String(baseRepoName),
			},
		},
		User: &github.User{
			Login: github.String(user),
			ID:    github.Int64(1234),
		},
	}
}
