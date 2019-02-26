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

const CommitQueueContext = "evergreen/commitqueue"

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
		s.ErrorHandler(errors.New("not proceeding with merge for failed patch"), m)
		s.ErrorHandler(dequeueFromCommitQueue(msg.ProjectID, msg.PRNum), m)
		return
	}

	mergeOpts := &github.PullRequestOptions{
		MergeMethod: msg.MergeMethod,
		CommitTitle: msg.CommitTitle,
		SHA:         msg.Ref,
	}

	// do the merge
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	res, _, err := s.prService.Merge(ctx, msg.Owner, msg.Repo, msg.PRNum, msg.CommitMessage, mergeOpts)

	if err != nil {
		s.ErrorHandler(errors.Wrap(err, "can't access GitHub merge API"), m)
	}

	// send the result to github
	// only send if the merge was not successful since
	// GitHub won't display statuses received after the PR has been merged
	if !res.GetMerged() {
		s.ErrorHandler(s.sendMergeFailedStatus(res.GetMessage(), msg), m)
	}

	s.ErrorHandler(dequeueFromCommitQueue(msg.ProjectID, msg.PRNum), m)
}

func (s *githubPRLogger) sendMergeFailedStatus(githubMessage string, msg *GithubMergePR) error {
	state := message.GithubStateFailure
	description := fmt.Sprintf("merge failed: %s", githubMessage)

	status := message.GithubStatus{
		Owner:       msg.Owner,
		Repo:        msg.Repo,
		Ref:         msg.Ref,
		Context:     CommitQueueContext,
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
		Context:     CommitQueueContext,
		State:       state,
		Description: description,
		URL:         msg.URL,
	}
	c := message.NewGithubStatusMessageWithRepo(level.Notice, status)

	s.statusSender.Send(c)
}

func dequeueFromCommitQueue(projectID string, PRNum int) error {
	cq, err := FindOneId(projectID)
	if err != nil {
		return errors.Wrapf(err, "can't find commit queue for %s", projectID)
	}
	found, err := cq.Remove(strconv.Itoa(PRNum))
	if err != nil {
		return errors.Wrapf(err, "can't dequeue %d from commit queue", PRNum)
	} else if !found {
		return errors.Errorf("item %d did not exist on the queue", PRNum)
	}

	return nil
}
