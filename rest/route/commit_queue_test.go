package route

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/mock"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
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

func (s *CommitQueueSuite) TestGetCommitQueue() {
	route := makeGetCommitQueueItems(s.sc).(*commitQueueGetHandler)
	route.project = "mci"
	pos, err := s.sc.EnqueueItem(
		"mci",
		model.APICommitQueueItem{
			Issue: model.ToAPIString("1"),
			Modules: []model.APIModule{
				model.APIModule{
					Module: model.ToAPIString("test_module"),
					Issue:  model.ToAPIString("1234"),
				},
			},
		})
	s.NoError(err)
	s.Equal(1, pos)

	pos, err = s.sc.EnqueueItem("mci", model.APICommitQueueItem{Issue: model.ToAPIString("2")})
	s.Require().NoError(err)
	s.Require().Equal(2, pos)

	response := route.Run(context.Background())
	s.Equal(200, response.Status())
	s.Equal(&model.APICommitQueue{
		ProjectID: model.ToAPIString("mci"),
		Queue: []model.APICommitQueueItem{
			model.APICommitQueueItem{
				Issue: model.ToAPIString("1"),
				Modules: []model.APIModule{
					model.APIModule{
						Module: model.ToAPIString("test_module"),
						Issue:  model.ToAPIString("1234"),
					},
				},
			},
			model.APICommitQueueItem{
				Issue: model.ToAPIString("2"),
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
	pos, err := s.sc.EnqueueItem("mci", model.APICommitQueueItem{Issue: model.ToAPIString("1")})
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.sc.EnqueueItem("mci", model.APICommitQueueItem{Issue: model.ToAPIString("2")})
	s.Require().NoError(err)
	s.Require().Equal(2, pos)

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
	pos, err := s.sc.EnqueueItem("mci", model.APICommitQueueItem{Issue: model.ToAPIString("12")})
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.sc.EnqueueItem("mci", model.APICommitQueueItem{Issue: model.ToAPIString("23")})
	s.Require().NoError(err)
	s.Require().Equal(2, pos)
	pos, err = s.sc.EnqueueItem("logkeeper", model.APICommitQueueItem{Issue: model.ToAPIString("34")})
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	pos, err = s.sc.EnqueueItem("logkeeper", model.APICommitQueueItem{Issue: model.ToAPIString("45")})
	s.Require().NoError(err)
	s.Require().Equal(2, pos)

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
	id := mgobson.NewObjectId()
	s.sc.CachedPatches = append(s.sc.CachedPatches, patch.Patch{
		Id: id,
	})
	route.item = id.Hex()
	response := route.Run(context.Background())
	s.Equal(200, response.Status())
	s.Equal(model.APICommitQueuePosition{Position: 1}, response.Data())
}

func (s *CommitQueueSuite) TestGetItemAuthor() {
	ctx := context.Background()
	route := makeGetCommitQueueItemAuthor(s.sc).(*getCommitQueueItemAuthorHandler)
	p := patch.Patch{
		Id:     mgobson.NewObjectId(),
		Author: "evergreen.user",
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
	route.item = p.Id.Hex()
	resp := route.Run(ctx)
	s.Equal(200, resp.Status())
	s.Equal(model.APICommitQueueItemAuthor{Author: model.ToAPIString(p.Author)}, resp.Data())

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
	s.Equal(model.APICommitQueueItemAuthor{Author: model.ToAPIString("github.user")}, resp.Data())
}
