package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/testutil"

	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by id route

type PatchByIdSuite struct {
	sc     *data.MockConnector
	objIds []string
	data   data.MockPatchConnector
	route  *patchByIdHandler
	suite.Suite
}

func TestPatchByIdSuite(t *testing.T) {
	suite.Run(t, new(PatchByIdSuite))
}

func (s *PatchByIdSuite) SetupSuite() {
	s.objIds = []string{mgobson.NewObjectId().Hex(), mgobson.NewObjectId().Hex()}

	s.data = data.MockPatchConnector{
		CachedPatches: []model.APIPatch{
			{Id: &s.objIds[0]},
			{Id: &s.objIds[1]},
		},
	}
	s.sc = &data.MockConnector{
		URL:                "https://evergreen.example.net",
		MockPatchConnector: s.data,
	}
}

func (s *PatchByIdSuite) SetupTest() {
	s.route = makeFetchPatchByID(s.sc).(*patchByIdHandler)
}

func (s *PatchByIdSuite) TestFindById() {
	s.route.patchId = s.objIds[0]
	res := s.route.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status(), "%+v", res.Data())

	p, ok := res.Data().(*model.APIPatch)
	s.True(ok)
	s.Equal(model.ToStringPtr(s.objIds[0]), p.Id)
}
func (s *PatchByIdSuite) TestFindByIdFail() {
	new_id := mgobson.NewObjectId()
	for _, i := range s.objIds {
		s.NotEqual(new_id, i)
	}

	s.route.patchId = new_id.Hex()
	res := s.route.Run(context.TODO())
	s.Equal(http.StatusNotFound, res.Status())
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by project route

type PatchesByProjectSuite struct {
	sc    *data.MockConnector
	data  data.MockPatchConnector
	now   time.Time
	route *patchesByProjectHandler
	suite.Suite
}

func TestPatchesByProjectSuite(t *testing.T) {
	suite.Run(t, new(PatchesByProjectSuite))
}

func (s *PatchesByProjectSuite) SetupSuite() {
	s.now = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.FixedZone("", 0))
	proj1 := "project1"
	proj2 := "project2"
	nowPlus2 := s.now.Add(time.Second * 2)
	nowPlus4 := s.now.Add(time.Second * 4)
	nowPlus6 := s.now.Add(time.Second * 6)
	nowPlus8 := s.now.Add(time.Second * 8)
	nowPlus10 := s.now.Add(time.Second * 10)
	s.data = data.MockPatchConnector{
		CachedPatches: []model.APIPatch{
			{ProjectId: &proj1, CreateTime: &s.now},
			{ProjectId: &proj2, CreateTime: &nowPlus2},
			{ProjectId: &proj1, CreateTime: &nowPlus4},
			{ProjectId: &proj1, CreateTime: &nowPlus6},
			{ProjectId: &proj2, CreateTime: &nowPlus8},
			{ProjectId: &proj1, CreateTime: &nowPlus10},
		},
	}
	s.sc = &data.MockConnector{
		URL:                "https://evergreen.example.net",
		MockPatchConnector: s.data,
	}
}

func (s *PatchesByProjectSuite) SetupTest() {
	s.route = makePatchesByProjectRoute(s.sc).(*patchesByProjectHandler)
}

func (s *PatchesByProjectSuite) TestPaginatorShouldErrorIfNoResults() {
	s.route.projectId = "project3"
	s.route.key = s.now
	s.route.limit = 1

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusNotFound, resp.Status())
	s.Contains(resp.Data().(gimlet.ErrorResponse).Message, "no patches found")
}

func (s *PatchesByProjectSuite) TestPaginatorShouldReturnResultsIfDataExists() {
	s.route.projectId = "project1"
	s.route.key = s.now.Add(time.Second * 7)
	s.route.limit = 2

	resp := s.route.Run(context.Background())
	s.NotNil(resp)

	payload := resp.Data().([]interface{})
	s.NotNil(payload)

	s.Len(payload, 2)
	s.Equal(s.now.Add(time.Second*6), *(payload[0]).(model.APIPatch).CreateTime)
	s.Equal(s.now.Add(time.Second*4), *(payload[1]).(model.APIPatch).CreateTime)

	pages := resp.Pages()
	s.NotNil(pages)
	s.Nil(pages.Prev)
	s.NotNil(pages.Next)

	nextTime := s.now.Format(model.APITimeFormat)
	s.Equal(nextTime, pages.Next.Key)
}

func (s *PatchesByProjectSuite) TestPaginatorShouldReturnEmptyResultsIfDataIsEmpty() {
	s.route.projectId = "project2"
	s.route.key = s.now.Add(time.Hour)
	s.route.limit = 100

	resp := s.route.Run(context.Background())
	s.Len(resp.Data().([]interface{}), 2)

	s.Nil(resp.Pages())
}

func (s *PatchesByProjectSuite) TestInvalidTimesAsKeyShouldError() {
	inputs := []string{
		"200096-01-02T15:04:05.000Z",
		"15:04:05.000Z",
		"invalid",
	}

	for _, i := range inputs {
		for limit := 0; limit < 3; limit++ {
			req, err := http.NewRequest("GET", "https://example.net/foo/?limit=10&start_at="+i, nil)
			s.Require().NoError(err)
			err = s.route.Parse(context.Background(), req)
			s.Error(err)
		}
	}
}

func (s *PatchesByProjectSuite) TestEmptyTimeShouldSetNow() {
	req, err := http.NewRequest("GET", "https://example.net/foo/?limit=10", nil)
	s.Require().NoError(err)
	s.NoError(s.route.Parse(context.Background(), req))
	s.InDelta(time.Now().UnixNano(), s.route.key.UnixNano(), float64(time.Second))
}

////////////////////////////////////////////////////////////////////////
//
// Tests for abort patch route

type PatchAbortSuite struct {
	sc     *data.MockConnector
	objIds []string
	data   data.MockPatchConnector

	suite.Suite
}

func TestPatchAbortSuite(t *testing.T) {
	suite.Run(t, new(PatchAbortSuite))
}

func (s *PatchAbortSuite) SetupSuite() {
	s.objIds = []string{mgobson.NewObjectId().Hex(), mgobson.NewObjectId().Hex()}
	version1 := "version1"

	s.data = data.MockPatchConnector{
		CachedPatches: []model.APIPatch{
			{Id: &s.objIds[0], Version: &version1},
			{Id: &s.objIds[1]},
		},
		CachedAborted: make(map[string]string),
	}
	s.sc = &data.MockConnector{
		MockPatchConnector: s.data,
	}
}

func (s *PatchAbortSuite) TestAbort() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	rm := makeAbortPatch(s.sc).(*patchAbortHandler)
	rm.patchId = s.objIds[0]
	res := rm.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	s.Equal("user1", s.data.CachedAborted[s.objIds[0]])
	s.Equal("", s.data.CachedAborted[s.objIds[1]])
	p, ok := (res.Data()).(*model.APIPatch)
	s.True(ok)
	s.Equal(model.ToStringPtr(s.objIds[0]), p.Id)

	res = rm.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted[s.objIds[0]])
	s.Equal("", s.data.CachedAborted[s.objIds[1]])
	p, ok = (res.Data()).(*model.APIPatch)
	s.True(ok)
	s.Equal(model.ToStringPtr(s.objIds[0]), p.Id)

	rm.patchId = s.objIds[1]
	res = rm.Run(ctx)

	s.NotEqual(http.StatusOK, res.Status())
}

func (s *PatchAbortSuite) TestAbortFail() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	rm := makeAbortPatch(s.sc).(*patchAbortHandler)
	new_id := mgobson.NewObjectId()
	for _, i := range s.objIds {
		s.NotEqual(new_id, i)
	}

	rm.patchId = new_id.Hex()
	res := rm.Run(ctx)
	s.NotEqual(http.StatusOK, res.Status())
}

////////////////////////////////////////////////////////////////////////
//
// Tests for change patch status route

type PatchesChangeStatusSuite struct {
	sc     *data.MockConnector
	objIds []string
	data   data.MockPatchConnector

	suite.Suite
}

func TestPatchesChangeStatusSuite(t *testing.T) {
	suite.Run(t, new(PatchesChangeStatusSuite))
}

func (s *PatchesChangeStatusSuite) SetupSuite() {
	s.objIds = []string{mgobson.NewObjectId().Hex(), mgobson.NewObjectId().Hex()}

	s.data = data.MockPatchConnector{
		CachedPatches: []model.APIPatch{
			{Id: &s.objIds[0]},
			{Id: &s.objIds[1]},
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
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})
	env := testutil.NewEnvironment(ctx, s.T())
	rm := makeChangePatchStatus(s.sc, env).(*patchChangeStatusHandler)

	rm.patchId = s.objIds[0]
	var tmp_true = true
	rm.Activated = &tmp_true
	var tmp_seven = int64(7)
	rm.Priority = &tmp_seven
	res := rm.Run(ctx)
	s.NotNil(res)
	s.Equal(int64(7), s.data.CachedPriority[s.objIds[0]])
	s.Equal(int64(0), s.data.CachedPriority[s.objIds[1]])
	p, ok := res.Data().(*model.APIPatch)
	s.True(ok)
	s.True(p.Activated)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for restart patch by id route

type PatchRestartSuite struct {
	sc          *data.MockConnector
	objIds      []string
	patchData   data.MockPatchConnector
	versionData data.MockVersionConnector

	suite.Suite
}

func TestPatchRestartSuite(t *testing.T) {
	suite.Run(t, new(PatchRestartSuite))
}

func (s *PatchRestartSuite) SetupSuite() {
	s.objIds = []string{mgobson.NewObjectId().Hex(), mgobson.NewObjectId().Hex()}
	version1 := "version1"

	s.patchData = data.MockPatchConnector{
		CachedPatches: []model.APIPatch{
			{Id: &s.objIds[0], Version: &version1},
			{Id: &s.objIds[1]},
		},
		CachedAborted: make(map[string]string),
	}
	s.versionData = data.MockVersionConnector{
		CachedVersions: []serviceModel.Version{
			{Id: s.objIds[0]},
			{Id: s.objIds[1]},
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
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	rm := makeRestartPatch(s.sc).(*patchRestartHandler)
	rm.patchId = s.objIds[0]
	res := rm.Run(ctx)
	s.NotNil(res)

	s.Equal(http.StatusOK, res.Status())
	s.Equal("user1", s.sc.CachedRestartedVersions[s.objIds[0]])
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patches for current user

type PatchesByUserSuite struct {
	sc    *data.MockConnector
	data  data.MockPatchConnector
	now   time.Time
	route *patchesByUserHandler

	suite.Suite
}

func TestPatchesByUserSuite(t *testing.T) {
	s := new(PatchesByUserSuite)
	s.now = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.FixedZone("", 0))
	user1 := "user1"
	user2 := "user2"
	nowPlus2 := s.now.Add(time.Second * 2)
	nowPlus4 := s.now.Add(time.Second * 4)
	nowPlus6 := s.now.Add(time.Second * 6)
	nowPlus8 := s.now.Add(time.Second * 8)
	nowPlus10 := s.now.Add(time.Second * 10)
	s.data = data.MockPatchConnector{
		CachedPatches: []model.APIPatch{
			{Author: &user1, CreateTime: &s.now},
			{Author: &user2, CreateTime: &nowPlus2},
			{Author: &user1, CreateTime: &nowPlus4},
			{Author: &user1, CreateTime: &nowPlus6},
			{Author: &user2, CreateTime: &nowPlus8},
			{Author: &user1, CreateTime: &nowPlus10},
		},
	}
	s.sc = &data.MockConnector{
		URL:                "https://evergreen.example.net",
		MockPatchConnector: s.data,
	}

	suite.Run(t, s)
}

func (s *PatchesByUserSuite) SetupTest() {
	s.route = &patchesByUserHandler{
		sc: s.sc,
	}
}

func (s *PatchesByUserSuite) TestPaginatorShouldErrorIfNoResults() {
	s.route.user = "zzz"
	s.route.key = s.now
	s.route.limit = 1

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusNotFound, resp.Status())
	s.Contains(resp.Data().(gimlet.ErrorResponse).Message, "no patches found")
}

func (s *PatchesByUserSuite) TestPaginatorShouldReturnResultsIfDataExists() {
	s.route.user = "user1"
	s.route.key = s.now.Add(time.Second * 7)
	s.route.limit = 2

	resp := s.route.Run(context.Background())
	s.Equal(http.StatusOK, resp.Status())
	payload := resp.Data().([]interface{})

	s.Len(payload, 2)
	s.Equal(s.now.Add(time.Second*6), *(payload[0]).(model.APIPatch).CreateTime)
	s.Equal(s.now.Add(time.Second*4), *(payload[1]).(model.APIPatch).CreateTime)

	pageData := resp.Pages()

	s.Nil(pageData.Prev)
	s.NotNil(pageData.Next)

	nextTime := s.now.Format(model.APITimeFormat)
	s.Equal(nextTime, pageData.Next.Key)
}

func (s *PatchesByUserSuite) TestPaginatorShouldReturnEmptyResultsIfDataIsEmpty() {
	s.route.user = "user2"
	s.route.key = s.now.Add(time.Hour)
	s.route.limit = 100

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	s.Len(resp.Data().([]interface{}), 2)

	s.Nil(resp.Pages())
}

func (s *PatchesByUserSuite) TestInvalidTimesAsKeyShouldError() {
	inputs := []string{
		"200096-01-02T15:04:05.000Z",
		"15:04:05.000Z",
		"invalid",
	}

	for _, i := range inputs {
		for limit := 0; limit < 3; limit++ {
			req, err := http.NewRequest("GET", "https://example.net/foo/?limit=10&start_at="+i, nil)
			s.Require().NoError(err)
			err = s.route.Parse(context.Background(), req)
			s.Error(err)
			apiErr, ok := err.(gimlet.ErrorResponse)
			s.True(ok)
			s.Contains(apiErr.Message, i)
		}
	}
}

func (s *PatchesByUserSuite) TestEmptyTimeShouldSetNow() {
	req, err := http.NewRequest("GET", "https://example.net/foo/?limit=10", nil)
	s.Require().NoError(err)
	s.NoError(s.route.Parse(context.Background(), req))
	s.InDelta(time.Now().UnixNano(), s.route.key.UnixNano(), float64(time.Second))
}
