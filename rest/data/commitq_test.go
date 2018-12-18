package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitq"
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
	s.Require().NoError(db.Clear(commitq.Collection))
	s.Require().NoError(db.Clear(model.ProjectRefCollection))
	projRef := model.ProjectRef{Identifier: "mci"}
	s.Require().NoError(projRef.Insert())
	q := &commitq.CommitQueue{ProjectID: "mci"}
	s.Require().NoError(commitq.InsertQueue(q))
}

func (s *CommitQSuite) TestEnqueue() {
	s.ctx = &DBConnector{}
	s.NoError(s.ctx.EnqueueItem("mci", "item1"))

	q, err := commitq.FindOneId("mci")
	s.NoError(err)

	if s.Len(q.Queue, 1) {
		s.Equal(q.Queue[0], "item1")
	}
}

func (s *CommitQSuite) TestMockEnqueue() {
	s.ctx = &MockConnector{}
	s.NoError(s.ctx.EnqueueItem("mci", "item1"))

	conn := s.ctx.(*MockConnector)
	q, ok := conn.MockCommitQConnector.Queue["mci"]
	if s.True(ok) && s.Len(q, 1) {
		s.Equal(q[0], "item1")
	}
}
