package route

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CommitQueueSuite struct {
	sc *data.DBConnector
	suite.Suite
}

func TestCommitQueueSuite(t *testing.T) {
	suite.Run(t, new(CommitQueueSuite))
}

func (s *CommitQueueSuite) SetupTest() {
	s.sc = &data.DBConnector{
		MockCommitQueueConnector: data.MockCommitQueueConnector{},
	}
}

func (s *CommitQueueSuite) TestParse() {
	ctx := context.Background()
	route := makeCommitQueueEnqueueItem(s.sc).(*commitQueueEnqueueItemHandler)

	patchID := mgobson.NewObjectId().Hex()
	projectID := "proj"
	s.sc.CachedPatches = append(s.sc.CachedPatches, model.APIPatch{Id: &patchID, ProjectId: &projectID})
	req, _ := http.NewRequest("PUT", fmt.Sprintf("http://example.com/api/rest/v2/commit_queue/%s?force=true", patchID), nil)
	req = gimlet.SetURLVars(req, map[string]string{"patch_id": patchID})
	s.NoError(route.Parse(ctx, req))
	s.True(route.force)
}

func (s *CommitQueueSuite) TestGetCommitQueue() {
	route := makeGetCommitQueueItems(s.sc).(*commitQueueGetHandler)
	route.project = "mci"
	pos, err := s.sc.EnqueueItem(
		"mci",
		model.APICommitQueueItem{
			Issue:  utility.ToStringPtr("1"),
			Source: utility.ToStringPtr(commitqueue.SourceDiff),
			Modules: []model.APIModule{
				model.APIModule{
					Module: utility.ToStringPtr("test_module"),
					Issue:  utility.ToStringPtr("1234"),
				},
			},
		}, false)
	s.NoError(err)
	s.Equal(0, pos)

	pos, err = s.sc.EnqueueItem("mci", model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("2")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)

	response := route.Run(context.Background())
	s.Equal(200, response.Status())
	s.Equal(&model.APICommitQueue{
		ProjectID: utility.ToStringPtr("mci"),
		Queue: []model.APICommitQueueItem{
			model.APICommitQueueItem{
				Source: utility.ToStringPtr(commitqueue.SourceDiff),
				Issue:  utility.ToStringPtr("1"),
				Modules: []model.APIModule{
					model.APIModule{
						Module: utility.ToStringPtr("test_module"),
						Issue:  utility.ToStringPtr("1234"),
					},
				},
			},
			model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff),
				Issue: utility.ToStringPtr("2"),
			},
		},
	}, response.Data())
}

func (s *CommitQueueSuite) TestDeleteItem() {
	projRef := dbModel.ProjectRef{
		Id: "mci",
	}
	s.NoError(projRef.Insert())
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})
	env := &mock.Environment{}
	s.Require().NoError(env.Configure(ctx))

	route := makeDeleteCommitQueueItems(s.sc, env).(*commitQueueDeleteItemHandler)
	pos, err := s.sc.EnqueueItem("mci", model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("1")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = s.sc.EnqueueItem("mci", model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("2")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)

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
	s.Equal(404, response.Status())
}

func (s *CommitQueueSuite) TestClearAll() {
	route := makeClearCommitQueuesHandler(s.sc).(*commitQueueClearAllHandler)
	pos, err := s.sc.EnqueueItem("mci", model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("12")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = s.sc.EnqueueItem("mci", model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("23")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.sc.EnqueueItem("logkeeper", model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("34")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = s.sc.EnqueueItem("logkeeper", model.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("45")}, false)
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
	route := makeCommitQueueEnqueueItem(s.sc).(*commitQueueEnqueueItemHandler)
	id := mgobson.NewObjectId().Hex()
	s.sc.CachedPatches = append(s.sc.CachedPatches, model.APIPatch{
		Id: &id,
	})
	route.item = id
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
	handler := makecqMessageForPatch(&data.DBConnector{})
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

	handler := makeCommitQueueAdditionalPatches(&data.DBConnector{})
	ctx := context.Background()

	request, err := http.NewRequest(http.MethodGet, "", nil)
	assert.NoError(t, err)
	request = gimlet.SetURLVars(request, map[string]string{"patch_id": patchId.Hex()})
	assert.NoError(t, handler.Parse(ctx, request))
	resp := handler.Run(ctx)
	assert.Equal(t, []string{"a", "b"}, resp.Data().([]string))
}
