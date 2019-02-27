package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type CommitQueueSuite struct {
	ctx      Connector
	settings *evergreen.Settings
	suite.Suite
}

func TestCommitQueueSuite(t *testing.T) {
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	testutil.ConfigureIntegrationTest(t, testConfig, "TestCommitQueueSuite")
	s := &CommitQueueSuite{settings: testConfig}
	suite.Run(t, s)
}

func (s *CommitQueueSuite) SetupTest() {
	s.Require().NoError(db.Clear(commitqueue.Collection))
	s.Require().NoError(db.Clear(model.ProjectRefCollection))
	projRef := model.ProjectRef{
		Identifier: "mci",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "master",
		CommitQueue: model.CommitQueueParams{
			Enabled: true,
		},
	}
	s.Require().NoError(projRef.Insert())
	q := &commitqueue.CommitQueue{ProjectID: "mci"}
	s.Require().NoError(commitqueue.InsertQueue(q))
}

func (s *CommitQueueSuite) TestEnqueue() {
	s.ctx = &DBConnector{}
	s.NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("1234")}))

	q, err := commitqueue.FindOneId("mci")
	s.NoError(err)

	if s.Len(q.Queue, 1) {
		s.Equal(q.Queue[0].Issue, "1234")
	}
}

func (s *CommitQueueSuite) TestFindCommitQueueByID() {
	s.ctx = &DBConnector{}
	cq, err := s.ctx.FindCommitQueueByID("mci")
	s.NoError(err)
	s.Equal(restModel.ToAPIString("mci"), cq.ProjectID)
}

func (s *CommitQueueSuite) TestCommitQueueRemoveItem() {
	s.ctx = &DBConnector{}
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("1")}))
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("2")}))
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("3")}))

	found, err := s.ctx.CommitQueueRemoveItem("mci", "not_here")
	s.NoError(err)
	s.False(found)

	found, err = s.ctx.CommitQueueRemoveItem("mci", "1")
	s.NoError(err)
	s.True(found)
	cq, err := s.ctx.FindCommitQueueByID("mci")
	s.NoError(err)
	s.Equal(restModel.ToAPIString("2"), cq.Queue[0].Issue)
	s.Equal(restModel.ToAPIString("3"), cq.Queue[1].Issue)
}

func (s *CommitQueueSuite) TestCommitQueueClearAll() {
	s.ctx = &DBConnector{}
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("12")}))
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("34")}))
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("56")}))

	q := &commitqueue.CommitQueue{ProjectID: "logkeeper"}
	s.Require().NoError(commitqueue.InsertQueue(q))

	// Only one queue is cleared since the second is empty
	clearedCount, err := s.ctx.CommitQueueClearAll()
	s.NoError(err)
	s.Equal(1, clearedCount)

	// both queues have items
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("12")}))
	s.NoError(q.Enqueue(commitqueue.CommitQueueItem{Issue: "78"}))
	clearedCount, err = s.ctx.CommitQueueClearAll()
	s.NoError(err)
	s.Equal(2, clearedCount)
}

func (s *CommitQueueSuite) TestIsAuthorizedToPatchAndMerge() {
	s.ctx = &DBConnector{}
	ctx := context.Background()

	args := UserRepoInfo{
		Username: "evrg-bot-webhook",
		Owner:    "evergreen-ci",
		Repo:     "evergreen",
	}
	authorized, err := s.ctx.IsAuthorizedToPatchAndMerge(ctx, s.settings, args)
	s.NoError(err)
	s.True(authorized)

	args = UserRepoInfo{
		Username: "octocat",
		Owner:    "evergreen-ci",
		Repo:     "evergreen",
	}
	authorized, err = s.ctx.IsAuthorizedToPatchAndMerge(ctx, s.settings, args)
	s.NoError(err)
	s.False(authorized)
}

func (s *CommitQueueSuite) TestMockGetGitHubPR() {
	s.ctx = &MockConnector{}
	pr, err := s.ctx.GetGitHubPR(context.Background(), "evergreen-ci", "evergreen", 1234)
	s.NoError(err)

	s.Require().NotNil(pr.User.ID)
	s.Equal(1234, *pr.User.ID)

	s.Require().NotNil(pr.Base.Ref)
	s.Equal("master", *pr.Base.Ref)
}

func (s *CommitQueueSuite) TestMockEnqueue() {
	s.ctx = &MockConnector{}
	s.NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("1234")}))

	conn := s.ctx.(*MockConnector)
	q, ok := conn.MockCommitQueueConnector.Queue["evergreen-ci.evergreen.master"]
	if s.True(ok) && s.Len(q, 1) {
		s.Equal(restModel.ToAPIString("1234"), q[0].Issue)
	}
}

func (s *CommitQueueSuite) TestMockFindCommitQueueByID() {
	s.ctx = &MockConnector{}
	s.NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("1234")}))

	cq, err := s.ctx.FindCommitQueueByID("evergreen-ci.evergreen.master")
	s.NoError(err)
	s.Equal(restModel.ToAPIString("evergreen-ci.evergreen.master"), cq.ProjectID)
	s.Equal(restModel.ToAPIString("1234"), cq.Queue[0].Issue)
}

func (s *CommitQueueSuite) TestMockCommitQueueRemoveItem() {
	s.ctx = &MockConnector{}
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("1")}))
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("2")}))
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("3")}))

	found, err := s.ctx.CommitQueueRemoveItem("evergreen-ci.evergreen.master", "not_here")
	s.NoError(err)
	s.False(found)

	found, err = s.ctx.CommitQueueRemoveItem("evergreen-ci.evergreen.master", "1")
	s.NoError(err)
	s.True(found)
	cq, err := s.ctx.FindCommitQueueByID("evergreen-ci.evergreen.master")
	s.NoError(err)
	s.Equal(restModel.ToAPIString("2"), cq.Queue[0].Issue)
	s.Equal(restModel.ToAPIString("3"), cq.Queue[1].Issue)
}

func (s *CommitQueueSuite) TestMockCommitQueueClearAll() {
	s.ctx = &MockConnector{}
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("12")}))
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("34")}))

	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "logkeeper", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("56")}))
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "logkeeper", "master", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("78")}))

	clearedCount, err := s.ctx.CommitQueueClearAll()
	s.NoError(err)
	s.Equal(2, clearedCount)
}
