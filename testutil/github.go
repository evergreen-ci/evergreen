package testutil

import (
	"time"

	"github.com/google/go-github/github"
)

func NewGithubPREvent(prNumber int, baseRepoName, headRepoName, headHash, user, title string) *github.PullRequestEvent {
	return &github.PullRequestEvent{
		Action: github.String("opened"),
		Number: github.Int(prNumber),
		Repo: &github.Repository{
			FullName: github.String(baseRepoName),
			PushedAt: &github.Timestamp{time.Now().Truncate(time.Millisecond)},
		},
		Sender: &github.User{
			Login: github.String(user),
			ID:    github.Int(1234),
		},
		PullRequest: &github.PullRequest{
			Title: github.String(title),
			Head: &github.PullRequestBranch{
				SHA: github.String(headHash),
				Repo: &github.Repository{
					FullName: github.String(headRepoName),
				},
			},
			Base: &github.PullRequestBranch{
				Ref: github.String("master"),
			},
		},
	}
}
