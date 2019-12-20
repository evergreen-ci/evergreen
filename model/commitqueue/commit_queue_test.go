package commitqueue

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	s.Equal(sampleCommitQueueItem.Issue, dbq.Next().Issue)

	// Ensure EnqueueTime set
	s.False(dbq.Next().EnqueueTime.IsZero())

	s.NotEqual(-1, dbq.FindItem("c123"))
}

func (s *CommitQueueSuite) TestEnqueueAtFront() {
	// if queue is empty, puts as the first item
	pos, err := s.q.EnqueueAtFront(sampleCommitQueueItem)
	s.Require().NoError(err)
	s.Equal(pos, 1)

	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Len(dbq.Queue, 1)

	// insert different items
	item := sampleCommitQueueItem
	item.Issue = "456"
	_, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	item.Issue = "789"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Equal(3, pos)

	item.Issue = "critical"
	pos, err = s.q.EnqueueAtFront(item)
	s.Require().NoError(err)
	s.Equal(2, pos)

	dbq, err = FindOneId("mci")
	s.NoError(err)
	s.Require().Len(dbq.Queue, 4)
	s.Equal("critical", dbq.Queue[1].Issue)
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

	s.Equal(item.Issue, dbq.Next().Issue)
	s.Equal(item.Version, dbq.Next().Version)
}

func (s *CommitQueueSuite) TestNext() {
	pos, err := s.q.Enqueue(sampleCommitQueueItem)
	s.NoError(err)
	s.Equal(1, pos)
	s.Len(s.q.Queue, 1)
	s.Require().NotNil(s.q.Next())
	s.Equal("c123", s.q.Next().Issue)
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

	found, err := s.q.Remove("not_here")
	s.NoError(err)
	s.False(found)

	found, err = s.q.Remove("d234")
	s.NoError(err)
	s.True(found)
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
	found, err = s.q.Remove("c123")
	s.True(found)
	s.NoError(err)
	s.NotNil(s.q.Next())
	s.Equal(s.q.Next().Issue, "e345")
	s.False(s.q.Processing)
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

func TestPreventMergeForItemPR(t *testing.T) {
	assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection))

	patchID := "abcdef012345"
	patchSub := event.NewPatchOutcomeSubscription(patchID, event.NewGithubMergeSubscriber(event.GithubMergeSubscriber{}))
	require.NoError(t, patchSub.Upsert())

	item := CommitQueueItem{
		Issue:   "1234",
		Version: patchID,
	}

	assert.NoError(t, preventMergeForItem(PRPatchType, false, &item))
	subscriptions, err := event.FindSubscriptions(event.ResourceTypePatch, []event.Selector{{Type: event.SelectorID, Data: item.Version}})
	assert.NoError(t, err)
	assert.Empty(t, subscriptions)
}

func TestPreventMergeForItemCLI(t *testing.T) {
	assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection, task.Collection))

	patchID := "abcdef012345"
	patchSub := event.NewPatchOutcomeSubscription(patchID, event.NewCommitQueueDequeueSubscriber())
	require.NoError(t, patchSub.Upsert())

	item := CommitQueueItem{
		Issue: patchID,
	}

	mergeTask := &task.Task{Id: "t1", CommitQueueMerge: true, Version: patchID}
	require.NoError(t, mergeTask.Insert())

	// Without a corresponding version
	assert.NoError(t, preventMergeForItem(CLIPatchType, false, &item))
	subscriptions, err := event.FindSubscriptions(event.ResourceTypePatch, []event.Selector{{Type: event.SelectorID, Data: patchID}})
	assert.NoError(t, err)
	assert.NotEmpty(t, subscriptions)

	mergeTask, err = task.FindOneId("t1")
	assert.NoError(t, err)
	assert.Equal(t, int64(0), mergeTask.Priority)

	// With a corresponding version
	assert.NoError(t, preventMergeForItem(CLIPatchType, true, &item))
	subscriptions, err = event.FindSubscriptions(event.ResourceTypePatch, []event.Selector{{Type: event.SelectorID, Data: patchID}})
	assert.NoError(t, err)
	assert.Empty(t, subscriptions)

	mergeTask, err = task.FindOneId("t1")
	assert.NoError(t, err)
	assert.Equal(t, int64(-1), mergeTask.Priority)
}

func TestClearVersionPatchSubscriber(t *testing.T) {
	require.NoError(t, db.Clear(event.SubscriptionsCollection))

	patchID := "abcdef012345"
	patchSub := event.NewPatchOutcomeSubscription(patchID, event.NewCommitQueueDequeueSubscriber())
	assert.NoError(t, patchSub.Upsert())

	assert.NoError(t, clearVersionPatchSubscriber(patchID, event.CommitQueueDequeueSubscriberType))
	subs, err := event.FindSubscriptions(event.ResourceTypePatch, []event.Selector{{Type: event.SelectorID, Data: patchID}})
	assert.NoError(t, err)
	assert.Empty(t, subs)
}
