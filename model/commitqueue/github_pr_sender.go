package commitqueue

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

type prService interface {
	Merge(context.Context, string, string, int, string, *github.PullRequestOptions) (*github.PullRequestMergeResult, *github.Response, error)
}

type statusSender interface {
	Send(message.Composer)
}

type githubPRLogger struct {
	ctx          context.Context
	prService    prService
	statusSender statusSender
	*send.Base
}

// NewGithubPRLogger creates a new Sender implementation that
// merges a pull request
// Specify an OAuth token for GitHub authentication
func NewGithubPRLogger(ctx context.Context, name string, token string, statusSender send.Sender) (send.Sender, error) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	tc, err := util.GetOAuth2HTTPClient(token)
	if err != nil {
		defer cancel()
		return nil, errors.Wrap(err, "can't get oauth session")
	}

	base := send.MakeBase(name, func() {}, func() error {
		util.PutHTTPClient(tc)
		cancel()
		return nil
	})

	githubClient := github.NewClient(tc)
	s := &githubPRLogger{
		Base:         base,
		ctx:          ctx,
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

	msg, ok := m.Raw().(*GithubMergePR)
	if !ok {
		s.ErrorHandler(errors.New("message of type githubPRLogger does not contain a GithubMergePR"), m)
		return
	}

	s.sendPatchResult(msg)
	if !msg.PatchSucceeded {
		return
	}

	mergeOpts := &github.PullRequestOptions{
		MergeMethod: msg.MergeMethod,
		CommitTitle: msg.CommitTitle,
	}

	// do the merge
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	res, _, err := s.prService.Merge(ctx, msg.Owner, msg.Repo, msg.PRNum, msg.CommitMessage, mergeOpts)

	if err != nil {
		s.ErrorHandler(errors.Wrap(err, "can't access GitHub merge API"), m)
	}

	// send the result to github
	s.ErrorHandler(s.sendMergeResult(res, msg), m)

	// dequeue the PR from its commit queue
	cq, err := FindOneId(msg.ProjectID)
	if err != nil {
		s.ErrorHandler(errors.Wrapf(err, "can't find commit queue for %s", msg.ProjectID), m)
		return
	}
	found, err := cq.Remove(strconv.Itoa(msg.PRNum))
	if err != nil {
		s.ErrorHandler(errors.Wrapf(err, "can't dequeue %d from commit queue", msg.PRNum), m)
	} else if !found {
		s.ErrorHandler(errors.Errorf("item %d did not exist on the queue", msg.PRNum), m)
	}
}

func (s *githubPRLogger) sendMergeResult(PRResult *github.PullRequestMergeResult, msg *GithubMergePR) error {
	var state message.GithubState
	var description string
	if PRResult.GetMerged() {
		state = message.GithubStateSuccess
		description = "merge succeeded"
	} else {
		state = message.GithubStateFailure
		description = fmt.Sprintf("merge failed: %s", PRResult.GetMessage())
	}

	status := message.GithubStatus{
		Owner:       msg.Owner,
		Repo:        msg.Repo,
		Ref:         msg.Ref,
		Context:     "evergreen/commitqueue",
		State:       state,
		Description: description,
	}
	c := message.NewGithubStatusMessageWithRepo(level.Notice, status)

	s.statusSender.Send(c)
	return nil
}

func (s *githubPRLogger) sendPatchResult(msg *GithubMergePR) {
	var state message.GithubState
	var description string
	if msg.PatchSucceeded {
		state = message.GithubStateSuccess
		description = "merge test succeeded"
	} else {
		state = message.GithubStateFailure
		description = "merge test failed"
	}

	status := message.GithubStatus{
		Owner:       msg.Owner,
		Repo:        msg.Repo,
		Ref:         msg.Ref,
		Context:     "evergreen/commitqueue",
		State:       state,
		Description: description,
		URL:         msg.URL,
	}
	c := message.NewGithubStatusMessageWithRepo(level.Notice, status)

	s.statusSender.Send(c)
}
