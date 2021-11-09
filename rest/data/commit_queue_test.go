package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
	mgobson "gopkg.in/mgo.v2/bson"
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
		Id:               "mci",
		Owner:            "evergreen-ci",
		Repo:             "evergreen",
		Branch:           "main",
		Enabled:          utility.TruePtr(),
		PatchingDisabled: utility.FalsePtr(),
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	s.Require().NoError(s.projectRef.Insert())
	s.queue = &commitqueue.CommitQueue{ProjectID: "mci"}
	s.Require().NoError(commitqueue.InsertQueue(s.queue))
}

func (s *CommitQueueSuite) TestEnqueue() {
	s.ctx = &DBConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("1234")}, false)
	s.NoError(err)
	s.Equal(0, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("5678")}, false)
	s.NoError(err)
	s.Equal(1, pos)

	q, err := commitqueue.FindOneId("mci")
	s.NoError(err)
	s.Require().Len(q.Queue, 2)
	s.Equal("1234", q.Queue[0].Issue)
	s.Equal("5678", q.Queue[1].Issue)

	// move to front
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("important")}, true)
	s.NoError(err)
	s.Equal(0, pos)
	q, err = commitqueue.FindOneId("mci")
	s.NoError(err)
	s.Require().Len(q.Queue, 3)
	s.Equal("important", q.Queue[0].Issue)
}

func (s *CommitQueueSuite) TestFindCommitQueueByID() {
	s.ctx = &DBConnector{}
	cq, err := s.ctx.FindCommitQueueForProject("mci")
	s.NoError(err)
	s.Equal(utility.ToStringPtr("mci"), cq.ProjectID)
}

func (s *CommitQueueSuite) TestCommitQueueRemoveItem() {
	s.ctx = &DBConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("1")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("2")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("3")}, false)
	s.Require().NoError(err)
	s.Require().Equal(2, pos)

	found, err := s.ctx.CommitQueueRemoveItem("mci", "not_here", "user")
	s.Error(err)
	s.Nil(found)

	found, err = s.ctx.CommitQueueRemoveItem("mci", "1", "user")
	s.NoError(err)
	s.NotNil(found)
	cq, err := s.ctx.FindCommitQueueForProject("mci")
	s.NoError(err)
	s.Equal(utility.ToStringPtr("2"), cq.Queue[0].Issue)
	s.Equal(utility.ToStringPtr("3"), cq.Queue[1].Issue)
}

func (s *CommitQueueSuite) TestIsItemOnCommitQueue() {
	s.ctx = &DBConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("1")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)

	exists, err := s.ctx.IsItemOnCommitQueue("mci", "1")
	s.NoError(err)
	s.True(exists)

	exists, err = s.ctx.IsItemOnCommitQueue("mci", "2")
	s.NoError(err)
	s.False(exists)

	exists, err = s.ctx.IsItemOnCommitQueue("not-a-project", "1")
	s.Error(err)
	s.False(exists)
}

func (s *CommitQueueSuite) TestCommitQueueClearAll() {
	s.ctx = &DBConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("12")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("34")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("56")}, false)
	s.Require().NoError(err)
	s.Require().Equal(2, pos)

	q := &commitqueue.CommitQueue{ProjectID: "logkeeper"}
	s.Require().NoError(commitqueue.InsertQueue(q))

	// Only one queue is cleared since the second is empty
	clearedCount, err := s.ctx.CommitQueueClearAll()
	s.NoError(err)
	s.Equal(1, clearedCount)

	// both queues have items
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("12")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = q.Enqueue(commitqueue.CommitQueueItem{Issue: "78"})
	s.NoError(err)
	s.Equal(0, pos)
	clearedCount, err = s.ctx.CommitQueueClearAll()
	s.NoError(err)
	s.Equal(2, clearedCount)
}

func (s *CommitQueueSuite) TestIsAuthorizedToPatchAndMerge() {
	args1 := UserRepoInfo{
		Username: "evrg-bot-webhook",
		Owner:    "evergreen-ci",
		Repo:     "evergreen",
	}
	args2 := UserRepoInfo{
		Username: "octocat",
		Owner:    "evergreen-ci",
		Repo:     "evergreen",
	}
	c := &MockCommitQueueConnector{
		UserPermissions: map[UserRepoInfo]string{
			args1: "admin",
			args2: "read",
		},
	}
	ctx := context.Background()
	authorized, err := c.IsAuthorizedToPatchAndMerge(ctx, s.settings, args1)
	s.NoError(err)
	s.True(authorized)

	authorized, err = c.IsAuthorizedToPatchAndMerge(ctx, s.settings, args2)
	s.NoError(err)
	s.False(authorized)
}

func (s *CommitQueueSuite) TestCreatePatchForMerge() {
	s.ctx = &DBConnector{}
	s.Require().NoError(db.ClearCollections(patch.Collection, model.ProjectAliasCollection, user.Collection))

	u := &user.DBUser{Id: "octocat"}
	s.Require().NoError(u.Insert())

	cqAlias := model.ProjectAlias{
		ProjectID: s.projectRef.Id,
		Alias:     evergreen.CommitQueueAlias,
		Variant:   "v0",
		Task:      "t0",
	}
	s.Require().NoError(cqAlias.Upsert())

	existingPatch := &patch.Patch{
		Author:  "octocat",
		Project: s.projectRef.Id,
		GitInfo: &patch.GitMetadata{
			Username: "octocat",
			Email:    "octocat @github.com",
		},
		PatchedConfig: `
tasks:
  - name: t0
buildvariants:
  - name: v0
    tasks:
    - name: "t0"
`,
	}
	s.Require().NoError(existingPatch.Insert())
	existingPatch, err := patch.FindOne(db.Q{})
	s.Require().NoError(err)
	s.Require().NotNil(existingPatch)

	newPatch, err := s.ctx.CreatePatchForMerge(context.Background(), existingPatch.Id.Hex(), "")
	s.NoError(err)
	s.NotNil(newPatch)

	newPatchDB, err := patch.FindOneId(utility.FromStringPtr(newPatch.Id))
	s.NoError(err)
	s.Equal(evergreen.CommitQueueAlias, newPatchDB.Alias)
}

func (s *CommitQueueSuite) TestMockGetGitHubPR() {
	s.ctx = &MockConnector{}
	pr, err := s.ctx.GetGitHubPR(context.Background(), "evergreen-ci", "evergreen", 1234)
	s.NoError(err)

	s.Require().NotNil(pr.User.ID)
	s.Equal(1234, int(*pr.User.ID))

	s.Require().NotNil(pr.Base.Ref)
	s.Equal("main", *pr.Base.Ref)
}

func (s *CommitQueueSuite) TestMockEnqueue() {
	s.ctx = &MockConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("1234")}, false)
	s.NoError(err)
	s.Equal(0, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("5678")}, false)
	s.NoError(err)
	s.Equal(1, pos)

	conn := s.ctx.(*MockConnector)
	q, ok := conn.MockCommitQueueConnector.Queue["mci"]
	s.True(ok)
	s.Require().Len(q, 2)

	s.Equal("1234", utility.FromStringPtr(q[0].Issue))
	s.Equal("5678", utility.FromStringPtr(q[1].Issue))

	// move to front
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("important")}, true)
	s.NoError(err)
	s.Equal(1, pos)
	q, ok = conn.MockCommitQueueConnector.Queue["mci"]
	s.True(ok)
	s.Require().Len(q, 3)

	s.Equal("1234", utility.FromStringPtr(q[0].Issue))
	s.Equal("important", utility.FromStringPtr(q[1].Issue))
	s.Equal("5678", utility.FromStringPtr(q[2].Issue))

}

func (s *CommitQueueSuite) TestMockFindCommitQueueForProject() {
	s.ctx = &MockConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("1234")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)

	cq, err := s.ctx.FindCommitQueueForProject("mci")
	s.NoError(err)
	s.Equal(utility.ToStringPtr("mci"), cq.ProjectID)
	s.Equal(utility.ToStringPtr("1234"), cq.Queue[0].Issue)
}

func (s *CommitQueueSuite) TestMockCommitQueueRemoveItem() {
	s.ctx = &MockConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("1")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("2")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("3")}, false)
	s.Require().NoError(err)
	s.Require().Equal(2, pos)

	found, err := s.ctx.CommitQueueRemoveItem("mci", "not_here", "user")
	s.NoError(err)
	s.Nil(found)

	found, err = s.ctx.CommitQueueRemoveItem("mci", "1", "user")
	s.NoError(err)
	s.NotNil(found)
	cq, err := s.ctx.FindCommitQueueForProject("mci")
	s.NoError(err)
	s.Equal(utility.ToStringPtr("2"), cq.Queue[0].Issue)
	s.Equal(utility.ToStringPtr("3"), cq.Queue[1].Issue)
}

func (s *CommitQueueSuite) TestMockIsItemOnCommitQueue() {
	s.ctx = &MockConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("1")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)

	exists, err := s.ctx.IsItemOnCommitQueue("mci", "1")
	s.NoError(err)
	s.True(exists)

	exists, err = s.ctx.IsItemOnCommitQueue("mci", "2")
	s.NoError(err)
	s.False(exists)

	exists, err = s.ctx.IsItemOnCommitQueue("not-a-project", "1")
	s.Error(err)
	s.False(exists)
}

func (s *CommitQueueSuite) TestMockCommitQueueClearAll() {
	s.ctx = &MockConnector{}
	pos, err := s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("12")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = s.ctx.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("34")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)

	pos, err = s.ctx.EnqueueItem("logkeeper", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("12")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = s.ctx.EnqueueItem("logkeeper", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("34")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)

	clearedCount, err := s.ctx.CommitQueueClearAll()
	s.NoError(err)
	s.Equal(2, clearedCount)
}

func (s *CommitQueueSuite) TestWritePatchInfo() {
	s.NoError(db.ClearGridCollections(patch.GridFSPrefix))

	patchDoc := &patch.Patch{
		Id:      mgobson.ObjectIdHex("aabbccddeeff112233445566"),
		Githash: "abcdef",
	}

	patchSummaries := []thirdparty.Summary{
		thirdparty.Summary{
			Name:      "myfile.go",
			Additions: 1,
			Deletions: 0,
		},
	}

	patchContents := `diff --git a/myfile.go b/myfile.go
	index abcdef..123456 100644
	--- a/myfile.go
	+++ b/myfile.go
	@@ +2,1 @@ func myfunc {
	+				fmt.Print(\"hello world\")
			}
	`

	s.NoError(writePatchInfo(patchDoc, patchSummaries, patchContents))
	s.Len(patchDoc.Patches, 1)
	s.Equal(patchSummaries, patchDoc.Patches[0].PatchSet.Summary)
	storedPatchContents, err := patch.FetchPatchContents(patchDoc.Patches[0].PatchSet.PatchFileId)
	s.NoError(err)
	s.Equal(patchContents, storedPatchContents)
}

func TestConcludeMerge(t *testing.T) {
	require.NoError(t, db.Clear(commitqueue.Collection))
	projectID := "evergreen"
	itemID := bson.NewObjectId()
	p := patch.Patch{
		Id:      itemID,
		Project: projectID,
	}
	assert.NoError(t, p.Insert())
	queue := &commitqueue.CommitQueue{
		ProjectID: projectID,
		Queue:     []commitqueue.CommitQueueItem{{Issue: itemID.Hex(), Version: itemID.Hex()}},
	}
	require.NoError(t, commitqueue.InsertQueue(queue))
	dc := &DBCommitQueueConnector{}

	assert.NoError(t, dc.ConcludeMerge(itemID.Hex(), "foo"))

	queue, err := commitqueue.FindOneId(projectID)
	require.NoError(t, err)
	assert.Len(t, queue.Queue, 0)
}
