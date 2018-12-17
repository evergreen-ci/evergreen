package commitq

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type CommitQueueSuite struct {
	suite.Suite
	q *CommitQueue
}

func TestCommitQueueSuite(t *testing.T) {
	s := new(CommitQueueSuite)
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	suite.Run(t, s)
}

func (s *CommitQueueSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(Collection))
	s.Require().NoError(db.ClearCollections(model.ProjectRefCollection))

	projRef := model.ProjectRef{
		Identifier: "mci",
	}
	s.Require().NoError(projRef.Insert())

	s.q = &CommitQueue{
		ProjectID:    "mci",
		MergeAction:  "squash",
		StatusAction: "github",
	}

	s.NoError(InsertQueue(s.q))
	q, err := FindOneId("mci")
	s.Require().NotNil(q)
	s.Require().NoError(err)
}

func (s *CommitQueueSuite) TestEnqueue() {
	s.NoError(s.q.Enqueue("c123"))
	s.False(s.q.IsEmpty())
	s.NotEqual(-1, s.q.findItem("c123"))

	// Persisted to db
	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.False(dbq.IsEmpty())
	s.NotEqual(-1, dbq.findItem("c123"))
}

func (s *CommitQueueSuite) TestAll() {
	s.Require().NoError(s.q.Enqueue("c123"))
	s.Require().NoError(s.q.Enqueue("d234"))
	s.Require().NoError(s.q.Enqueue("e345"))

	items := s.q.All()
	s.Len(items, 3)
	s.Equal("c123", items[0])
	s.Equal("d234", items[1])
	s.Equal("e345", items[2])
}

func (s *CommitQueueSuite) TestRemoveOne() {
	s.Require().NoError(s.q.Enqueue("c123"))
	s.Require().NoError(s.q.Enqueue("d234"))
	s.Require().NoError(s.q.Enqueue("e345"))
	s.Require().Len(s.q.All(), 3)

	s.Error(s.q.Remove("not_here"))

	s.NoError(s.q.Remove("d234"))
	items := s.q.All()
	s.Len(items, 2)
	// Still in order
	s.Equal("c123", items[0])
	s.Equal("e345", items[1])

	// Persisted to db
	dbq, err := FindOneId("mci")
	s.NoError(err)
	items = dbq.All()
	s.Len(items, 2)
	s.Equal("c123", items[0])
	s.Equal("e345", items[1])
}

func (s *CommitQueueSuite) TestRemoveAll() {
	s.Require().NoError(s.q.Enqueue("c123"))
	s.Require().NoError(s.q.Enqueue("d234"))
	s.Require().NoError(s.q.Enqueue("e345"))
	s.Require().Len(s.q.All(), 3)

	s.NoError(s.q.RemoveAll())
	s.Empty(s.q.All())

	// Persisted to db
	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Empty(dbq.All())
}

func (s *CommitQueueSuite) TestUpdateMerge() {
	s.Equal("squash", s.q.MergeAction)
	s.NoError(s.q.UpdateMergeAction("rebase"))
	s.Equal("rebase", s.q.MergeAction)

	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Equal("rebase", dbq.MergeAction)
}

func (s *CommitQueueSuite) TestUpdateStatus() {
	s.Equal("github", s.q.StatusAction)
	s.NoError(s.q.UpdateStatusAction("email"))
	s.Equal("email", s.q.StatusAction)

	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Equal("email", dbq.StatusAction)
}

func (s *CommitQueueSuite) TestCommentTrigger() {
	comment := "no dice"
	action := "create"
	s.False(TriggersCommitQueue(action, comment))

	comment = triggerComment
	s.True(TriggersCommitQueue(action, comment))

	action = "delete"
	s.False(TriggersCommitQueue(action, comment))
}
