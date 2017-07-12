package route

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
	"gopkg.in/mgo.v2/bson"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by id route

type PatchByIdSuite struct {
	sc      *data.MockConnector
	obj_ids []bson.ObjectId
	data    data.MockPatchConnector

	suite.Suite
}

func TestPatchByIdSuite(t *testing.T) {
	suite.Run(t, new(PatchByIdSuite))
}

func (s *PatchByIdSuite) SetupSuite() {
	s.obj_ids = []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}

	s.data = data.MockPatchConnector{
		CachedPatches: []patch.Patch{
			{Id: s.obj_ids[0]},
			{Id: s.obj_ids[1]},
		},
	}
	s.sc = &data.MockConnector{
		MockPatchConnector: s.data,
	}
}

func (s *PatchByIdSuite) TestFindById() {
	rm := getPatchByIdManager("", 2)
	(rm.Methods[0].RequestHandler).(*patchByIdHandler).patchId = s.obj_ids[0].Hex()
	res, err := rm.Methods[0].Execute(nil, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Len(res.Result, 1)

	p, ok := (res.Result[0]).(*model.APIPatch)
	s.True(ok)
	s.Equal(model.APIString(s.obj_ids[0].Hex()), p.Id)
}
func (s *PatchByIdSuite) TestFindByIdFail() {
	rm := getPatchByIdManager("", 2)
	new_id := bson.NewObjectId()
	for _, i := range s.obj_ids {
		s.NotEqual(new_id, i)
	}
	(rm.Methods[0].RequestHandler).(*patchByIdHandler).patchId = new_id.Hex()
	res, err := rm.Methods[0].Execute(nil, s.sc)
	s.Error(err)
	s.Len(res.Result, 0)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by project route

type PatchesByProjectSuite struct {
	sc        *data.MockConnector
	data      data.MockPatchConnector
	now       time.Time
	paginator PaginatorFunc

	suite.Suite
}

func TestPatchesByProjectSuite(t *testing.T) {
	suite.Run(t, new(PatchesByProjectSuite))
}

func (s *PatchesByProjectSuite) SetupSuite() {
	s.now = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.FixedZone("", 0))
	s.data = data.MockPatchConnector{
		CachedPatches: []patch.Patch{
			{Project: "project1", CreateTime: s.now},
			{Project: "project2", CreateTime: s.now.Add(time.Second * 2)},
			{Project: "project1", CreateTime: s.now.Add(time.Second * 4)},
			{Project: "project1", CreateTime: s.now.Add(time.Second * 6)},
			{Project: "project2", CreateTime: s.now.Add(time.Second * 8)},
			{Project: "project1", CreateTime: s.now.Add(time.Second * 10)},
		},
	}
	s.paginator = patchesByProjectPaginator
	s.sc = &data.MockConnector{
		MockPatchConnector: s.data,
	}
}

func (s *PatchesByProjectSuite) TestPaginatorShouldErrorIfNoResults() {
	rd, err := executePatchesByProjectRequest("project3", s.now, 1, s.sc)
	s.Error(err)
	s.NotNil(rd)
	s.Len(rd.Result, 0)
	s.Contains(err.Error(), "no patches found")
}

func (s *PatchesByProjectSuite) TestPaginatorShouldReturnResultsIfDataExists() {
	rd, err := executePatchesByProjectRequest("project1", s.now.Add(time.Second*7), 2, s.sc)
	s.NoError(err)
	s.NotNil(rd)
	s.Len(rd.Result, 2)
	s.Equal(model.APITime(s.now.Add(time.Second*6)), (rd.Result[0]).(*model.APIPatch).CreateTime)
	s.Equal(model.APITime(s.now.Add(time.Second*4)), (rd.Result[1]).(*model.APIPatch).CreateTime)

	metadata, ok := rd.Metadata.(*PaginationMetadata)
	s.True(ok)
	s.NotNil(metadata)
	pageData := metadata.Pages
	s.NotNil(pageData.Prev)
	s.NotNil(pageData.Next)

	//nextTime := model.NewTime(s.now).String()
	nextTime := s.now.Format(model.APITimeFormat)
	s.Equal(nextTime, pageData.Next.Key)
	//prevTime := model.NewTime(s.now.Add(time.Second * 10)).String()
	prevTime := s.now.Add(time.Second * 10).Format(model.APITimeFormat)
	s.Equal(prevTime, pageData.Prev.Key)
}

func (s *PatchesByProjectSuite) TestPaginatorShouldReturnEmptyResultsIfDataIsEmpty() {
	rd, err := executePatchesByProjectRequest("project2", s.now.Add(time.Hour), 100, s.sc)
	s.NoError(err)
	s.NotNil(rd)
	s.Len(rd.Result, 2)
	metadata, ok := rd.Metadata.(*PaginationMetadata)
	s.True(ok)
	s.NotNil(metadata)
	pageData := metadata.Pages
	s.Nil(pageData.Prev)
	s.Nil(pageData.Next)
}

func (s *PatchesByProjectSuite) TestInvalidTimesAsKeyShouldError() {
	inputs := []string{
		"200096-01-02T15:04:05.000Z",
		"15:04:05.000Z",
		"invalid",
	}

	for _, i := range inputs {
		for limit := 0; limit < 3; limit++ {
			a, b, err := s.paginator(i, limit, patchesByProjectArgs{}, s.sc)
			s.Len(a, 0)
			s.Nil(b)
			s.Error(err)
			apiErr, ok := err.(rest.APIError)
			s.True(ok)
			s.Contains(apiErr.Message, i)
		}
	}
}

func executePatchesByProjectRequest(projectId string, ts time.Time, limit int, sc *data.MockConnector) (ResponseData, error) {
	rm := getPatchesByProjectManager("", 2)
	pe := (rm.Methods[0].RequestHandler).(*patchesByProjectHandler)
	pe.Args = patchesByProjectArgs{projectId: projectId}
	pe.key = ts.Format(model.APITimeFormat)
	pe.limit = limit

	return pe.Execute(nil, sc)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for abort patch by id route

type PatchesAbortByIdSuite struct {
	sc      *data.MockConnector
	obj_ids []bson.ObjectId
	data    data.MockPatchConnector

	suite.Suite
}

func TestPatchesAbortByIdSuite(t *testing.T) {
	suite.Run(t, new(PatchesAbortByIdSuite))
}

func (s *PatchesAbortByIdSuite) SetupSuite() {
	s.obj_ids = []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}

	s.data = data.MockPatchConnector{
		CachedPatches: []patch.Patch{
			{Id: s.obj_ids[0], Version: "version1"},
			{Id: s.obj_ids[1]},
		},
		CachedAborted: make(map[string]string),
	}
	s.sc = &data.MockConnector{
		MockPatchConnector: s.data,
	}
}

func (s *PatchesAbortByIdSuite) TestAbort() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getPatchAbortManager("", 2)
	(rm.Methods[0].RequestHandler).(*patchAbortHandler).patchId = s.obj_ids[0].Hex()
	res, err := rm.Methods[0].Execute(ctx, s.sc)

	s.NoError(err)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted[s.obj_ids[0].Hex()])
	s.Equal("", s.data.CachedAborted[s.obj_ids[1].Hex()])
	p, ok := (res.Result[0]).(*model.APIPatch)
	s.True(ok)
	s.Equal(model.APIString(s.obj_ids[0].Hex()), p.Id)

	res, err = rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted[s.obj_ids[0].Hex()])
	s.Equal("", s.data.CachedAborted[s.obj_ids[1].Hex()])
	p, ok = (res.Result[0]).(*model.APIPatch)
	s.True(ok)
	s.Equal(model.APIString(s.obj_ids[0].Hex()), p.Id)

	rm = getPatchAbortManager("", 2)
	(rm.Methods[0].RequestHandler).(*patchAbortHandler).patchId = s.obj_ids[1].Hex()
	res, err = rm.Methods[0].Execute(ctx, s.sc)

	s.NoError(err)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted[s.obj_ids[0].Hex()])
	s.Equal("user1", s.data.CachedAborted[s.obj_ids[1].Hex()])
	s.Len(res.Result, 0)
}

func (s *PatchesAbortByIdSuite) TestAbortFail() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getPatchAbortManager("", 2)
	new_id := bson.NewObjectId()
	for _, i := range s.obj_ids {
		s.NotEqual(new_id, i)
	}
	(rm.Methods[0].RequestHandler).(*patchAbortHandler).patchId = new_id.Hex()
	res, err := rm.Methods[0].Execute(ctx, s.sc)
	s.Error(err)
	s.NotNil(res)
	s.Len(res.Result, 0)
}
