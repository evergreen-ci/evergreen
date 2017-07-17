package route

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
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
	sc     *data.MockConnector
	objIds []bson.ObjectId
	data   data.MockPatchConnector

	suite.Suite
}

func TestPatchByIdSuite(t *testing.T) {
	suite.Run(t, new(PatchByIdSuite))
}

func (s *PatchByIdSuite) SetupSuite() {
	s.objIds = []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}

	s.data = data.MockPatchConnector{
		CachedPatches: []patch.Patch{
			{Id: s.objIds[0]},
			{Id: s.objIds[1]},
		},
	}
	s.sc = &data.MockConnector{
		MockPatchConnector: s.data,
	}
}

func (s *PatchByIdSuite) TestFindById() {
	rm := getPatchByIdManager("", 2)
	(rm.Methods[0].RequestHandler).(*patchByIdHandler).patchId = s.objIds[0].Hex()
	res, err := rm.Methods[0].Execute(nil, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Len(res.Result, 1)

	p, ok := (res.Result[0]).(*model.APIPatch)
	s.True(ok)
	s.Equal(model.APIString(s.objIds[0].Hex()), p.Id)
}
func (s *PatchByIdSuite) TestFindByIdFail() {
	rm := getPatchByIdManager("", 2)
	new_id := bson.NewObjectId()
	for _, i := range s.objIds {
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
	s.Equal(model.NewTime(s.now.Add(time.Second*6)), (rd.Result[0]).(*model.APIPatch).CreateTime)
	s.Equal(model.NewTime(s.now.Add(time.Second*4)), (rd.Result[1]).(*model.APIPatch).CreateTime)

	metadata, ok := rd.Metadata.(*PaginationMetadata)
	s.True(ok)
	s.NotNil(metadata)
	pageData := metadata.Pages
	s.NotNil(pageData.Prev)
	s.NotNil(pageData.Next)

	nextTime := s.now.Format(model.APITimeFormat)
	s.Equal(nextTime, pageData.Next.Key)
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
// Tests for abort patch route

type PatchAbortSuite struct {
	sc     *data.MockConnector
	objIds []bson.ObjectId
	data   data.MockPatchConnector

	suite.Suite
}

func TestPatchAbortSuite(t *testing.T) {
	suite.Run(t, new(PatchAbortSuite))
}

func (s *PatchAbortSuite) SetupSuite() {
	s.objIds = []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}

	s.data = data.MockPatchConnector{
		CachedPatches: []patch.Patch{
			{Id: s.objIds[0], Version: "version1"},
			{Id: s.objIds[1]},
		},
		CachedAborted: make(map[string]string),
	}
	s.sc = &data.MockConnector{
		MockPatchConnector: s.data,
	}
}

func (s *PatchAbortSuite) TestAbort() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getPatchAbortManager("", 2)
	(rm.Methods[0].RequestHandler).(*patchAbortHandler).patchId = s.objIds[0].Hex()
	res, err := rm.Methods[0].Execute(ctx, s.sc)

	s.NoError(err)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted[s.objIds[0].Hex()])
	s.Equal("", s.data.CachedAborted[s.objIds[1].Hex()])
	p, ok := (res.Result[0]).(*model.APIPatch)
	s.True(ok)
	s.Equal(model.APIString(s.objIds[0].Hex()), p.Id)

	res, err = rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted[s.objIds[0].Hex()])
	s.Equal("", s.data.CachedAborted[s.objIds[1].Hex()])
	p, ok = (res.Result[0]).(*model.APIPatch)
	s.True(ok)
	s.Equal(model.APIString(s.objIds[0].Hex()), p.Id)

	rm = getPatchAbortManager("", 2)
	(rm.Methods[0].RequestHandler).(*patchAbortHandler).patchId = s.objIds[1].Hex()
	res, err = rm.Methods[0].Execute(ctx, s.sc)

	s.NoError(err)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted[s.objIds[0].Hex()])
	s.Equal("user1", s.data.CachedAborted[s.objIds[1].Hex()])
	s.Len(res.Result, 0)
}

func (s *PatchAbortSuite) TestAbortFail() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getPatchAbortManager("", 2)
	new_id := bson.NewObjectId()
	for _, i := range s.objIds {
		s.NotEqual(new_id, i)
	}
	(rm.Methods[0].RequestHandler).(*patchAbortHandler).patchId = new_id.Hex()
	res, err := rm.Methods[0].Execute(ctx, s.sc)
	s.Error(err)
	s.NotNil(res)
	s.Len(res.Result, 0)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for change patch status route

type PatchesChangeStatusSuite struct {
	sc     *data.MockConnector
	objIds []bson.ObjectId
	data   data.MockPatchConnector

	suite.Suite
}

func TestPatchesChangeStatusSuite(t *testing.T) {
	suite.Run(t, new(PatchesChangeStatusSuite))
}

func (s *PatchesChangeStatusSuite) SetupSuite() {
	s.objIds = []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}

	s.data = data.MockPatchConnector{
		CachedPatches: []patch.Patch{
			{Id: s.objIds[0]},
			{Id: s.objIds[1]},
		},
		CachedAborted:  make(map[string]string),
		CachedPriority: make(map[string]int64),
	}
	s.sc = &data.MockConnector{
		MockPatchConnector: s.data,
	}
}

func (s *PatchesChangeStatusSuite) TestChangeStatus() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getPatchByIdManager("", 2)
	(rm.Methods[1].RequestHandler).(*patchChangeStatusHandler).patchId = s.objIds[0].Hex()
	var tmp_true = true
	(rm.Methods[1].RequestHandler).(*patchChangeStatusHandler).Activated = &tmp_true
	var tmp_seven = int64(7)
	(rm.Methods[1].RequestHandler).(*patchChangeStatusHandler).Priority = &tmp_seven
	res, err := rm.Methods[1].Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(int64(7), s.data.CachedPriority[s.objIds[0].Hex()])
	s.Equal(int64(0), s.data.CachedPriority[s.objIds[1].Hex()])
	s.Len(res.Result, 1)
	p, ok := (res.Result[0]).(*model.APIPatch)
	s.True(ok)
	s.True(p.Activated)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for restart patch by id route

type PatchRestartSuite struct {
	sc          *data.MockConnector
	objIds      []bson.ObjectId
	patchData   data.MockPatchConnector
	versionData data.MockVersionConnector

	suite.Suite
}

func TestPatchRestartSuite(t *testing.T) {
	suite.Run(t, new(PatchRestartSuite))
}

func (s *PatchRestartSuite) SetupSuite() {
	s.objIds = []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}

	s.patchData = data.MockPatchConnector{
		CachedPatches: []patch.Patch{
			{Id: s.objIds[0], Version: "version1"},
			{Id: s.objIds[1]},
		},
		CachedAborted: make(map[string]string),
	}
	s.versionData = data.MockVersionConnector{
		CachedVersions: []version.Version{
			{Id: s.objIds[0].Hex()},
			{Id: s.objIds[1].Hex()},
		},
		CachedRestartedVersions: make(map[string]string),
	}
	s.sc = &data.MockConnector{
		MockPatchConnector:   s.patchData,
		MockVersionConnector: s.versionData,
	}
}

func (s *PatchRestartSuite) TestRestart() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getPatchRestartManager("", 2)
	(rm.Methods[0].RequestHandler).(*patchRestartHandler).patchId = s.objIds[0].Hex()
	res, err := rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(res)

	s.Equal("user1", s.sc.CachedRestartedVersions[s.objIds[0].Hex()])
}
