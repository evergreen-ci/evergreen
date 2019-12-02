package route

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch build by id

type BuildByIdSuite struct {
	sc   *data.MockConnector
	data data.MockBuildConnector
	rm   gimlet.RouteHandler
	suite.Suite
}

func TestBuildSuite(t *testing.T) {
	suite.Run(t, new(BuildByIdSuite))
}

func (s *BuildByIdSuite) SetupSuite() {
	s.data = data.MockBuildConnector{
		CachedBuilds: []build.Build{
			{Id: "build1", Project: "branch"},
			{Id: "build2", Project: "notbranch"},
		},
		CachedProjects: map[string]*serviceModel.ProjectRef{
			"branch": {Repo: "project", Identifier: "branch"},
		},
		CachedAborted: make(map[string]string),
	}
	s.sc = &data.MockConnector{
		MockBuildConnector: s.data,
	}
}

func (s *BuildByIdSuite) SetupTest() {
	s.rm = makeGetBuildByID(s.sc)
}

func (s *BuildByIdSuite) TestFindByIdProjFound() {
	s.rm.(*buildGetHandler).buildId = "build1"
	resp := s.rm.Run(context.TODO())
	s.Equal(resp.Status(), http.StatusOK)
	s.NotNil(resp.Data())

	b, ok := (resp.Data()).(*model.APIBuild)
	s.True(ok)
	s.Equal(model.ToAPIString("build1"), b.Id)
	s.Equal(model.ToAPIString("branch"), b.ProjectId)
}

func (s *BuildByIdSuite) TestFindByIdFail() {
	s.rm.(*buildGetHandler).buildId = "build3"
	resp := s.rm.Run(context.TODO())
	s.NotEqual(resp.Status(), http.StatusOK)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for change build status by id

type BuildChangeStatusSuite struct {
	sc   *data.MockConnector
	data data.MockBuildConnector
	rm   gimlet.RouteHandler
	suite.Suite
}

func TestBuildChangeStatusSuite(t *testing.T) {
	suite.Run(t, new(BuildChangeStatusSuite))
}

func (s *BuildChangeStatusSuite) SetupSuite() {
	s.data = data.MockBuildConnector{
		CachedBuilds: []build.Build{
			{Id: "build1"},
			{Id: "build2"},
		},
	}
	s.sc = &data.MockConnector{
		MockBuildConnector: s.data,
	}
	s.rm = makeChangeStatusForBuild(s.sc)
}

func (s *BuildChangeStatusSuite) TestSetActivation() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.rm.(*buildChangeStatusHandler).buildId = "build1"
	var tmpTrue = true
	s.rm.(*buildChangeStatusHandler).Activated = &tmpTrue

	res := s.rm.Run(ctx)
	s.NotNil(res)

	b, ok := res.Data().(*model.APIBuild)
	s.True(ok)
	s.True(b.Activated)
	s.Equal(model.ToAPIString("user1"), b.ActivatedBy)
}

func (s *BuildChangeStatusSuite) TestSetActivationFail() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.rm.(*buildChangeStatusHandler).buildId = "zzz"
	var tmpTrue = true
	s.rm.(*buildChangeStatusHandler).Activated = &tmpTrue

	resp := s.rm.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
	s.Contains(fmt.Sprint(resp.Data()), "not found")
}

func (s *BuildChangeStatusSuite) TestSetPriority() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.rm.(*buildChangeStatusHandler).buildId = "build1"
	var tmpSeven = int64(7)
	s.rm.(*buildChangeStatusHandler).Priority = &tmpSeven

	res := s.rm.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
	s.NotNil(res)
	_, ok := (res.Data()).(*model.APIBuild)
	s.True(ok)
}

func (s *BuildChangeStatusSuite) TestSetPriorityManualFail() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.rm.(*buildChangeStatusHandler).buildId = "build1"
	s.sc.FailOnChangePriority = true
	tmpInt := int64(7)
	s.rm.(*buildChangeStatusHandler).Priority = &tmpInt
	resp := s.rm.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
	s.sc.FailOnChangePriority = false
}

func (s *BuildChangeStatusSuite) TestSetPriorityPrivilegeFail() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.sc.SetSuperUsers([]string{"admin"})

	s.rm.(*buildChangeStatusHandler).buildId = "build1"
	tmpInt := int64(1000)
	s.rm.(*buildChangeStatusHandler).Priority = &tmpInt
	resp := s.rm.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
	s.Contains(fmt.Sprint(resp.Data()), "Insufficient privilege to set priority")
}

////////////////////////////////////////////////////////////////////////
//
// Tests for abort build route

type BuildAbortSuite struct {
	sc   *data.MockConnector
	rm   gimlet.RouteHandler
	data data.MockBuildConnector

	suite.Suite
}

func TestBuildAbortSuite(t *testing.T) {
	suite.Run(t, new(BuildAbortSuite))
}

func (s *BuildAbortSuite) SetupSuite() {
	s.data = data.MockBuildConnector{
		CachedBuilds: []build.Build{
			{Id: "build1", Project: "branch"},
			{Id: "build2", Project: "notbranch"},
		},
		CachedProjects: map[string]*serviceModel.ProjectRef{
			"branch": {Repo: "project", Identifier: "branch"},
		},
		CachedAborted: make(map[string]string),
	}
	s.sc = &data.MockConnector{
		MockBuildConnector: s.data,
	}
}

func (s *BuildAbortSuite) SetupTest() {
	s.rm = makeAbortBuild(s.sc)
}

func (s *BuildAbortSuite) TestAbort() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.rm.(*buildAbortHandler).buildId = "build1"
	res := s.rm.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
	s.NotNil(res)

	s.Equal("user1", s.data.CachedAborted["build1"])
	s.Equal("", s.data.CachedAborted["build2"])
	b, ok := res.Data().(*model.APIBuild)
	s.True(ok)
	s.Equal(model.ToAPIString("build1"), b.Id)

	res = s.rm.Run(ctx)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted["build1"])
	s.Equal("", s.data.CachedAborted["build2"])
	b, ok = res.Data().(*model.APIBuild)
	s.True(ok)
	s.Equal(model.ToAPIString("build1"), b.Id)
}

func (s *BuildAbortSuite) TestAbortFail() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.rm.(*buildAbortHandler).buildId = "build1"
	s.sc.MockBuildConnector.FailOnAbort = true
	resp := s.rm.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
}

////////////////////////////////////////////////////////////////////////
//
// Tests for restart build route

type BuildRestartSuite struct {
	sc   *data.MockConnector
	rm   gimlet.RouteHandler
	data data.MockBuildConnector

	suite.Suite
}

func TestBuildRestartSuite(t *testing.T) {
	suite.Run(t, new(BuildRestartSuite))
}

func (s *BuildRestartSuite) SetupSuite() {
	s.data = data.MockBuildConnector{
		CachedBuilds: []build.Build{
			{Id: "build1", Project: "branch"},
			{Id: "build2", Project: "notbranch"},
		},
	}
	s.sc = &data.MockConnector{
		MockBuildConnector: s.data,
	}
}

func (s *BuildRestartSuite) SetupTest() {
	s.rm = makeRestartBuild(s.sc)
}

func (s *BuildRestartSuite) TestRestart() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.rm.(*buildRestartHandler).buildId = "build1"

	res := s.rm.Run(ctx)
	s.NotNil(res)
	b, ok := res.Data().(*model.APIBuild)
	s.True(ok)
	s.Equal(model.ToAPIString("build1"), b.Id)

	res = s.rm.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	b, ok = res.Data().(*model.APIBuild)
	s.True(ok)
	s.Equal(model.ToAPIString("build1"), b.Id)
}

func (s *BuildRestartSuite) TestRestartFail() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.rm.(*buildRestartHandler).buildId = "build1"
	s.sc.FailOnRestart = true
	resp := s.rm.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
}
