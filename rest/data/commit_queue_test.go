package data

import (
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

func (s *CommitQueueSuite) TestMockGithubPREnqueue() {
	s.ctx = &MockConnector{}
	s.NoError(s.ctx.GithubPREnqueueItem("evergreen-ci", "evergreen", 1234))
	conn := s.ctx.(*MockConnector)
	q, ok := conn.MockCommitQueueConnector.Queue["evergreen-ci.evergreen.master"]
	if s.True(ok) && s.Len(q, 1) {
		s.Equal(restModel.ToAPIString("1234"), q[0])
	}
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
