package route

import (
	"testing"

	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch build by id

type BuildByIdSuite struct {
	sc   *data.MockConnector
	data data.MockBuildConnector

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

func (s *BuildByIdSuite) TestFindByIdProjFound() {
	rm := getBuildByIdRouteManager("", 2)
	(rm.Methods[0].RequestHandler).(*buildGetHandler).buildId = "build1"
	res, err := rm.Methods[0].Execute(nil, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))

	b, ok := (res.Result[0]).(*model.APIBuild)
	s.True(ok)
	s.Equal(model.APIString("build1"), b.Id)
	s.Equal(model.APIString("project"), b.ProjectId)
}

func (s *BuildByIdSuite) TestFindByIdProjNotFound() {
	rm := getBuildByIdRouteManager("", 2)
	(rm.Methods[0].RequestHandler).(*buildGetHandler).buildId = "build2"
	res, err := rm.Methods[0].Execute(nil, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))

	b, ok := (res.Result[0]).(*model.APIBuild)
	s.True(ok)
	s.Equal(model.APIString("build2"), b.Id)
	s.Equal(model.APIString(""), b.ProjectId)
}

func (s *BuildByIdSuite) TestFindByIdFail() {
	rm := getBuildByIdRouteManager("", 2)
	(rm.Methods[0].RequestHandler).(*buildGetHandler).buildId = "build3"
	_, err := rm.Methods[0].Execute(nil, s.sc)
	s.Error(err)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for change build status by id

type BuildChangeStatusSuite struct {
	sc   *data.MockConnector
	data data.MockBuildConnector

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
}

func (s *BuildChangeStatusSuite) TestSetActivation() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getBuildByIdRouteManager("", 2)
	(rm.Methods[1].RequestHandler).(*buildChangeStatusHandler).buildId = "build1"
	var tmp_true = true
	(rm.Methods[1].RequestHandler).(*buildChangeStatusHandler).Activated = &tmp_true

	res, err := rm.Methods[1].Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))

	b, ok := (res.Result[0]).(*model.APIBuild)
	s.True(ok)
	s.True(b.Activated)
	s.Equal(model.APIString("user1"), b.ActivatedBy)
}

func (s *BuildChangeStatusSuite) TestSetActivationFail() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getBuildByIdRouteManager("", 2)
	(rm.Methods[1].RequestHandler).(*buildChangeStatusHandler).buildId = "zzz"
	var tmp_true = true
	(rm.Methods[1].RequestHandler).(*buildChangeStatusHandler).Activated = &tmp_true

	_, err := rm.Methods[1].Execute(ctx, s.sc)
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

func (s *BuildChangeStatusSuite) TestSetPriority() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getBuildByIdRouteManager("", 2)
	(rm.Methods[1].RequestHandler).(*buildChangeStatusHandler).buildId = "build1"
	var tmp_seven = int64(7)
	(rm.Methods[1].RequestHandler).(*buildChangeStatusHandler).Priority = &tmp_seven

	res, err := rm.Methods[1].Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))
	_, ok := (res.Result[0]).(*model.APIBuild)
	s.True(ok)
}

func (s *BuildChangeStatusSuite) TestSetPriorityManualFail() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getBuildByIdRouteManager("", 2)
	(rm.Methods[1].RequestHandler).(*buildChangeStatusHandler).buildId = "build1"
	s.sc.FailOnChangePriority = true
	tmp_int := int64(7)
	(rm.Methods[1].RequestHandler).(*buildChangeStatusHandler).Priority = &tmp_int
	_, err := rm.Methods[1].Execute(ctx, s.sc)
	s.Error(err)
	s.sc.FailOnChangePriority = false
}

func (s *BuildChangeStatusSuite) TestSetPriorityPrivilegeFail() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	s.sc.SetSuperUsers([]string{"admin"})

	rm := getBuildByIdRouteManager("", 2)
	(rm.Methods[1].RequestHandler).(*buildChangeStatusHandler).buildId = "build1"
	tmp_int := int64(1000)
	(rm.Methods[1].RequestHandler).(*buildChangeStatusHandler).Priority = &tmp_int
	_, err := rm.Methods[1].Execute(ctx, s.sc)

	s.Error(err)
	s.Contains(err.Error(), "Insufficient privilege to set priority")
}

////////////////////////////////////////////////////////////////////////
//
// Tests for abort build route

type BuildAbortSuite struct {
	sc   *data.MockConnector
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

func (s *BuildAbortSuite) TestAbort() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getBuildAbortRouteManager("", 2)
	(rm.Methods[0].RequestHandler).(*buildAbortHandler).buildId = "build1"
	res, err := rm.Methods[0].Execute(ctx, s.sc)

	s.NoError(err)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted["build1"])
	s.Equal("", s.data.CachedAborted["build2"])
	b, ok := (res.Result[0]).(*model.APIBuild)
	s.True(ok)
	s.Equal(model.APIString("build1"), b.Id)

	res, err = rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted["build1"])
	s.Equal("", s.data.CachedAborted["build2"])
	b, ok = (res.Result[0]).(*model.APIBuild)
	s.True(ok)
	s.Equal(model.APIString("build1"), b.Id)
}

func (s *BuildAbortSuite) TestAbortFail() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getBuildAbortRouteManager("", 2)
	(rm.Methods[0].RequestHandler).(*buildAbortHandler).buildId = "build1"
	s.sc.MockBuildConnector.FailOnAbort = true
	_, err := rm.Methods[0].Execute(ctx, s.sc)

	s.Error(err)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for restart build route

type BuildRestartSuite struct {
	sc   *data.MockConnector
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

func (s *BuildRestartSuite) TestRestart() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getBuildRestartManager("", 2)
	(rm.Methods[0].RequestHandler).(*buildRestartHandler).buildId = "build1"

	res, err := rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(res)
	b, ok := (res.Result[0]).(*model.APIBuild)
	s.True(ok)
	s.Equal(model.APIString("build1"), b.Id)

	res, err = rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(res)
	b, ok = (res.Result[0]).(*model.APIBuild)
	s.True(ok)
	s.Equal(model.APIString("build1"), b.Id)
}

func (s *BuildRestartSuite) TestRestartFail() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getBuildRestartManager("", 2)
	(rm.Methods[0].RequestHandler).(*buildRestartHandler).buildId = "build1"
	s.sc.FailOnRestart = true
	_, err := rm.Methods[0].Execute(ctx, s.sc)

	s.Error(err)
}
