package githubpr

import (
	"context"
	"log"
	"os"

	"github.com/google/go-github/github"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"golang.org/x/oauth2"
)

type githubPRLogger struct {
	prService *github.PullRequestsService
	*send.Base
}

// NewGithubPRLogger creates a new Sender implementation that
// merges a pull request
// Specify an OAuth token for GitHub authentication
func NewGithubPRLogger(name string, token string) (send.Sender, error) {
	ctx := context.TODO()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	githubClient := github.NewClient(tc)
	s := &githubPRLogger{
		Base:      send.NewBase(name),
		prService: githubClient.GetPullRequests(),
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := s.SetErrorHandler(send.ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *githubPRLogger) Send(m message.Composer) {
	if s.Level().ShouldLog(m) {
		msg := m.Raw().(*GithubMergePR)
		// For now we won't require the SHA to match when we try to merge
		mergeOpts := &github.PullRequestOptions{
			MergeMethod: string(msg.MergeMethod),
			CommitTitle: msg.CommitTitle,
		}

		ctx := context.TODO()
		PRResult, _, err := s.prService.Merge(ctx, msg.Owner, msg.Repo, msg.PRNum, msg.CommitMsg, mergeOpts)
		if err != nil {
			s.ErrorHandler(err, m)
		}
		if PRResult.GetMerged() {
			sendMergeResult(true, PRResult.GetSHA(), PRResult.GetMessage())
		} else {
			sendMergeResult(false, msg.SHA, PRResult.GetMessage())
		}

		// Pop the queue
		//TODO
	}
}

func sendMergeResult(success bool, sha string, message string) error {
	//TODO
	return nil
}
