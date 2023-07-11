package model

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type CommitQueueSuite struct {
	suite.Suite
	q *commitqueue.CommitQueue
}

func TestCommitQueueSuite(t *testing.T) {
	s := new(CommitQueueSuite)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	originalEnv := evergreen.GetEnvironment()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	defer func() {
		evergreen.SetEnvironment(originalEnv)
	}()
	suite.Run(t, s)
}

func (s *CommitQueueSuite) SetupTest() {
	s.NoError(db.ClearCollections(commitqueue.Collection))

	s.q = &commitqueue.CommitQueue{
		ProjectID: "mci",
	}

	s.NoError(commitqueue.InsertQueue(s.q))
	q, err := commitqueue.FindOneId("mci")
	s.NotNil(q)
	s.NoError(err)
}
func (s *CommitQueueSuite) TestPreventMergeForItem() {
	s.NoError(db.ClearCollections(event.SubscriptionsCollection, task.Collection, build.Collection, VersionCollection))

	patchID := "abcdef012345"

	item := commitqueue.CommitQueueItem{
		Issue:   patchID,
		Source:  commitqueue.SourceDiff,
		Version: patchID,
	}

	v := &Version{
		Id: patchID,
	}
	s.NoError(v.Insert())

	mergeBuild := &build.Build{Id: "b1"}
	s.NoError(mergeBuild.Insert())
	mergeTask := &task.Task{Id: "t1", CommitQueueMerge: true, Version: patchID, BuildId: "b1"}
	s.NoError(mergeTask.Insert())

	// With a corresponding version
	s.NoError(preventMergeForItem(item, "user"))
	subscriptions, err := event.FindSubscriptionsByAttributes(event.ResourceTypePatch, event.Attributes{ID: []string{patchID}})
	s.NoError(err)
	s.Empty(subscriptions)

	mergeTask, err = task.FindOneId("t1")
	s.NoError(err)
	s.Equal(evergreen.DisabledTaskPriority, mergeTask.Priority)
	s.False(mergeTask.Activated)
}
