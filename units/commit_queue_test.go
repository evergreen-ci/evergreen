package units

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/suite"
)

type commitQueueSuite struct {
	suite.Suite

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
		Identifier:         "mci",
		Owner:              "baxterthehacker",
		Repo:               "public-repo",
		CommitQConfigFile:  "test_config.yaml",
		CommitQMergeMethod: "squash",
	}
	s.Require().NoError(s.projectRef.Insert())
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
	job := NewCommitQueueJob("mci")
	s.Equal("commit-queue:mci", job.ID())
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

func (s *commitQueueSuite) TestMakeMergePatch() {
	p, err := makeMergePatch(s.pr, s.projectRef.Identifier, "a string")
	s.NoError(err)
	s.Equal(s.projectRef.Identifier, p.Project)
	s.Equal(evergreen.PatchCreated, p.Status)
	s.Equal(*s.pr.MergeCommitSHA, p.GithubPatchData.MergeCommitSHA)
}

func (s *commitQueueSuite) TestGetPatchUser() {
	uid := 1234
	u, err := getPatchUser(uid)
	s.NoError(err)
	s.Require().NotNil(u)
	s.Equal(evergreen.GithubPatchUser, u.Id)

	u = &user.DBUser{
		Id:       "me",
		DispName: "baxtor",
		Settings: user.UserSettings{
			GithubUser: user.GithubUser{
				UID: uid,
			},
		},
		APIKey: util.RandomString(),
	}
	s.NoError(u.Insert())
	u, err = getPatchUser(uid)
	s.Equal("me", u.Id)
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

func (s *commitQueueSuite) TestMakeVersionURL() {
	s.NoError(db.ClearCollections(evergreen.ConfigCollection))
	uiConfig := evergreen.UIConfig{
		Url: "baxter.thehacker.com",
	}
	uiConfig.Set()

	url, err := makeVersionURL("abcde")
	s.NoError(err)
	s.Equal("baxter.thehacker.com/version/abcde", url)
}
