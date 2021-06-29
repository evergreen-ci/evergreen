package commitqueue

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	s.Equal(0, pos)
	s.Require().Len(s.q.Queue, 1)
	s.Equal("c123", s.q.Queue[0].Issue)
	s.NotEqual(-1, s.q.FindItem("c123"))

	// Persisted to db
	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Len(dbq.Queue, 1)
	s.Equal(sampleCommitQueueItem.Issue, dbq.Queue[0].Issue)

	// Ensure EnqueueTime set
	s.False(dbq.Queue[0].EnqueueTime.IsZero())

	s.NotEqual(-1, dbq.FindItem("c123"))
}

func (s *CommitQueueSuite) TestEnqueueAtFront() {
	// if queue is empty, puts as the first item
	pos, err := s.q.EnqueueAtFront(sampleCommitQueueItem)
	s.Require().NoError(err)
	s.Equal(pos, 0)

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
	s.Equal(2, pos)

	// no processing items in queue - should go at the very front
	item.Issue = "critical1"
	pos, err = s.q.EnqueueAtFront(item)
	s.Require().NoError(err)
	s.Equal(0, pos)

	dbq, err = FindOneId("mci")
	s.NoError(err)
	s.Require().Len(dbq.Queue, 4)
	s.Equal("critical1", dbq.Queue[0].Issue)

	// check that it's enqueued at the end of the processing items
	item.Version = "critical1"
	s.NoError(dbq.UpdateVersion(item))
	item = sampleCommitQueueItem
	item.Issue = "critical2"
	pos, err = dbq.EnqueueAtFront(item)
	s.NoError(err)
	s.Equal(1, pos)
	dbq, err = FindOneId("mci")
	s.NoError(err)
	s.Len(dbq.Queue, 5)
	s.Equal("critical2", dbq.Queue[1].Issue)
}

func (s *CommitQueueSuite) TestEnqueueAtFrontFullQueue() {
	item := sampleCommitQueueItem
	item.Version = "abc"
	_, err := s.q.Enqueue(item)
	s.Require().NoError(err)

	item = sampleCommitQueueItem
	item.Issue = "new"
	pos, err := s.q.EnqueueAtFront(item)
	s.NoError(err)
	s.Equal(1, pos)
	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Len(dbq.Queue, 2)
	s.Equal("new", dbq.Queue[1].Issue)
}

func (s *CommitQueueSuite) TestUpdateVersion() {
	_, err := s.q.Enqueue(sampleCommitQueueItem)
	s.NoError(err)

	item := s.q.Queue[0]
	item.Version = "my_version"
	now := time.Now()
	s.NoError(s.q.UpdateVersion(item))

	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Len(dbq.Queue, 1)

	s.Equal(item.Issue, dbq.Queue[0].Issue)
	s.Equal(item.Version, dbq.Queue[0].Version)
	s.InDelta(now.Unix(), dbq.Queue[0].ProcessingStartTime.Unix(), float64(1*time.Millisecond))
}

func (s *CommitQueueSuite) TestNext() {
	// nothing is enqueued
	next, valid := s.q.Next()
	s.False(valid)
	s.Empty(next.Issue)

	// enqueue something
	pos, err := s.q.Enqueue(sampleCommitQueueItem)
	s.NoError(err)
	s.Equal(0, pos)

	// get it off the queue
	next, valid = s.q.Next()
	s.True(valid)
	s.Equal("c123", next.Issue)
}

func (s *CommitQueueSuite) TestRemoveOne() {
	item := sampleCommitQueueItem
	pos, err := s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	item.Issue = "d234"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	item.Issue = "e345"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(2, pos)
	s.Require().Len(s.q.Queue, 3)

	found, err := s.q.Remove("not_here")
	s.NoError(err)
	s.Nil(found)

	found, err = s.q.Remove("d234")
	s.NoError(err)
	s.NotNil(found)
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

	found, err = s.q.Remove("c123")
	s.NotNil(found)
	s.NoError(err)
	s.NotNil(s.q.Queue[0])
	s.Equal(s.q.Queue[0].Issue, "e345")
}

func (s *CommitQueueSuite) TestClearAll() {
	item := sampleCommitQueueItem
	pos, err := s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	item.Issue = "d234"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	item.Issue = "e345"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(2, pos)
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
	s.Require().Equal(0, pos)
	item.Issue = "d234"
	pos, err = q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
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

	cq, err := FindOneId("mci")
	s.NoError(err)
	s.Equal("mci", cq.ProjectID)

	cq, err = FindOneId("not_here")
	s.NoError(err)
	s.Nil(cq)
}

func TestPreventMergeForItem(t *testing.T) {
	assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection, task.Collection, build.Collection))

	patchID := "abcdef012345"

	item := CommitQueueItem{
		Issue:   patchID,
		Source:  SourceDiff,
		Version: patchID,
	}

	mergeBuild := &build.Build{Id: "b1"}
	require.NoError(t, mergeBuild.Insert())
	mergeTask := &task.Task{Id: "t1", CommitQueueMerge: true, Version: patchID, BuildId: "b1"}
	require.NoError(t, mergeTask.Insert())

	// With a corresponding version
	assert.NoError(t, preventMergeForItem(item, "user"))
	subscriptions, err := event.FindSubscriptions(event.ResourceTypePatch, []event.Selector{{Type: event.SelectorID, Data: patchID}})
	assert.NoError(t, err)
	assert.Empty(t, subscriptions)

	mergeTask, err = task.FindOneId("t1")
	assert.NoError(t, err)
	assert.Equal(t, int64(-1), mergeTask.Priority)
}

func TestNextUnprocessed(t *testing.T) {
	q := CommitQueue{
		Queue: []CommitQueueItem{},
	}
	assert.Len(t, q.NextUnprocessed(1), 0)
	assert.Len(t, q.NextUnprocessed(0), 0)
	q.Queue = append(q.Queue, CommitQueueItem{Issue: "1"})
	assert.Len(t, q.NextUnprocessed(1), 1)
	assert.Len(t, q.NextUnprocessed(0), 0)
	q.Queue = append(q.Queue, CommitQueueItem{Issue: "2"})
	next2 := q.NextUnprocessed(2)
	assert.Equal(t, "1", next2[0].Issue)
	assert.Equal(t, "2", next2[1].Issue)
	assert.Len(t, q.NextUnprocessed(0), 0)
	assert.Len(t, q.NextUnprocessed(5), 2)
	q.Queue = append(q.Queue, CommitQueueItem{Issue: "3"})
	next2 = q.NextUnprocessed(2)
	assert.Equal(t, "1", next2[0].Issue)
	assert.Equal(t, "2", next2[1].Issue)
	assert.Len(t, next2, 2)
	q.Queue = append(q.Queue, CommitQueueItem{Issue: "4", Version: "4"})
	next4 := q.NextUnprocessed(4)
	assert.Equal(t, "1", next4[0].Issue)
	assert.Equal(t, "2", next4[1].Issue)
	assert.Equal(t, "3", next4[2].Issue)
	assert.Len(t, next4, 3)
}

func TestProcessing(t *testing.T) {
	q := CommitQueue{
		Queue: []CommitQueueItem{
			{Issue: "1", Version: "1"},
			{Issue: "2", Version: "2"},
			{Issue: "3", Version: "3"},
			{Issue: "4"},
		},
	}
	assert.True(t, q.Processing())

	q = CommitQueue{
		Queue: []CommitQueueItem{
			{Issue: "1"},
			{Issue: "2"},
			{Issue: "3"},
			{Issue: "4"},
		},
	}
	assert.False(t, q.Processing())

	q = CommitQueue{
		Queue: []CommitQueueItem{},
	}
	assert.False(t, q.Processing())
}
