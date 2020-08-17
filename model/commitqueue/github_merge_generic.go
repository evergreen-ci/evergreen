package commitqueue

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

const (
	// valid Github merge methods
	githubMergeMethodMerge  = "merge"
	githubMergeMethodSquash = "squash"
	githubMergeMethodRebase = "rebase"

	GitHubContext = "evergreen/commitqueue"
)

var validMergeMethods = []string{githubMergeMethodMerge, githubMergeMethodSquash, githubMergeMethodRebase}

type GithubMergePR struct {
	Status      string         `bson:"status"`
	PatchID     string         `bson:"patch_id"`
	URL         string         `bson:"url"`
	ProjectID   string         `bson:"project_id"`
	MergeMethod string         `bson:"merge_method"`
	Item        string         `bson:"item"`
	PRs         []event.PRInfo `bson:"prs"`

	statusSender send.Sender
	gitHubToken  string
}

func (s *GithubMergePR) Initialize(env evergreen.Environment) error {
	githubToken, err := env.Settings().GetGithubOauthToken()
	if err != nil {
		return errors.Wrap(err, "can't get github token from settings")
	}

	githubStatusSender, err := env.GetSender(evergreen.SenderGithubStatus)
	if err != nil {
		return errors.Wrap(err, "can't get github status sender")
	}

	s.gitHubToken = githubToken
	s.statusSender = githubStatusSender

	return nil
}

func (s *GithubMergePR) Send() error {
	tc := utility.GetOAuth2HTTPClient(s.gitHubToken)
	defer utility.PutHTTPClient(tc)
	githubClient := github.NewClient(tc)

	for _, pr := range s.PRs {
		s.sendPatchResult(pr)
	}

	if s.Status != evergreen.PatchSucceeded {
		event.LogCommitQueueConcludeTest(s.PatchID, evergreen.MergeTestFailed)
		catcher := grip.NewBasicCatcher()
		catcher.Add(s.dequeueFromCommitQueue())
		catcher.Add(errors.New("not proceeding with merge for failed patch"))
		return catcher.Resolve()
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(len(s.PRs)*10)*time.Second)
	defer cancel()
	for i, pr := range s.PRs {
		mergeOpts := &github.PullRequestOptions{
			MergeMethod: s.MergeMethod,
			CommitTitle: pr.CommitTitle,
			SHA:         pr.Ref,
		}

		// do the merge
		res, _, err := githubClient.PullRequests.Merge(ctx, pr.Owner, pr.Repo, pr.PRNum, "", mergeOpts)
		if err != nil {
			return errors.Wrap(err, "can't access GitHub merge API")
			// don't send status to GitHub since we can't access their API anyway...
		}

		if !res.GetMerged() {
			catcher := grip.NewBasicCatcher()
			s.sendMergeFailedStatus(res.GetMessage(), pr)
			for j := i + 1; j < len(s.PRs); j++ {
				s.sendMergeFailedStatus("aborted", s.PRs[j])
			}
			event.LogCommitQueueConcludeTest(s.PatchID, evergreen.MergeTestFailed)
			catcher.Add(s.dequeueFromCommitQueue())
			return catcher.Resolve()
		}
	}

	event.LogCommitQueueConcludeTest(s.PatchID, evergreen.MergeTestSucceeded)
	return s.dequeueFromCommitQueue()
}

func (s *GithubMergePR) String() string {
	return fmt.Sprintf("GitHub commit queue merge '%s'", s.Item)
}

func (s *GithubMergePR) Valid() bool {
	if len(s.ProjectID) == 0 || len(s.Item) == 0 || len(s.Status) == 0 {
		return false
	}
	for _, pr := range s.PRs {
		if len(pr.Owner) == 0 || len(pr.Repo) == 0 || len(pr.Ref) == 0 || pr.PRNum <= 0 {
			return false
		}
	}

	if len(s.MergeMethod) > 0 && !utility.StringSliceContains(validMergeMethods, s.MergeMethod) {
		return false
	}

	return true
}

func (s *GithubMergePR) sendMergeFailedStatus(githubMessage string, pr event.PRInfo) {
	state := message.GithubStateFailure
	description := fmt.Sprintf("merge failed: %s", githubMessage)

	status := message.GithubStatus{
		Owner:       pr.Owner,
		Repo:        pr.Repo,
		Ref:         pr.Ref,
		Context:     GitHubContext,
		State:       state,
		Description: description,
	}
	c := message.NewGithubStatusMessageWithRepo(level.Notice, status)

	s.statusSender.Send(c)
}

func (s *GithubMergePR) sendPatchResult(pr event.PRInfo) {
	state := message.GithubStateFailure
	description := "merge test failed"
	if s.Status == evergreen.PatchSucceeded {
		state = message.GithubStateSuccess
		description = "merge test succeeded"
	}

	status := message.GithubStatus{
		Owner:       pr.Owner,
		Repo:        pr.Repo,
		Ref:         pr.Ref,
		Context:     GitHubContext,
		State:       state,
		Description: description,
		URL:         s.URL,
	}
	c := message.NewGithubStatusMessageWithRepo(level.Notice, status)

	s.statusSender.Send(c)
}

func (s *GithubMergePR) dequeueFromCommitQueue() error {
	cq, err := FindOneId(s.ProjectID)
	if err != nil {
		return errors.Wrapf(err, "can't find commit queue for '%s'", s.ProjectID)
	}
	if cq == nil {
		return errors.Errorf("no commit queue found for '%s'", s.ProjectID)
	}
	found, err := cq.Remove(s.Item)
	if err != nil {
		return errors.Wrapf(err, "can't dequeue '%s' from commit queue", s.Item)
	}
	if !found {
		return errors.Errorf("item '%s' did not exist on the queue", s.Item)
	}

	return nil
}
