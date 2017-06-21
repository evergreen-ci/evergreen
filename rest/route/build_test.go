package route

import (
	"testing"

	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
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
		CachedProjects: make(map[string]*serviceModel.ProjectRef),
	}
	s.data.CachedProjects["branch"] = &serviceModel.ProjectRef{Repo: "project", Identifier: "branch"}
	s.sc = &data.MockConnector{
		MockBuildConnector: s.data,
	}
}

func (s *BuildSuite) TestFindByIdProjFound() {
	rm := getBuildIdRouteManager("build1", 2)
	(rm.Methods[0].RequestHandler).(*buildIdGetHandler).buildId = "build1"
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
	rm := getBuildIdRouteManager("build2", 2)
	(rm.Methods[0].RequestHandler).(*buildIdGetHandler).buildId = "build2"
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
	rm := getBuildIdRouteManager("build3", 2)
	(rm.Methods[0].RequestHandler).(*buildIdGetHandler).buildId = "build3"
	_, err := rm.Methods[0].Execute(nil, s.sc)
	s.Error(err)
}
