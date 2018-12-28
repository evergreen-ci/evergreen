package githubpr

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/evergreen-ci/evergreen/model/commitq"
	"github.com/google/go-github/github"
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
	ctx := context.TODO()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	githubClient := github.NewClient(tc)
	s := &githubPRLogger{
		Base:         send.NewBase(name),
		prService:    githubClient.GetPullRequests(),
		statusSender: statusSender,
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := s.SetErrorHandler(send.ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *githubPRLogger) Send(m message.Composer) {
	if !s.Level().ShouldLog(m) {
		return
	}

	msg := m.Raw().(*GithubMergePR)
	mergeOpts := &github.PullRequestOptions{
		MergeMethod: string(msg.MergeMethod),
		CommitTitle: msg.CommitTitle,
	}

	ctx := context.TODO()
	PRResult, _, err := s.prService.Merge(ctx, msg.Owner, msg.Repo, msg.PRNum, msg.CommitMsg, mergeOpts)
	if err != nil {
		s.ErrorHandler(err, m)
	}

	s.sendMergeResult(PRResult, msg)

	// Pop the queue
	cq, err := commitq.FindOneId(msg.ProjectID)
	cq.Remove(strconv.Itoa(msg.PRNum))
}

func (s *githubPRLogger) sendMergeResult(PRResult *github.PullRequestMergeResult, msg *GithubMergePR) error {
	var state message.GithubState
	var description string
	if PRResult.GetMerged() {
		state = message.GithubStateSuccess
		description = "Commit queue merge succeeded"
	} else {
		state = message.GithubStateFailure
		description = "Commit queue merge failed: "
		description += PRResult.GetMessage()
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
