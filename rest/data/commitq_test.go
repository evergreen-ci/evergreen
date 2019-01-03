package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type CommitQSuite struct {
	ctx Connector
	suite.Suite
}

func TestCommitQSuite(t *testing.T) {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s := &CommitQSuite{}
	suite.Run(t, s)
}

func (s *CommitQSuite) SetupTest() {
	s.Require().NoError(db.Clear(commitqueue.Collection))
	s.Require().NoError(db.Clear(model.ProjectRefCollection))
	projRef := model.ProjectRef{
		Identifier:     "mci",
		Owner:          "evergreen-ci",
		Repo:           "evergreen",
		Branch:         "master",
		CommitQEnabled: true,
	}
	s.Require().NoError(projRef.Insert())
	q := &commitqueue.CommitQueue{ProjectID: "mci"}
	s.Require().NoError(commitqueue.InsertQueue(q))
}

func (s *CommitQSuite) TestEnqueue() {
	s.ctx = &DBConnector{}
	s.NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", "1234"))

	q, err := commitqueue.FindOneId("mci")
	s.NoError(err)

	if s.Len(q.Queue, 1) {
		s.Equal(q.Queue[0], "1234")
	}
}

func (s *CommitQSuite) TestMockGithubPREnqueue() {
	s.ctx = &MockConnector{}
	s.NoError(s.ctx.GithubPREnqueueItem("evergreen-ci", "evergreen", 1234))
	conn := s.ctx.(*MockConnector)
	q, ok := conn.MockCommitQConnector.Queue["evergreen-ci.evergreen.master"]
	if s.True(ok) && s.Len(q, 1) {
		s.Equal(q[0], "1234")
	}
}

func (s *CommitQSuite) TestMockEnqueue() {
	s.ctx = &MockConnector{}
	s.NoError(s.ctx.EnqueueItem("evergreen-ci", "evergreen", "master", "1234"))

	conn := s.ctx.(*MockConnector)
	q, ok := conn.MockCommitQConnector.Queue["evergreen-ci.evergreen.master"]
	if s.True(ok) && s.Len(q, 1) {
		s.Equal(q[0], "1234")
	}
}
