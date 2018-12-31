package githubpr

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

type githubPRLogger struct {
	prService    *github.PullRequestsService
	statusSender send.Sender
	*send.Base
}

// NewGithubPRLogger creates a new Sender implementation that
// merges a pull request
// Specify an OAuth token for GitHub authentication
func NewGithubPRLogger(name string, token string, statusSender send.Sender) (send.Sender, error) {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	githubClient := github.NewClient(tc)
	s := &githubPRLogger{
		Base:         send.NewBase(name),
		prService:    githubClient.GetPullRequests(),
		statusSender: statusSender,
	}

	if err := s.SetErrorHandler(send.ErrorHandlerFromSender(grip.GetSender())); err != nil {
		return nil, errors.Wrap(err, "can't set github pr logger error handler")
	}

	return s, nil
}

func (s *githubPRLogger) Send(m message.Composer) {
	if !s.Level().ShouldLog(m) {
		return
	}
	catcher := grip.NewBasicCatcher()
	msg, ok := m.Raw().(*GithubMergePR)
	if !ok {
		s.ErrorHandler(errors.New("message of type githubPRLogger does not contain a GithubMergePR"), m)
		return
	}

	mergeOpts := &github.PullRequestOptions{
		MergeMethod: msg.MergeMethod,
		CommitTitle: msg.CommitTitle,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	PRResult, _, err := s.prService.Merge(ctx, msg.Owner, msg.Repo, msg.PRNum, msg.CommitMessage, mergeOpts)
	if err != nil {
		catcher.Add(errors.Wrap(err, "can't access GitHub merge API"))
	}

	catcher.Add(s.sendMergeResult(PRResult, msg))
	s.ErrorHandler(catcher.Resolve(), m)
}

func (s *githubPRLogger) sendMergeResult(PRResult *github.PullRequestMergeResult, msg *GithubMergePR) error {
	var state message.GithubState
	var description string
	if PRResult.GetMerged() {
		state = message.GithubStateSuccess
		description = "Commit queue merge succeeded"
	} else {
		state = message.GithubStateFailure
		description = fmt.Sprintf("Commit queue merge failed: %s", PRResult.GetMessage())
	}

	status := message.GithubStatus{
		Owner:       msg.Owner,
		Repo:        msg.Repo,
		Ref:         msg.Ref,
		Context:     "evergreen",
		State:       state,
		Description: description,
	}
	c := message.NewGithubStatusMessageWithRepo(level.Notice, status)
	if !c.Loggable() {
		return errors.New("Can't send malformed GitHub status")
	}

	s.statusSender.Send(c)
	return nil
}
