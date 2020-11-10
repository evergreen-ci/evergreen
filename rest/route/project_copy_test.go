package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
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
				Id:         "projectA",
				Identifier: "projectA",
				Branch:     "abcd",
				Enabled:    true,
				Admins:     []string{"my-user"},
			},
			{
				Id:         "projectB",
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
	s.Equal("projectA", s.route.oldProject)
	s.Equal("projectB", s.route.newProject)
}

func (s *ProjectCopySuite) TestCopyToExistingProjectFails() {
	ctx := context.Background()
	s.route.oldProject = "projectA"
	s.route.newProject = "projectB"
	resp := s.route.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusBadRequest, resp.Status())
}

func (s *ProjectCopySuite) TestCopyToNewProject() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{})
	s.route.oldProject = "projectA"
	s.route.newProject = "projectC"
	resp := s.route.Run(ctx)
	s.NotNil(resp)
	s.Require().Equal(http.StatusOK, resp.Status())

	newProject := resp.Data().(*restmodel.APIProjectRef)
	s.Equal("projectC", restmodel.FromStringPtr(newProject.Id))
	s.Equal("projectC", restmodel.FromStringPtr(newProject.Identifier))
	s.Equal("abcd", restmodel.FromStringPtr(newProject.Branch))
	s.False(newProject.Enabled)
	s.Require().Len(newProject.Admins, 1)
	s.Equal("my-user", restmodel.FromStringPtr(newProject.Admins[0]))

	res, err := s.route.sc.FindProjectById("projectC")
	s.NoError(err)
	s.NotNil(res)
	res, err = s.route.sc.FindProjectById("projectA")
	s.NoError(err)
	s.NotNil(res)
}

type copyVariablesSuite struct {
	data  data.MockProjectConnector
	sc    *data.MockConnector
	route *copyVariablesHandler

	suite.Suite
}

func TestCopyVariablesSuite(t *testing.T) {
	suite.Run(t, new(copyVariablesSuite))
}

func (s *copyVariablesSuite) SetupSuite() {
	s.data = data.MockProjectConnector{
		CachedProjects: []model.ProjectRef{
			{
				Id:      "projectA",
				Branch:  "abcd",
				Enabled: true,
				Admins:  []string{"my-user"},
			},
			{
				Id:      "projectB",
				Branch:  "bcde",
				Enabled: true,
				Admins:  []string{"my-user"},
			},
		},
		CachedVars: []*model.ProjectVars{
			{
				Id:          "projectA",
				Vars:        map[string]string{"apple": "red", "hello": "world"},
				PrivateVars: map[string]bool{"hello": true},
			},
			{
				Id:          "projectB",
				Vars:        map[string]string{"banana": "yellow", "apple": "green", "hello": "its me"},
				PrivateVars: map[string]bool{},
			},
		},
	}

	s.sc = &data.MockConnector{
		URL:                  "https://evergreen.example.net",
		MockProjectConnector: s.data,
	}
}

func (s *copyVariablesSuite) SetupTest() {
	s.route = &copyVariablesHandler{sc: s.sc}
}

func (s *copyVariablesSuite) TestParse() {
	ctx := context.Background()
	opts := copyVariablesOptions{
		DryRun: true,
		CopyTo: "projectB",
	}
	jsonBytes, err := json.Marshal(opts)
	s.NoError(err)
	body := bytes.NewReader(jsonBytes)
	request, err := http.NewRequest("POST", "/projects/projectA/copy/variables", body)
	options := map[string]string{"project_id": "projectA"}
	request = gimlet.SetURLVars(request, options)
	s.NoError(err)
	s.NoError(s.route.Parse(ctx, request))
	s.Equal("projectA", s.route.copyFrom)
	s.Equal("projectB", s.route.opts.CopyTo)
	s.True(s.route.opts.DryRun)
	s.False(s.route.opts.Overwrite)
}

func (s *copyVariablesSuite) TestCopyAllVariables() {
	ctx := context.Background()
	s.route.copyFrom = "projectA"
	s.route.opts = copyVariablesOptions{
		CopyTo:         "projectB",
		DryRun:         true,
		IncludePrivate: true,
	}
	delete(s.data.CachedVars[1].Vars, "hello")
	delete(s.data.CachedVars[1].Vars, "apple")

	resp := s.route.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.Len(s.data.CachedVars[1].Vars, 1)

	s.route.opts.DryRun = false
	resp = s.route.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.Len(s.data.CachedVars[1].Vars, 3)
	s.Equal("world", s.data.CachedVars[1].Vars["hello"])
	s.Equal("red", s.data.CachedVars[1].Vars["apple"])
	s.True(s.data.CachedVars[1].PrivateVars["hello"])
}

func (s *copyVariablesSuite) TestCopyAllVariablesWithOverlap() {
	ctx := context.Background()
	s.route.copyFrom = "projectA"
	s.route.opts = copyVariablesOptions{
		CopyTo:         "projectB",
		DryRun:         true,
		IncludePrivate: true,
	}
	resp := s.route.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	result := (resp.Data()).(*restmodel.APIProjectVars)
	s.Len(result.Vars, 2)
	s.Equal("", result.Vars["hello"]) // redacted
	s.Equal("red", result.Vars["apple"])

	s.route.opts.DryRun = false
	resp = s.route.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.Len(s.data.CachedVars[1].Vars, 3)
	s.Equal("world", s.data.CachedVars[1].Vars["hello"]) // overwrites old variable
	s.True(s.data.CachedVars[1].PrivateVars["hello"])
	s.Equal("red", s.data.CachedVars[1].Vars["apple"])
	s.False(s.data.CachedVars[1].PrivateVars["apple"])
	s.Equal("yellow", s.data.CachedVars[1].Vars["banana"]) // unchanged

}

func (s *copyVariablesSuite) TestCopyVariablesWithOverwrite() {
	ctx := context.Background()
	s.route.copyFrom = "projectA"
	s.route.opts = copyVariablesOptions{
		CopyTo:         "projectB",
		DryRun:         true,
		IncludePrivate: true,
		Overwrite:      true,
	}
	resp := s.route.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	result := (resp.Data()).(*restmodel.APIProjectVars)
	s.Len(result.Vars, 2)
	s.Equal("", result.Vars["hello"]) // redacted
	s.Equal("red", result.Vars["apple"])

	s.route.opts.DryRun = false
	resp = s.route.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.Len(s.data.CachedVars[1].Vars, 2)
	s.Equal("world", s.data.CachedVars[1].Vars["hello"]) // overwrites old variable
	s.True(s.data.CachedVars[1].PrivateVars["hello"])
	s.Equal("red", s.data.CachedVars[1].Vars["apple"])
	s.False(s.data.CachedVars[1].PrivateVars["apple"])
	_, ok := s.data.CachedVars[1].Vars["banana"] // no longer exists
	s.False(ok)
}
