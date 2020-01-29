package route

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/mock"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
)

type CommitQueueSuite struct {
	sc *data.MockConnector
	suite.Suite
}

func TestCommitQueueSuite(t *testing.T) {
	suite.Run(t, new(CommitQueueSuite))
}

func (s *CommitQueueSuite) SetupTest() {
	s.sc = &data.MockConnector{
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
			Issue: model.ToStringPtr("1"),
			Modules: []model.APIModule{
				model.APIModule{
					Module: model.ToStringPtr("test_module"),
					Issue:  model.ToStringPtr("1234"),
				},
			},
		}, false)
	s.NoError(err)
	s.Equal(0, pos)

	pos, err = s.sc.EnqueueItem("mci", model.APICommitQueueItem{Issue: model.ToStringPtr("2")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)

	response := route.Run(context.Background())
	s.Equal(200, response.Status())
	s.Equal(&model.APICommitQueue{
		ProjectID: model.ToStringPtr("mci"),
		Queue: []model.APICommitQueueItem{
			model.APICommitQueueItem{
				Issue: model.ToStringPtr("1"),
				Modules: []model.APIModule{
					model.APIModule{
						Module: model.ToStringPtr("test_module"),
						Issue:  model.ToStringPtr("1234"),
					},
				},
			},
			model.APICommitQueueItem{
				Issue: model.ToStringPtr("2"),
			},
		},
	}, response.Data())
}

func (s *CommitQueueSuite) TestDeleteItem() {
	s.sc.MockProjectConnector.CachedProjects = []dbModel.ProjectRef{
		{
			Identifier:  "mci",
			CommitQueue: dbModel.CommitQueueParams{PatchType: commitqueue.PRPatchType},
		},
	}

	ctx := context.Background()
	env := &mock.Environment{}
	s.Require().NoError(env.Configure(ctx))

	route := makeDeleteCommitQueueItems(s.sc, env).(*commitQueueDeleteItemHandler)
	pos, err := s.sc.EnqueueItem("mci", model.APICommitQueueItem{Issue: model.ToStringPtr("1")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = s.sc.EnqueueItem("mci", model.APICommitQueueItem{Issue: model.ToStringPtr("2")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)

	route.project = "mci"

	// Valid delete
	route.item = "1"
	response := route.Run(ctx)
	s.Equal(1, env.LocalQueue().Stats(ctx).Total)
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
	pos, err := s.sc.EnqueueItem("mci", model.APICommitQueueItem{Issue: model.ToStringPtr("12")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = s.sc.EnqueueItem("mci", model.APICommitQueueItem{Issue: model.ToStringPtr("23")}, false)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.sc.EnqueueItem("logkeeper", model.APICommitQueueItem{Issue: model.ToStringPtr("34")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)
	pos, err = s.sc.EnqueueItem("logkeeper", model.APICommitQueueItem{Issue: model.ToStringPtr("45")}, false)
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

func (s *CommitQueueSuite) TestGetItemAuthor() {
	ctx := context.Background()
	route := makeGetCommitQueueItemAuthor(s.sc).(*getCommitQueueItemAuthorHandler)
	id := mgobson.NewObjectId().Hex()
	author := "evergreen.user"
	p := model.APIPatch{
		Id:     &id,
		Author: &author,
	}
	s.sc.CachedPatches = append(s.sc.CachedPatches, p)
	pRef := &dbModel.ProjectRef{
		Identifier: "mci",
		CommitQueue: dbModel.CommitQueueParams{
			PatchType: commitqueue.CLIPatchType,
		},
	}
	s.NoError(s.sc.CreateProject(pRef))

	route.projectID = pRef.Identifier
	route.item = *p.Id
	resp := route.Run(ctx)
	s.Equal(200, resp.Status())
	s.Equal(model.APICommitQueueItemAuthor{Author: p.Author}, resp.Data())

	pRef = &dbModel.ProjectRef{
		Identifier: "not-mci",
		CommitQueue: dbModel.CommitQueueParams{
			PatchType: commitqueue.PRPatchType,
		},
	}
	s.NoError(s.sc.CreateProject(pRef))
	route.projectID = pRef.Identifier
	route.item = "1234"
	resp = route.Run(ctx)
	s.Equal(200, resp.Status())
	s.Equal(model.APICommitQueueItemAuthor{Author: model.ToStringPtr("github.user")}, resp.Data())
}
