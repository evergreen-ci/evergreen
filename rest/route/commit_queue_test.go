package route

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CommitQueueSuite struct {
	suite.Suite
}

func TestCommitQueueSuite(t *testing.T) {
	suite.Run(t, new(CommitQueueSuite))
}

func (s *CommitQueueSuite) SetupTest() {
	s.NoError(db.ClearCollections(dbModel.ProjectRefCollection, patch.Collection, commitqueue.Collection))
	projRef := dbModel.ProjectRef{
		Id: "proj",
	}
	s.Require().NoError(projRef.Insert())
	projRef2 := dbModel.ProjectRef{
		Id: "mci",
	}
	cq := &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue:     []commitqueue.CommitQueueItem{},
	}
	s.Require().NoError(commitqueue.InsertQueue(cq))
	s.NoError(projRef2.Insert())
}

func (s *CommitQueueSuite) TestParse() {
	ctx := context.Background()
	route := makeCommitQueueEnqueueItem().(*commitQueueEnqueueItemHandler)
	patchID := "aabbccddeeff001122334455"
	projectID := "proj"
	p1 := patch.Patch{Id: patch.NewId(patchID), Project: projectID}
	s.NoError(p1.Insert())
	req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("http://example.com/api/rest/v2/commit_queue/%s?force=true", patchID), nil)
	req = gimlet.SetURLVars(req, map[string]string{"patch_id": patchID})
	s.NoError(route.Parse(ctx, req))
	s.True(route.force)
}

func (s *CommitQueueSuite) TestGetCommitQueue() {
	route := makeGetCommitQueueItems().(*commitQueueGetHandler)
	route.project = "mci"
	pos, err := data.EnqueueItem(
		"mci",
		model.APICommitQueueItem{
			Issue:  utility.ToStringPtr("1"),
			Source: utility.ToStringPtr(commitqueue.SourceDiff),
			Modules: []model.APIModule{
				{
					Module: utility.ToStringPtr("test_module"),
					Issue:  utility.ToStringPtr("1234"),
				},
			},
		}, false)
	s.NoError(err)
	s.Equal(0, pos)

	pos, err = data.EnqueueItem("mci", model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("2")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)

	response := route.Run(context.Background())
	s.Equal(200, response.Status())
	cqResp := response.Data().(*model.APICommitQueue)
	list := cqResp.Queue
	s.Len(list, 2)
	s.Equal(list[0].Issue, utility.ToStringPtr("1"))
	s.Equal(list[1].Issue, utility.ToStringPtr("2"))
}

func (s *CommitQueueSuite) TestDeleteItem() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, s.T())
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	route := makeDeleteCommitQueueItems(env).(*commitQueueDeleteItemHandler)
	p1 := patch.Patch{
		Id: mgobson.NewObjectId(),
	}
	s.Require().NoError(p1.Insert())
	pos1, err := data.EnqueueItem("mci", model.APICommitQueueItem{
		Source:  utility.ToStringPtr(commitqueue.SourceDiff),
		PatchId: utility.ToStringPtr(p1.Id.Hex()),
		Issue:   utility.ToStringPtr("1"),
	}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos1)

	p2 := patch.Patch{
		Id: mgobson.NewObjectId(),
	}
	s.Require().NoError(p2.Insert())
	pos2, err := data.EnqueueItem("mci", model.APICommitQueueItem{
		Source:  utility.ToStringPtr(commitqueue.SourceDiff),
		PatchId: utility.ToStringPtr(p2.Id.Hex()),
		Issue:   utility.ToStringPtr("2"),
	}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos2)

	route.project = "mci"

	// Valid delete
	route.item = "1"
	response := route.Run(ctx)
	s.Equal(204, response.Status())

	// Already deleted
	response = route.Run(ctx)
	s.Equal(404, response.Status())

	// Invalid project
	route.project = "not_here"
	route.item = "2"
	response = route.Run(ctx)
	s.Equal(500, response.Status())
}

func (s *CommitQueueSuite) TestClearAll() {
	s.NoError(db.ClearCollections(commitqueue.Collection))
	cq0 := &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue:     []commitqueue.CommitQueueItem{},
	}
	cq1 := &commitqueue.CommitQueue{
		ProjectID: "logkeeper",
		Queue:     []commitqueue.CommitQueueItem{},
	}
	s.Require().NoError(commitqueue.InsertQueue(cq0))
	s.Require().NoError(commitqueue.InsertQueue(cq1))

	route := makeClearCommitQueuesHandler().(*commitQueueClearAllHandler)
	pos, err := data.EnqueueItem("mci", model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("12")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = data.EnqueueItem("mci", model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("23")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = data.EnqueueItem("logkeeper", model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("34")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = data.EnqueueItem("logkeeper", model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("45")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)

	response := route.Run(context.Background())
	s.Equal(200, response.Status())
	s.Equal(struct {
		ClearedCount int `json:"cleared_count"`
	}{2}, response.Data())

	response = route.Run(context.Background())
	s.Equal(200, response.Status())
	s.Equal(struct {
		ClearedCount int `json:"cleared_count"`
	}{0}, response.Data())
}

func (s *CommitQueueSuite) TestEnqueueItem() {
	route := makeCommitQueueEnqueueItem().(*commitQueueEnqueueItemHandler)
	id := "aabbccddeeff112233445566"
	patch1 := patch.Patch{
		Id: patch.NewId(id),
		Patches: []patch.ModulePatch{
			{
				ModuleName: "",
				PatchSet:   patch.PatchSet{CommitMessages: []string{"Commit"}},
			},
		},
	}
	s.NoError(patch1.Insert())
	route.item = id
	route.project = "mci"
	response := route.Run(context.Background())
	s.Equal(200, response.Status())
	s.Equal(model.APICommitQueuePosition{Position: 0}, response.Data())
}

func TestCqMessageForPatch(t *testing.T) {
	assert.NoError(t, db.ClearCollections(dbModel.ProjectRefCollection, patch.Collection))
	project := dbModel.ProjectRef{
		Id:          "mci",
		CommitQueue: dbModel.CommitQueueParams{Message: "you found me!"},
	}
	assert.NoError(t, project.Insert())
	p := patch.Patch{
		Id:      mgobson.NewObjectId(),
		Project: "mci",
	}
	assert.NoError(t, p.Insert())
	handler := makecqMessageForPatch()
	ctx := context.Background()

	request, err := http.NewRequest(http.MethodGet, "", nil)
	assert.NoError(t, err)
	request = gimlet.SetURLVars(request, map[string]string{"patch_id": p.Id.Hex()})
	assert.NoError(t, handler.Parse(ctx, request))
	resp := handler.Run(ctx)
	assert.Equal(t, project.CommitQueue.Message, resp.Data().(string))
}

func TestAdditionalPatches(t *testing.T) {
	assert.NoError(t, db.ClearCollections(commitqueue.Collection, patch.Collection))
	patchId := bson.NewObjectId()
	p := patch.Patch{
		Id:      patchId,
		Project: "proj",
	}
	assert.NoError(t, p.Insert())
	cq := commitqueue.CommitQueue{
		ProjectID: "proj",
		Queue: []commitqueue.CommitQueueItem{
			{Version: "a"},
			{Version: "b"},
			{Version: patchId.Hex()},
		},
	}
	assert.NoError(t, commitqueue.InsertQueue(&cq))

	handler := makeCommitQueueAdditionalPatches()
	ctx := context.Background()

	request, err := http.NewRequest(http.MethodGet, "", nil)
	assert.NoError(t, err)
	request = gimlet.SetURLVars(request, map[string]string{"patch_id": patchId.Hex()})
	assert.NoError(t, handler.Parse(ctx, request))
	resp := handler.Run(ctx)
	assert.Equal(t, []string{"a", "b"}, resp.Data().([]string))
}
