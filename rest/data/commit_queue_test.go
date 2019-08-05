package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type CommitQueueSuite struct {
	ctx      Connector
	settings *evergreen.Settings
	suite.Suite

	projectRef *model.ProjectRef
	queue      *commitqueue.CommitQueue
}

func TestCommitQueueSuite(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestCommitQueueSuite")
	s := &CommitQueueSuite{settings: testConfig}
	suite.Run(t, s)
}

func (s *CommitQueueSuite) SetupTest() {
	s.Require().NoError(db.Clear(commitqueue.Collection))
	s.Require().NoError(db.Clear(model.ProjectRefCollection))
	s.projectRef = &model.ProjectRef{
		Identifier: "mci",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "master",
		CommitQueue: model.CommitQueueParams{
			Enabled: true,
		},
	}
	s.Require().NoError(s.projectRef.Insert())
	s.queue = &commitqueue.CommitQueue{ProjectID: "mci"}
	s.Require().NoError(commitqueue.InsertQueue(s.queue))
}

func (s *CommitQueueSuite) TestEnqueue() {
	s.ctx = &DBConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("1234")})
	s.NoError(err)
	s.Equal(1, pos)

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
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("1")})
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("2")})
	s.Require().NoError(err)
	s.Require().Equal(2, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("3")})
	s.Require().NoError(err)
	s.Require().Equal(3, pos)

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
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("12")})
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("34")})
	s.Require().NoError(err)
	s.Require().Equal(2, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("56")})
	s.Require().NoError(err)
	s.Require().Equal(3, pos)

	q := &commitqueue.CommitQueue{ProjectID: "logkeeper"}
	s.Require().NoError(commitqueue.InsertQueue(q))

	// Only one queue is cleared since the second is empty
	clearedCount, err := s.ctx.CommitQueueClearAll()
	s.NoError(err)
	s.Equal(1, clearedCount)

	// both queues have items
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("12")})
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = q.Enqueue(commitqueue.CommitQueueItem{Issue: "78"})
	s.NoError(err)
	s.Equal(1, pos)
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

func (s *CommitQueueSuite) TestPreventMergeForItemPR() {
	s.NoError(db.ClearCollections(event.SubscriptionsCollection))

	s.projectRef.CommitQueue.PatchType = commitqueue.PRPatchType
	s.NoError(s.projectRef.Upsert())

	patchID := "abcdef012345"
	patchSub := event.NewPatchOutcomeSubscription(patchID, event.NewGithubMergeSubscriber(event.GithubMergeSubscriber{}))
	s.Require().NoError(patchSub.Upsert())

	item := commitqueue.CommitQueueItem{
		Issue:   "1234",
		Version: patchID,
	}
	_, err := s.queue.Enqueue(item)
	s.Require().NoError(err)

	s.NoError(preventMergeForItem(s.projectRef.Identifier, &item))
	subscriptions, err := event.FindSubscriptions(event.ResourceTypePatch, []event.Selector{{Type: event.SelectorID, Data: item.Version}})
	s.NoError(err)
	s.Empty(subscriptions)
}

func (s *CommitQueueSuite) TestPreventMergeForItemCLI() {
	s.NoError(db.ClearCollections(event.SubscriptionsCollection, task.Collection, model.VersionCollection))

	s.projectRef.CommitQueue.PatchType = commitqueue.CLIPatchType
	s.NoError(s.projectRef.Upsert())

	patchID := "abcdef012345"
	patchSub := event.NewPatchOutcomeSubscription(patchID, event.NewCommitQueueDequeueSubscriber())
	s.Require().NoError(patchSub.Upsert())

	item := commitqueue.CommitQueueItem{
		Issue: patchID,
	}
	_, err := s.queue.Enqueue(item)
	s.Require().NoError(err)

	mergeTask := &task.Task{Id: "t1", CommitQueueMerge: true, Version: patchID}
	s.Require().NoError(mergeTask.Insert())

	// Without a corresponding version
	s.NoError(preventMergeForItem(s.projectRef.Identifier, &item))
	subscriptions, err := event.FindSubscriptions(event.ResourceTypePatch, []event.Selector{{Type: event.SelectorID, Data: patchID}})
	s.NoError(err)
	s.NotEmpty(subscriptions)

	mergeTask, err = task.FindOneId("t1")
	s.NoError(err)
	s.Equal(int64(0), mergeTask.Priority)

	// With a corresponding version
	version := model.Version{Id: patchID}
	s.Require().NoError(version.Insert())

	s.NoError(preventMergeForItem(s.projectRef.Identifier, &item))
	subscriptions, err = event.FindSubscriptions(event.ResourceTypePatch, []event.Selector{{Type: event.SelectorID, Data: patchID}})
	s.NoError(err)
	s.Empty(subscriptions)

	mergeTask, err = task.FindOneId("t1")
	s.NoError(err)
	s.Equal(int64(-1), mergeTask.Priority)
}

func (s *CommitQueueSuite) TestClearVersionPatchSubscriber() {
	s.Require().NoError(db.Clear(event.SubscriptionsCollection))

	patchID := "abcdef012345"
	patchSub := event.NewPatchOutcomeSubscription(patchID, event.NewCommitQueueDequeueSubscriber())
	s.Require().NoError(patchSub.Upsert())

	s.NoError(clearVersionPatchSubscriber(patchID, event.CommitQueueDequeueSubscriberType))
	subs, err := event.FindSubscriptions(event.ResourceTypePatch, []event.Selector{{Type: event.SelectorID, Data: patchID}})
	s.NoError(err)
	s.Empty(subs)
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
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("1234")})
	s.Require().NoError(err)
	s.Require().Equal(1, pos)

	conn := s.ctx.(*MockConnector)
	q, ok := conn.MockCommitQueueConnector.Queue["mci"]
	if s.True(ok) && s.Len(q, 1) {
		s.Equal(restModel.ToAPIString("1234"), q[0].Issue)
	}
}

func (s *CommitQueueSuite) TestMockFindCommitQueueByID() {
	s.ctx = &MockConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("1234")})
	s.Require().NoError(err)
	s.Require().Equal(1, pos)

	cq, err := s.ctx.FindCommitQueueByID("mci")
	s.NoError(err)
	s.Equal(restModel.ToAPIString("mci"), cq.ProjectID)
	s.Equal(restModel.ToAPIString("1234"), cq.Queue[0].Issue)
}

func (s *CommitQueueSuite) TestMockCommitQueueRemoveItem() {
	s.ctx = &MockConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("1")})
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("2")})
	s.Require().NoError(err)
	s.Require().Equal(2, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("3")})
	s.Require().NoError(err)
	s.Require().Equal(3, pos)

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

func (s *CommitQueueSuite) TestMockCommitQueueClearAll() {
	s.ctx = &MockConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("12")})
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("34")})
	s.Require().NoError(err)
	s.Require().Equal(2, pos)

	pos, err = s.ctx.EnqueueItem("logkeeper", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("12")})
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.ctx.EnqueueItem("logkeeper", restModel.APICommitQueueItem{Issue: restModel.ToAPIString("34")})
	s.Require().NoError(err)
	s.Require().Equal(2, pos)

	clearedCount, err := s.ctx.CommitQueueClearAll()
	s.NoError(err)
	s.Equal(2, clearedCount)
}
