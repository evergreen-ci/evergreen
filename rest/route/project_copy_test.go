package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
)

type ProjectCopySuite struct {
	data  data.MockProjectConnector
	sc    *data.MockConnector
	route *projectCopyHandler

	suite.Suite
}

func TestProjectCopySuite(t *testing.T) {
	suite.Run(t, new(ProjectCopySuite))
}

func (s *ProjectCopySuite) SetupSuite() {
	s.data = data.MockProjectConnector{
		CachedProjects: []model.ProjectRef{
			{
				Identifier: "projectA",
				Branch:     "abcd",
				Enabled:    true,
				Admins:     []string{"my-user"},
			},
			{
				Identifier: "projectB",
				Branch:     "bcde",
				Enabled:    true,
				Admins:     []string{"my-user"},
			},
		},
		CachedVars: []*model.ProjectVars{
			{
				Id:          "projectA",
				Vars:        map[string]string{"a": "1", "b": "2"},
				PrivateVars: map[string]bool{"b": true},
			},
		},
	}

	s.sc = &data.MockConnector{
		URL:                  "https://evergreen.example.net",
		MockProjectConnector: s.data,
	}
}

func (s *ProjectCopySuite) SetupTest() {
	s.route = &projectCopyHandler{sc: s.sc}
}

func (s *ProjectCopySuite) TestParse() {
	ctx := context.Background()
	request, err := http.NewRequest("POST", "/projects/projectA/copy?new_project=projectB", nil)
	options := map[string]string{"project_id": "projectA"}
	request = gimlet.SetURLVars(request, options)
	s.NoError(err)
	s.NoError(s.route.Parse(ctx, request))
	s.Equal("projectA", s.route.oldProjectId)
	s.Equal("projectB", s.route.newProjectId)
}

func (s *ProjectCopySuite) TestCopyToExistingProjectFails() {
	ctx := context.Background()
	s.route.oldProjectId = "projectA"
	s.route.newProjectId = "projectB"
	resp := s.route.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusBadRequest, resp.Status())
}

func (s *ProjectCopySuite) TestCopyToNewProject() {
	ctx := context.Background()
	s.route.oldProjectId = "projectA"
	s.route.newProjectId = "projectC"
	resp := s.route.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	newProject := resp.Data().(*restmodel.APIProjectRef)
	s.Equal("projectC", restmodel.FromAPIString(newProject.Identifier))
	s.Equal("abcd", restmodel.FromAPIString(newProject.Branch))
	s.False(newProject.Enabled)
	s.Require().Len(newProject.Admins, 1)
	s.Equal("my-user", restmodel.FromAPIString(newProject.Admins[0]))

	res, err := s.route.sc.FindProjectById("projectC")
	s.NoError(err)
	s.NotNil(res)
	res, err = s.route.sc.FindProjectById("projectA")
	s.NoError(err)
	s.NotNil(res)
}
