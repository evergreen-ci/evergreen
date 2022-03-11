package route

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch build by id

type BuildByIdSuite struct {
	rm gimlet.RouteHandler
	suite.Suite
}

func TestBuildSuite(t *testing.T) {
	suite.Run(t, new(BuildByIdSuite))
}

func (s *BuildByIdSuite) SetupSuite() {
	s.NoError(db.ClearCollections(serviceModel.ProjectRefCollection, build.Collection))
	projRef := serviceModel.ProjectRef{Repo: "project", Id: "branch"}
	s.NoError(projRef.Insert())
	builds := []build.Build{
		{Id: "build1", Project: "branch"},
		{Id: "build2", Project: "notbranch"},
	}
	for _, item := range builds {
		s.Require().NoError(item.Insert())
	}
}

func (s *BuildByIdSuite) SetupTest() {
	s.rm = makeGetBuildByID()
}

func (s *BuildByIdSuite) TestFindByIdProjFound() {
	s.rm.(*buildGetHandler).buildId = "build1"
	resp := s.rm.Run(context.TODO())
	s.Equal(resp.Status(), http.StatusOK)
	s.NotNil(resp.Data())

	b, ok := (resp.Data()).(*model.APIBuild)
	s.True(ok)
	s.Equal(utility.ToStringPtr("build1"), b.Id)
	s.Equal(utility.ToStringPtr("branch"), b.ProjectId)
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
	rm gimlet.RouteHandler
	suite.Suite
}

func TestBuildChangeStatusSuite(t *testing.T) {
	suite.Run(t, new(BuildChangeStatusSuite))
}

func (s *BuildChangeStatusSuite) SetupSuite() {
	s.NoError(db.ClearCollections(build.Collection, serviceModel.VersionCollection))
	builds := []build.Build{
		{Id: "build1", Version: "v1"},
		{Id: "build2", Version: "v1"},
	}
	s.NoError((&serviceModel.Version{Id: "v1"}).Insert())
	for _, item := range builds {
		s.Require().NoError(item.Insert())
	}
	s.rm = makeChangeStatusForBuild()
}

func (s *BuildChangeStatusSuite) TestSetActivation() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.rm.(*buildChangeStatusHandler).buildId = "build1"
	var tmpTrue = true
	s.rm.(*buildChangeStatusHandler).Activated = &tmpTrue

	res := s.rm.Run(ctx)
	s.NotNil(res)

	b, err := build.FindOneId("build1")
	s.NoError(err)
	s.True(b.Activated)
	s.Equal(utility.ToStringPtr("user1"), &b.ActivatedBy)
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

func (s *BuildChangeStatusSuite) TestSetPriorityPrivilegeFail() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

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
	rm gimlet.RouteHandler

	suite.Suite
}

func TestBuildAbortSuite(t *testing.T) {
	suite.Run(t, new(BuildAbortSuite))
}

func (s *BuildAbortSuite) SetupSuite() {
	s.NoError(db.ClearCollections(serviceModel.ProjectRefCollection, build.Collection))
	projRef := serviceModel.ProjectRef{Repo: "project", Id: "branch"}
	s.NoError(projRef.Insert())
	builds := []build.Build{
		{Id: "build1", Project: "branch"},
		{Id: "build2", Project: "notbranch"},
	}
	for _, item := range builds {
		s.Require().NoError(item.Insert())
	}
}

func (s *BuildAbortSuite) SetupTest() {
	s.rm = makeAbortBuild()
}

func (s *BuildAbortSuite) TestAbort() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.rm.(*buildAbortHandler).buildId = "build1"
	res := s.rm.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
	s.NotNil(res)

	build1, err := build.FindOneId("build1")
	s.NoError(err)
	s.Equal("user1", build1.ActivatedBy)
	build2, err := build.FindOneId("build2")
	s.NoError(err)
	s.Equal("", build2.ActivatedBy)
	b, ok := res.Data().(*model.APIBuild)
	s.True(ok)
	s.Equal(utility.ToStringPtr("build1"), b.Id)

	res = s.rm.Run(ctx)
	s.NotNil(res)
	build1, err = build.FindOneId("build1")
	s.NoError(err)
	s.Equal("user1", build1.ActivatedBy)
	build2, err = build.FindOneId("build2")
	s.NoError(err)
	s.Equal("", build2.ActivatedBy)
	b, ok = res.Data().(*model.APIBuild)
	s.True(ok)
	s.Equal(utility.ToStringPtr("build1"), b.Id)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for restart build route

type BuildRestartSuite struct {
	rm gimlet.RouteHandler

	suite.Suite
}

func TestBuildRestartSuite(t *testing.T) {
	suite.Run(t, new(BuildRestartSuite))
}

func (s *BuildRestartSuite) SetupSuite() {
	s.NoError(db.ClearCollections(build.Collection))
	builds := []build.Build{
		{Id: "build1", Project: "branch"},
		{Id: "build2", Project: "notbranch"},
	}
	for _, item := range builds {
		s.Require().NoError(item.Insert())
	}
}

func (s *BuildRestartSuite) SetupTest() {
	s.rm = makeRestartBuild()
}

func (s *BuildRestartSuite) TestRestart() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	s.rm.(*buildRestartHandler).buildId = "build1"

	res := s.rm.Run(ctx)
	s.NotNil(res)
	b, ok := res.Data().(*model.APIBuild)
	s.True(ok)
	s.Equal(utility.ToStringPtr("build1"), b.Id)

	res = s.rm.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	b, ok = res.Data().(*model.APIBuild)
	s.True(ok)
	s.Equal(utility.ToStringPtr("build1"), b.Id)
}
