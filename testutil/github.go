package testutil

import (
	"time"

	"github.com/google/go-github/v52/github"
)

func NewGithubPR(prNumber int, baseRepoName, baseHash, headRepoName, headHash, user, title string) *github.PullRequest {
	return &github.PullRequest{
		Title:  github.String(title),
		Number: github.Int(prNumber),
		Head: &github.PullRequestBranch{
			Ref: github.String("DEVPROD-123"),
			SHA: github.String(headHash),
			Repo: &github.Repository{
				FullName: github.String(headRepoName),
				PushedAt: &github.Timestamp{Time: time.Now().Truncate(time.Millisecond)},
			},
		},
		Base: &github.PullRequestBranch{
			Ref: github.String("main"),
			SHA: github.String(baseHash),
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
