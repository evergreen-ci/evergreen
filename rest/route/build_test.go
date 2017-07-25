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

type BuildSuite struct {
	sc   *data.MockConnector
	data data.MockBuildConnector

	suite.Suite
}

func TestBuildSuite(t *testing.T) {
	suite.Run(t, new(BuildSuite))
}

func (s *BuildSuite) SetupSuite() {
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

func (s *BuildSuite) TestFindByIdProjFound() {
	rm := getBuildGetRouteManager("", 2)
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

func (s *BuildSuite) TestFindByIdProjNotFound() {
	rm := getBuildGetRouteManager("", 2)
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

func (s *BuildSuite) TestFindByIdFail() {
	rm := getBuildGetRouteManager("", 2)
	(rm.Methods[0].RequestHandler).(*buildGetHandler).buildId = "build3"
	_, err := rm.Methods[0].Execute(nil, s.sc)
	s.Error(err)
}

func (s *BuildSuite) TestAbort() {
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
