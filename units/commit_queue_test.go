package units

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/suite"
)

type commitQueueSuite struct {
	suite.Suite
	env *mock.Environment
	ctx context.Context

	prBody     []byte
	pr         *github.PullRequest
	projectRef *model.ProjectRef
}

func TestCommitQueueJob(t *testing.T) {
	suite.Run(t, &commitQueueSuite{})
}

func (s *commitQueueSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s.NoError(db.ClearCollections(model.ProjectRefCollection))
	var err error
	s.prBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "pull_request.json"))
	s.NoError(err)
	s.Require().Len(s.prBody, 24757)

	s.projectRef = &model.ProjectRef{
		Identifier:             "mci",
		Owner:                  "baxterthehacker",
		Repo:                   "public-repo",
		CommitQueueConfigFile:  "test_config.yaml",
		CommitQueueMergeMethod: "squash",
	}
	s.Require().NoError(s.projectRef.Insert())

	s.env = &mock.Environment{}
	s.ctx = context.Background()
	s.NoError(s.env.Configure(s.ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
}

func (s *commitQueueSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(commitqueue.Collection, user.Collection))

	webhookInterface, err := github.ParseWebHook("pull_request", s.prBody)
	s.NoError(err)
	prEvent, ok := webhookInterface.(*github.PullRequestEvent)
	s.True(ok)
	s.pr = prEvent.GetPullRequest()

	cq := &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue: []string{
			"1",
			"2",
			"3",
			"4",
		},
		MergeAction:  "github",
		StatusAction: "github",
	}
	s.Require().NoError(commitqueue.InsertQueue(cq))
}

func (s *commitQueueSuite) TestNewCommitQueueJob() {
	job := NewCommitQueueJob(s.env, "mci", "job-1")
	s.Equal("commit-queue:mci_job-1", job.ID())
}

func (s *commitQueueSuite) TestValidatePR() {
	s.NoError(validatePR(s.pr))

	mergeCommitSha := s.pr.MergeCommitSHA
	s.pr.MergeCommitSHA = nil
	s.Error(validatePR(s.pr))
	s.pr.MergeCommitSHA = mergeCommitSha

	s.pr.Base = nil
	s.Error(validatePR(s.pr))
}

func (s *commitQueueSuite) TestSubscribeMerge() {
	s.NoError(db.ClearCollections(event.SubscriptionsCollection))
	s.NoError(subscribeMerge(s.projectRef, s.pr, "abcdef"))

	selectors := []event.Selector{
		event.Selector{
			Type: "id",
			Data: "abcdef",
		},
	}
	subscriptions, err := event.FindSubscriptions(event.ResourceTypePatch, selectors)
	s.NoError(err)
	s.Require().Len(subscriptions, 1)

	subscription := subscriptions[0]

	s.Equal(event.GithubMergeSubscriberType, subscription.Subscriber.Type)
	s.Equal(event.ResourceTypePatch, subscription.ResourceType)
	s.Equal(event.GithubMergeSubscriberType, subscription.Subscriber.Type)
	target, ok := subscription.Subscriber.Target.(*event.GithubMergeSubscriber)
	s.True(ok)
	s.Equal(s.projectRef.Identifier, target.ProjectID)
	s.Equal(s.projectRef.Owner, target.Owner)
	s.Equal(s.projectRef.Repo, target.Repo)
	s.Equal(s.pr.GetTitle(), target.CommitTitle)
}
