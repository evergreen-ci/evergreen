package commitqueue

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

const Context = "evergreen/commitqueue"

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
		prService:    githubClient.PullRequests,
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
		s.ErrorHandler()(errors.New("message of type githubPRLogger does not contain a GithubMergePR"), m)
		return
	}

	for _, pr := range msg.PRs {
		s.sendPatchResult(msg, pr)
	}

	if !(msg.Status == evergreen.PatchSucceeded) {
		s.ErrorHandler()(errors.New("not proceeding with merge for failed patch"), m)
		event.LogCommitQueueConcludeTest(msg.PatchID, evergreen.MergeTestFailed)
		s.ErrorHandler()(dequeueFromCommitQueue(msg.ProjectID, msg.Item), m)

		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(len(msg.PRs)*10)*time.Second)
	defer cancel()
	for i, pr := range msg.PRs {
		mergeOpts := &github.PullRequestOptions{
			MergeMethod: msg.MergeMethod,
			CommitTitle: pr.CommitTitle,
			SHA:         pr.Ref,
		}

		// do the merge
		res, _, err := s.prService.Merge(ctx, pr.Owner, pr.Repo, pr.PRNum, "", mergeOpts)

		if err != nil {
			s.ErrorHandler()(errors.Wrap(err, "can't access GitHub merge API"), m)
			// don't send status to GitHub since we can't access their API anyway...
		}

		if !res.GetMerged() {
			s.ErrorHandler()(s.sendMergeFailedStatus(res.GetMessage(), pr), m)
			for j := i + 1; j < len(msg.PRs); j++ {
				s.ErrorHandler()(s.sendMergeFailedStatus("aborted", msg.PRs[j]), m)
			}
			event.LogCommitQueueConcludeTest(msg.PatchID, evergreen.MergeTestFailed)
			s.ErrorHandler()(dequeueFromCommitQueue(msg.ProjectID, msg.Item), m)
			return
		}
	}

	event.LogCommitQueueConcludeTest(msg.PatchID, evergreen.MergeTestSucceeded)
	s.ErrorHandler()(dequeueFromCommitQueue(msg.ProjectID, msg.Item), m)
}

func (s *githubPRLogger) sendMergeFailedStatus(githubMessage string, pr event.PRInfo) error {
	state := message.GithubStateFailure
	description := fmt.Sprintf("merge failed: %s", githubMessage)

	status := message.GithubStatus{
		Owner:       pr.Owner,
		Repo:        pr.Repo,
		Ref:         pr.Ref,
		Context:     Context,
		State:       state,
		Description: description,
	}
	c := message.NewGithubStatusMessageWithRepo(level.Notice, status)

	s.statusSender.Send(c)
	return nil
}

func (s *githubPRLogger) sendPatchResult(msg *GithubMergePR, pr event.PRInfo) {
	var state message.GithubState
	var description string
	if msg.Status == evergreen.PatchSucceeded {
		state = message.GithubStateSuccess
		description = "merge test succeeded"
	} else {
		state = message.GithubStateFailure
		description = "merge test failed"
	}

	status := message.GithubStatus{
		Owner:       pr.Owner,
		Repo:        pr.Repo,
		Ref:         pr.Ref,
		Context:     Context,
		State:       state,
		Description: description,
		URL:         msg.URL,
	}
	c := message.NewGithubStatusMessageWithRepo(level.Notice, status)

	s.statusSender.Send(c)
}

func dequeueFromCommitQueue(projectID string, item string) error {
	cq, err := FindOneId(projectID)
	if err != nil {
		return errors.Wrapf(err, "can't find commit queue for '%s'", projectID)
	}
	found, err := cq.Remove(item)
	if err != nil {
		return errors.Wrapf(err, "can't dequeue '%s' from commit queue", item)
	}
	if !found {
		return errors.Errorf("item '%s' did not exist on the queue", item)
	}

	return nil
}
