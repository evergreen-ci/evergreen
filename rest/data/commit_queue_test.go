package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type CommitQueueSuite struct {
	ctx Connector
	suite.Suite
}

func TestCommitQueueSuite(t *testing.T) {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s := &CommitQueueSuite{}
	suite.Run(t, s)
}

func (s *CommitQueueSuite) SetupTest() {
	s.Require().NoError(db.Clear(commitqueue.Collection))
	s.Require().NoError(db.Clear(model.ProjectRefCollection))
	projRef := model.ProjectRef{
		Identifier:         "mci",
		Owner:              "evergreen-ci",
		Repo:               "evergreen",
		Branch:             "master",
		CommitQueueEnabled: true,
	}
	s.Require().NoError(projRef.Insert())
	q := &commitqueue.CommitQueue{ProjectID: "mci"}
	s.Require().NoError(commitqueue.InsertQueue(q))
}

func (s *CommitQueueSuite) TestEnqueue() {
	s.ctx = &DBConnector{}
	s.NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", "1234"))

	q, err := commitqueue.FindOneId("mci")
	s.NoError(err)

	if s.Len(q.Queue, 1) {
		s.Equal(q.Queue[0], "1234")
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
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", "1"))
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", "2"))
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", "3"))

	found, err := s.ctx.CommitQueueRemoveItem("mci", "not_here")
	s.NoError(err)
	s.False(found)

	found, err = s.ctx.CommitQueueRemoveItem("mci", "1")
	s.NoError(err)
	s.True(found)
	cq, err := s.ctx.FindCommitQueueByID("mci")
	s.NoError(err)
	s.Equal(restModel.ToAPIString("2"), cq.Queue[0])
	s.Equal(restModel.ToAPIString("3"), cq.Queue[1])
}

func (s *CommitQueueSuite) TestMockGetGitHubPR() {
	s.ctx = &MockConnector{}
	pr, err := s.ctx.GetGitHubPR(context.Background(), "evergreen-ci", "evergreen", 1234)
	s.NoError(err)

	s.Require().NotNil(pr.User.ID)
	s.Equal(1234, *pr.User.ID)

	s.Require().NotNil(pr.Base.Label)
	s.Equal("master", *pr.Base.Label)
}

func (s *CommitQueueSuite) TestMockEnqueue() {
	s.ctx = &MockConnector{}
	s.NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", "1234"))

	conn := s.ctx.(*MockConnector)
	q, ok := conn.MockCommitQueueConnector.Queue["evergreen-ci.evergreen.master"]
	if s.True(ok) && s.Len(q, 1) {
		s.Equal(restModel.ToAPIString("1234"), q[0])
	}
}

func (s *CommitQueueSuite) TestMockFindCommitQueueByID() {
	s.ctx = &MockConnector{}
	s.NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", "1234"))

	cq, err := s.ctx.FindCommitQueueByID("evergreen-ci.evergreen.master")
	s.NoError(err)
	s.Equal(restModel.ToAPIString("evergreen-ci.evergreen.master"), cq.ProjectID)
	s.Equal([]restModel.APIString{restModel.ToAPIString("1234")}, cq.Queue)
}

func (s *CommitQueueSuite) TestMockCommitQueueRemoveItem() {
	s.ctx = &MockConnector{}
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", "1"))
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", "2"))
	s.Require().NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", "3"))

	found, err := s.ctx.CommitQueueRemoveItem("evergreen-ci.evergreen.master", "not_here")
	s.NoError(err)
	s.False(found)

	found, err = s.ctx.CommitQueueRemoveItem("evergreen-ci.evergreen.master", "1")
	s.NoError(err)
	s.True(found)
	cq, err := s.ctx.FindCommitQueueByID("evergreen-ci.evergreen.master")
	s.NoError(err)
	s.Equal(restModel.ToAPIString("2"), cq.Queue[0])
	s.Equal(restModel.ToAPIString("3"), cq.Queue[1])
}
