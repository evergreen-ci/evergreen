package data

import (
	"context"
	"strconv"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CommitQueueSuite struct {
	mockCtx  MockGitHubConnector
	settings *evergreen.Settings
	suite.Suite

	ctx    context.Context
	cancel context.CancelFunc

	projectRef *model.ProjectRef
	queue      *commitqueue.CommitQueue
}

func TestCommitQueueSuite(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, t.Name())
	s := &CommitQueueSuite{settings: testConfig}
	suite.Run(t, s)
}

func (s *CommitQueueSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.Require().NoError(db.ClearCollections(commitqueue.Collection, model.ProjectRefCollection, model.VersionCollection, patch.Collection, task.Collection))
	s.projectRef = &model.ProjectRef{
		Id:               "mci",
		Owner:            "evergreen-ci",
		Repo:             "evergreen",
		Branch:           "main",
		Enabled:          true,
		PatchingDisabled: utility.FalsePtr(),
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	s.Require().NoError(s.projectRef.Insert())
	s.queue = &commitqueue.CommitQueue{ProjectID: "mci"}
	s.Require().NoError(commitqueue.InsertQueue(s.queue))
	logkeeper := &model.ProjectRef{
		Id:               "logkeeper",
		Owner:            "evergreen-ci",
		Repo:             "evergreen",
		Branch:           "main",
		Enabled:          true,
		PatchingDisabled: utility.FalsePtr(),
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	s.Require().NoError(logkeeper.Insert())
	s.queue = &commitqueue.CommitQueue{ProjectID: "logkeeper"}
	s.Require().NoError(commitqueue.InsertQueue(s.queue))
}

func (s *CommitQueueSuite) TearDownTest() {
	s.cancel()
}

func (s *CommitQueueSuite) TestEnqueue() {
	pos, err := EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("1234")}, false)
	s.NoError(err)
	s.Equal(0, pos)
	pos, err = EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("5678")}, false)
	s.NoError(err)
	s.Equal(1, pos)

	q, err := commitqueue.FindOneId("mci")
	s.NoError(err)
	s.Require().Len(q.Queue, 2)
	s.Equal("1234", q.Queue[0].Issue)
	s.Equal("5678", q.Queue[1].Issue)

	// move to front
	pos, err = EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("important")}, true)
	s.NoError(err)
	s.Equal(0, pos)
	q, err = commitqueue.FindOneId("mci")
	s.NoError(err)
	s.Require().Len(q.Queue, 3)
	s.Equal("important", q.Queue[0].Issue)
}

func (s *CommitQueueSuite) TestFindCommitQueueByID() {
	cq, err := FindCommitQueueForProject("mci")
	s.NoError(err)
	s.Equal(utility.ToStringPtr("mci"), cq.ProjectID)
}

func (s *CommitQueueSuite) TestCommitQueueRemoveNonexistentItem() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	found, err := FindAndRemoveCommitQueueItem(ctx, "mci", "not_here", "user", "reason")
	s.Error(err)
	s.Nil(found)
}

func (s *CommitQueueSuite) TestCommitQueueRemoveUnfinalizedItem() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const project = "mci"

	for i := 0; i < 3; i++ {
		patchID := mgobson.NewObjectId()
		p := patch.Patch{
			Id:      patchID,
			Project: project,
		}
		s.Require().NoError(p.Insert())

		pos, err := EnqueueItem(project, restModel.APICommitQueueItem{
			Source:  utility.ToStringPtr(commitqueue.SourceDiff),
			PatchId: utility.ToStringPtr(p.Id.Hex()),
			Issue:   utility.ToStringPtr(strconv.Itoa(i)),
		}, false)
		s.Require().NoError(err)
		s.Require().Equal(i, pos)
	}

	found, err := FindAndRemoveCommitQueueItem(ctx, project, "0", "user", "reason")
	s.NoError(err)
	s.NotNil(found)
	cq, err := FindCommitQueueForProject(project)
	s.NoError(err)
	s.Require().Len(cq.Queue, 2)
	s.Equal("1", utility.FromStringPtr(cq.Queue[0].Issue))
	s.Equal("2", utility.FromStringPtr(cq.Queue[1].Issue))
}

func (s *CommitQueueSuite) TestCommitQueueRemoveFinalizedItem() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const project = "mci"

	for i := 0; i < 3; i++ {
		patchID := mgobson.NewObjectId()
		p := patch.Patch{
			Id:      patchID,
			Project: project,
			Version: patchID.Hex(),
		}
		s.Require().NoError(p.Insert())
		v := model.Version{
			Id:         patchID.Hex(),
			Identifier: project,
		}
		s.Require().NoError(v.Insert())
		testTask := task.Task{
			Id:      "test-" + strconv.Itoa(i),
			Project: project,
			Version: v.Id,
			Status:  evergreen.TaskUndispatched,
		}
		s.Require().NoError(testTask.Insert())
		mergeTask := task.Task{
			Id:               "merge-task-" + strconv.Itoa(i),
			CommitQueueMerge: true,
			Project:          project,
			Version:          v.Id,
			Status:           evergreen.TaskUndispatched,
		}
		s.Require().NoError(mergeTask.Insert())

		pos, err := EnqueueItem(project, restModel.APICommitQueueItem{
			Source:  utility.ToStringPtr(commitqueue.SourceDiff),
			PatchId: utility.ToStringPtr(p.Id.Hex()),
			Issue:   utility.ToStringPtr(strconv.Itoa(i)),
			Version: utility.ToStringPtr(v.Id),
		}, false)
		s.Require().NoError(err)
		s.Require().Equal(i, pos)
	}

	found, err := FindAndRemoveCommitQueueItem(ctx, project, "0", "user", "reason")
	s.NoError(err)
	s.NotNil(found)
	cq, err := FindCommitQueueForProject(project)
	s.NoError(err)
	s.Require().Len(cq.Queue, 2)
	s.Equal("1", utility.FromStringPtr(cq.Queue[0].Issue))
	s.Equal("2", utility.FromStringPtr(cq.Queue[1].Issue))
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
	c := &MockGitHubConnectorImpl{
		UserPermissions: map[UserRepoInfo]string{
			args1: "admin",
			args2: "read",
		},
	}
	authorized, err := c.IsAuthorizedToPatchAndMerge(s.ctx, s.settings, args1)
	s.NoError(err)
	s.True(authorized)

	authorized, err = c.IsAuthorizedToPatchAndMerge(s.ctx, s.settings, args2)
	s.NoError(err)
	s.False(authorized)
}

func (s *CommitQueueSuite) TestCreatePatchForMerge() {
	s.Require().NoError(db.ClearCollections(model.ParserProjectCollection, model.ProjectAliasCollection, user.Collection))

	u := &user.DBUser{Id: "octocat"}
	s.Require().NoError(u.Insert())

	cqAlias := model.ProjectAlias{
		ProjectID: s.projectRef.Id,
		Alias:     evergreen.CommitQueueAlias,
		Variant:   "v0",
		Task:      "t0",
	}
	s.Require().NoError(cqAlias.Upsert())

	const config = `
buildvariants:
- name: v0
  tasks:
    - t0
tasks:
- name: t0`

	existingPatchId := mgobson.ObjectIdHex("5e9748c4e3c331422d0d1d7a")
	existingPatch := &patch.Patch{
		Id:      existingPatchId,
		Author:  "octocat",
		Project: s.projectRef.Id,
		GitInfo: &patch.GitMetadata{
			Username: "octocat",
			Email:    "octocat @github.com",
		},
		ProjectStorageMethod: evergreen.ProjectStorageMethodDB,
		PatchedProjectConfig: config,
	}
	s.Require().NoError(existingPatch.Insert())

	existingPatchParserProject := &model.ParserProject{}
	s.Require().NoError(util.UnmarshalYAMLWithFallback([]byte(config), existingPatchParserProject))
	existingPatchParserProject.Id = existingPatch.Id.Hex()
	existingPatchParserProject.Identifier = utility.ToStringPtr(s.projectRef.Id)

	s.Require().NoError(existingPatchParserProject.Insert())

	existingPatch, err := patch.FindOne(db.Q{})
	s.Require().NoError(err)
	s.Require().NotNil(existingPatch)

	newPatch, err := CreatePatchForMerge(s.ctx, s.settings, existingPatch.Id.Hex(), "")
	s.NoError(err)
	s.NotNil(newPatch)

	newPatchDB, err := patch.FindOneId(utility.FromStringPtr(newPatch.Id))
	s.NoError(err)
	s.Equal(evergreen.CommitQueueAlias, newPatchDB.Alias)
}

func (s *CommitQueueSuite) TestMockGetGitHubPR() {
	pr, err := s.mockCtx.GetGitHubPR(s.ctx, "evergreen-ci", "evergreen", 1234)
	s.NoError(err)

	s.Require().NotNil(pr.User.ID)
	s.Equal(1234, int(*pr.User.ID))

	s.Require().NotNil(pr.Base.Ref)
	s.Equal("main", *pr.Base.Ref)
}

func (s *CommitQueueSuite) TestMockEnqueue() {
	pos, err := EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("1234")}, false)
	s.NoError(err)
	s.Equal(0, pos)
	pos, err = EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("5678")}, false)
	s.NoError(err)
	s.Equal(1, pos)

	cq, err := commitqueue.FindOneId("mci")
	s.NoError(err)
	s.Require().Len(cq.Queue, 2)

	s.Equal("1234", utility.FromStringPtr(&cq.Queue[0].Issue))
	s.Equal("5678", utility.FromStringPtr(&cq.Queue[1].Issue))

	// move to front
	pos, err = EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("important")}, true)
	s.NoError(err)
	s.Equal(0, pos)
	cq, err = commitqueue.FindOneId("mci")
	s.NoError(err)
	s.Require().Len(cq.Queue, 3)

	s.Equal("important", utility.FromStringPtr(&cq.Queue[0].Issue))
	s.Equal("1234", utility.FromStringPtr(&cq.Queue[1].Issue))
	s.Equal("5678", utility.FromStringPtr(&cq.Queue[2].Issue))

}

func (s *CommitQueueSuite) TestMockFindCommitQueueForProject() {
	pos, err := EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("1234")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)

	cq, err := FindCommitQueueForProject("mci")
	s.NoError(err)
	s.Equal(utility.ToStringPtr("mci"), cq.ProjectID)
	s.Equal(utility.ToStringPtr("1234"), cq.Queue[0].Issue)
}

func (s *CommitQueueSuite) TestWritePatchInfo() {
	s.NoError(db.ClearGridCollections(patch.GridFSPrefix))

	patchDoc := &patch.Patch{
		Id:      mgobson.ObjectIdHex("aabbccddeeff112233445566"),
		Githash: "abcdef",
		GithubPatchData: thirdparty.GithubPatch{
			CommitTitle:   "my title",
			CommitMessage: "more info",
		},
	}

	patchSummaries := []thirdparty.Summary{
		{
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
	s.Require().Len(patchDoc.Patches, 1)
	s.Equal(patchSummaries, patchDoc.Patches[0].PatchSet.Summary)
	s.Require().Len(patchDoc.Patches[0].PatchSet.CommitMessages, 1)
	s.Equal(patchDoc.Patches[0].PatchSet.CommitMessages[0], patchDoc.GithubPatchData.CommitTitle)
	storedPatchContents, err := patch.FetchPatchContents(patchDoc.Patches[0].PatchSet.PatchFileId)
	s.NoError(err)
	s.Equal(patchContents, storedPatchContents)
}

func TestConcludeMerge(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	assert.NoError(t, ConcludeMerge(ctx, itemID.Hex(), "foo"))

	queue, err := commitqueue.FindOneId(projectID)
	require.NoError(t, err)
	assert.Len(t, queue.Queue, 0)
}

func TestCheckCanRemoveCommitQueueItem(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(
		user.Collection,
		model.ProjectRefCollection,
		model.ProjectVarsCollection,
		evergreen.ScopeCollection,
		evergreen.RoleCollection,
		patch.Collection,
		commitqueue.Collection,
	))

	basicUser := &user.DBUser{Id: "me", OnlyAPI: false}
	require.NoError(t, basicUser.Insert())

	otherUser := &user.DBUser{Id: "other user", OnlyAPI: false}
	require.NoError(t, otherUser.Insert())

	serviceUser := &user.DBUser{Id: "service user", OnlyAPI: true}
	require.NoError(t, serviceUser.Insert())

	projectAdmin := &user.DBUser{Id: "admin", OnlyAPI: false}
	require.NoError(t, projectAdmin.Insert())

	project := &model.ProjectRef{
		Id:      "evergreen",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "main",
		Enabled: true,
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	require.NoError(t, project.Add(projectAdmin))

	myPatch := patch.Patch{
		Id:      bson.NewObjectId(),
		Project: "evergreen",
		Author:  basicUser.Id,
	}
	require.NoError(t, myPatch.Insert())

	otherPatch := patch.Patch{
		Id:      bson.NewObjectId(),
		Project: "evergreen",
		Author:  otherUser.Id,
	}
	require.NoError(t, otherPatch.Insert())

	servicePatch := patch.Patch{
		Id:      bson.NewObjectId(),
		Project: "evergreen",
		Author:  serviceUser.Id,
	}
	require.NoError(t, servicePatch.Insert())

	mockConnector := &MockGitHubConnector{
		MockGitHubConnectorImpl: MockGitHubConnectorImpl{},
	}

	// Basic user can remove their own CLI patch.
	err := CheckCanRemoveCommitQueueItem(ctx, mockConnector, basicUser, project, myPatch.Id.Hex())
	require.NoError(t, err)

	// Basic user can remove service user's CLI patch.
	err = CheckCanRemoveCommitQueueItem(ctx, mockConnector, basicUser, project, servicePatch.Id.Hex())
	require.NoError(t, err)

	// Basic user cannot remove other user's CLI patch.
	err = CheckCanRemoveCommitQueueItem(ctx, mockConnector, basicUser, project, otherPatch.Id.Hex())
	require.EqualError(t, err, "401 (Unauthorized): not authorized to perform action on behalf of author")

	// Basic user can remove their own GitHub patch.
	basicUser.Settings.GithubUser.UID = 1234
	err = CheckCanRemoveCommitQueueItem(ctx, mockConnector, basicUser, project, "570")
	require.NoError(t, err)

	// Basic user cannot remove other user's GitHub patch.
	basicUser.Settings.GithubUser.UID = 4321
	err = CheckCanRemoveCommitQueueItem(ctx, mockConnector, basicUser, project, "570")
	require.EqualError(t, err, "401 (Unauthorized): not authorized to perform action on behalf of GitHub user")

	// Project admin can remove other user's CLI patch.
	err = CheckCanRemoveCommitQueueItem(ctx, mockConnector, projectAdmin, project, otherPatch.Id.Hex())
	require.NoError(t, err)

	// Project admin can remove other user's GitHub patch.
	err = CheckCanRemoveCommitQueueItem(ctx, mockConnector, projectAdmin, project, "570")
	require.NoError(t, err)
}
