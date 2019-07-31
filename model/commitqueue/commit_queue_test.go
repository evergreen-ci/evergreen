package commitqueue

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	_ "github.com/evergreen-ci/evergreen/testutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

type CommitQueueSuite struct {
	suite.Suite
	q *CommitQueue
}

var sampleCommitQueueItem = CommitQueueItem{
	Issue: "c123",
	Modules: []Module{
		Module{
			Module: "test_module",
			Issue:  "d234",
		},
	},
}

func TestCommitQueueSuite(t *testing.T) {
	s := new(CommitQueueSuite)
	suite.Run(t, s)
}

func (s *CommitQueueSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(Collection))

	s.q = &CommitQueue{
		ProjectID: "mci",
	}

	s.NoError(InsertQueue(s.q))
	q, err := FindOneId("mci")
	s.Require().NotNil(q)
	s.Require().NoError(err)
}

func (s *CommitQueueSuite) TestEnqueue() {
	pos, err := s.q.Enqueue(sampleCommitQueueItem)
	s.Require().NoError(err)
	s.Equal(1, pos)
	s.Require().Len(s.q.Queue, 1)
	s.Equal("c123", s.q.Next().Issue)
	s.NotEqual(-1, s.q.FindItem("c123"))

	// Persisted to db
	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Len(dbq.Queue, 1)
	s.Equal(sampleCommitQueueItem, *dbq.Next())
	s.NotEqual(-1, dbq.FindItem("c123"))
}

func (s *CommitQueueSuite) TestUpdateVersion() {
	_, err := s.q.Enqueue(sampleCommitQueueItem)
	s.NoError(err)

	item := s.q.Next()
	s.Equal("c123", item.Issue)
	s.Equal("", s.q.Next().Version)
	item.Version = "my_version"
	s.NoError(s.q.UpdateVersion(*item))
	s.Equal("my_version", s.q.Next().Version)

	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Len(dbq.Queue, 1)
	s.Equal(item, dbq.Next())
}

func (s *CommitQueueSuite) TestNext() {
	pos, err := s.q.Enqueue(sampleCommitQueueItem)
	s.NoError(err)
	s.Equal(1, pos)
	s.Len(s.q.Queue, 1)

	s.NoError(s.q.SetProcessing(true))
	s.Nil(s.q.Next())

	s.NoError(s.q.SetProcessing(false))
	s.Require().NotNil(s.q.Next())
	s.Equal(s.q.Next().Issue, "c123")
}

func (s *CommitQueueSuite) TestRemoveOne() {
	item := sampleCommitQueueItem
	pos, err := s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	item.Issue = "d234"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(2, pos)
	item.Issue = "e345"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(3, pos)
	s.Require().Len(s.q.Queue, 3)

	itemRemoved, err := s.q.Remove("not_here")
	s.NoError(err)
	s.Nil(itemRemoved)

	itemRemoved, err = s.q.Remove("d234")
	s.NoError(err)
	s.Equal("d234", itemRemoved.Issue)
	items := s.q.Queue
	s.Len(items, 2)
	// Still in order
	s.Equal("c123", items[0].Issue)
	s.Equal("e345", items[1].Issue)

	// Persisted to db
	dbq, err := FindOneId("mci")
	s.NoError(err)
	items = dbq.Queue
	s.Len(items, 2)
	s.Equal("c123", items[0].Issue)
	s.Equal("e345", items[1].Issue)

	s.NoError(s.q.SetProcessing(true))
	s.Nil(s.q.Next())
	itemRemoved, err = s.q.Remove("c123")
	s.Equal("c123", itemRemoved.Issue)
	s.NoError(err)
	s.NotNil(s.q.Next())
	s.Equal(s.q.Next().Issue, "e345")
}

func (s *CommitQueueSuite) TestClearAll() {
	item := sampleCommitQueueItem
	pos, err := s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	item.Issue = "d234"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(2, pos)
	item.Issue = "e345"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(3, pos)
	s.Require().Len(s.q.Queue, 3)

	q := &CommitQueue{
		ProjectID: "logkeeper",
		Queue:     []CommitQueueItem{},
	}
	s.Require().NoError(InsertQueue(q))

	// Only one commit queue has contents
	clearedCount, err := ClearAllCommitQueues()
	s.NoError(err)
	s.Equal(1, clearedCount)

	s.q, err = FindOneId("mci")
	s.NoError(err)
	s.Empty(s.q.Queue)
	q, err = FindOneId("logkeeper")
	s.NoError(err)
	s.Empty(q.Queue)

	// both have contents
	item.Issue = "c1234"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	item.Issue = "d234"
	pos, err = q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	clearedCount, err = ClearAllCommitQueues()
	s.NoError(err)
	s.Equal(2, clearedCount)
}

func (s *CommitQueueSuite) TestCommentTrigger() {
	comment := "no dice"
	action := "created"
	s.False(TriggersCommitQueue(action, comment))

	comment = triggerComment
	s.True(TriggersCommitQueue(action, comment))

	action = "deleted"
	s.False(TriggersCommitQueue(action, comment))
}

func (s *CommitQueueSuite) TestFindOneId() {
	s.NoError(db.ClearCollections(Collection))
	cq := &CommitQueue{ProjectID: "mci"}
	s.NoError(InsertQueue(cq))

	_, err := FindOneId("mci")
	s.NoError(err)
	s.Equal("mci", cq.ProjectID)

	_, err = FindOneId("not_here")
	s.Error(err)
	s.True(adb.ResultsNotFound(err))
}

func (s *CommitQueueSuite) TestSetupEnv() {
	ctx := context.Background()
	env := testutil.NewEnvironment(ctx, s.T())
	testConfig := env.Settings()
	githubToken, err := testConfig.GetGithubOauthToken()
	s.NoError(err)

	githubStatusSender, err := send.NewGithubStatusLogger("evergreen", &send.GithubOptions{
		Token: githubToken,
	}, "")
	s.NoError(err)
	s.NotNil(githubStatusSender)
	s.NoError(env.SetSender(evergreen.SenderGithubStatus, githubStatusSender))

	sender, err := env.GetSender(evergreen.SenderCommitQueueDequeue)
	s.Nil(sender)
	s.Error(err)
	sender, err = env.GetSender(evergreen.SenderGithubMerge)
	s.Nil(sender)
	s.Error(err)

	s.NoError(SetupEnv(env))

	sender, err = env.GetSender(evergreen.SenderCommitQueueDequeue)
	s.NotNil(sender)
	s.NoError(err)
	sender, err = env.GetSender(evergreen.SenderGithubMerge)
	s.NotNil(sender)
	s.NoError(err)
}
