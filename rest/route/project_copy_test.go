package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"

	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

type ProjectCopySuite struct {
	route  *projectCopyHandler
	cancel context.CancelFunc

	suite.Suite
}

func TestProjectCopySuite(t *testing.T) {
	suite.Run(t, new(ProjectCopySuite))
}

func (s *ProjectCopySuite) SetupSuite() {
	s.NoError(db.ClearCollections(model.ProjectRefCollection, user.Collection, model.ProjectVarsCollection, fakeparameter.Collection,
		evergreen.ScopeCollection, evergreen.RoleCollection))
	pRefs := []model.ProjectRef{
		{
			Id:         "12345",
			Identifier: "projectA",
			Branch:     "abcd",
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Enabled:    true,
			Admins:     []string{"my-user"},
		},
		{
			Id:         "23456",
			Identifier: "projectB",
			Branch:     "bcde",
			Owner:      "evergreen-ci",
			Repo:       "evergreen",
			Enabled:    true,
			Admins:     []string{"my-user"},
		},
	}
	for _, pRef := range pRefs {
		s.NoError(pRef.Insert(s.T().Context()))
	}
	projectVar := &model.ProjectVars{
		Id:          "12345",
		Vars:        map[string]string{"a": "1", "b": "2"},
		PrivateVars: map[string]bool{"b": true},
	}
	s.NoError(projectVar.Insert(s.T().Context()))
}

func (s *ProjectCopySuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.route = &projectCopyHandler{
		env: testutil.NewEnvironment(ctx, s.T()),
	}
}

func (s *ProjectCopySuite) TearDownTest() {
	s.cancel()
}

func (s *ProjectCopySuite) TestParse() {
	ctx := context.Background()
	request, err := http.NewRequest(http.MethodPost, "/projects/projectA/copy?new_project=projectB", nil)
	options := map[string]string{"project_id": "projectA"}
	request = gimlet.SetURLVars(request, options)
	s.NoError(err)
	s.NoError(s.route.Parse(ctx, request))
	s.Equal("projectA", s.route.oldProject)
	s.Equal("projectB", s.route.newProject)
}

func (s *ProjectCopySuite) TestCopyToExistingProjectFails() {
	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "me"})
	s.route.oldProject = "projectA"
	s.route.newProject = "projectB"
	resp := s.route.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusBadRequest, resp.Status())
}

func (s *ProjectCopySuite) TestCopyToNewProject() {
	u := &user.DBUser{Id: "me"}
	admin := &user.DBUser{Id: "my-user"}
	s.NoError(u.Insert(s.T().Context()))
	s.NoError(admin.Insert(s.T().Context()))
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, u)
	s.route.oldProject = "projectA"
	s.route.newProject = "projectC"
	resp := s.route.Run(ctx)
	s.NotNil(resp)
	s.Require().Equal(http.StatusOK, resp.Status())

	newProject := resp.Data().(*restmodel.APIProjectRef)
	s.Require().NotNil(newProject)
	s.NotEqual("projectC", utility.FromStringPtr(newProject.Id))
	s.Equal("projectC", utility.FromStringPtr(newProject.Identifier))
	s.Equal("abcd", utility.FromStringPtr(newProject.Branch))
	s.False(*newProject.Enabled)
	s.Require().Len(newProject.Admins, 2)
	s.Contains(utility.FromStringPtrSlice(newProject.Admins), "my-user")
	s.Contains(utility.FromStringPtrSlice(newProject.Admins), "me")
	usrs, err := user.FindByRole(s.T().Context(), model.GetProjectAdminRole(utility.FromStringPtr(newProject.Id)))
	s.NoError(err)
	s.Len(usrs, 2)

	res, err := data.FindProjectById(s.T().Context(), "projectC", false, false)
	s.NoError(err)
	s.NotNil(res)
	res, err = data.FindProjectById(s.T().Context(), "projectA", false, false)
	s.NoError(err)
	s.NotNil(res)
	vars, err := data.FindProjectVarsById(s.T().Context(), utility.FromStringPtr(newProject.Id), "", false)
	s.NoError(err)
	s.Require().NotNil(vars)
	s.Len(vars.Vars, 2)
}

type copyVariablesSuite struct {
	route  *copyVariablesHandler
	ctx    context.Context
	cancel context.CancelFunc

	suite.Suite
}

func TestCopyVariablesSuite(t *testing.T) {
	suite.Run(t, new(copyVariablesSuite))
}

func (s *copyVariablesSuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	s.cancel = cancel

	s.route = &copyVariablesHandler{usr: &user.DBUser{Id: "admin"}}
	s.NoError(db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection, fakeparameter.Collection, model.RepoRefCollection, event.EventCollection))
	pRefs := []model.ProjectRef{
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
	}
	for _, pRef := range pRefs {
		s.NoError(pRef.Insert(s.T().Context()))
	}
	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id: "repoRef",
	}}
	s.NoError(repoRef.Replace(s.ctx))
	projectVar1 := &model.ProjectVars{
		Id:            "projectA",
		Vars:          map[string]string{"apple": "red", "hello": "world"},
		PrivateVars:   map[string]bool{"hello": true},
		AdminOnlyVars: map[string]bool{"hello": true},
	}
	projectVar2 := &model.ProjectVars{
		Id:          "projectB",
		Vars:        map[string]string{"banana": "yellow", "apple": "green", "hello": "its me"},
		PrivateVars: map[string]bool{},
	}
	projectVar3 := model.ProjectVars{
		Id:          "repoRef",
		Vars:        map[string]string{"chicago": "cubs"},
		PrivateVars: map[string]bool{},
	}

	s.NoError(projectVar1.Insert(s.T().Context()))
	s.NoError(projectVar2.Insert(s.T().Context()))
	s.NoError(projectVar3.Insert(s.T().Context()))
}

func (s *copyVariablesSuite) TearDownTest() {
	s.cancel()
}

func (s *copyVariablesSuite) TestParse() {
	opts := copyVariablesOptions{
		DryRun: true,
		CopyTo: "projectB",
	}
	s.route.usr = nil
	jsonBytes, err := json.Marshal(opts)
	s.NoError(err)
	body := bytes.NewReader(jsonBytes)
	request, err := http.NewRequest(http.MethodPost, "/projects/projectA/copy/variables", body)
	options := map[string]string{"project_id": "projectA"}
	request = gimlet.SetURLVars(request, options)
	s.NoError(err)
	s.NoError(s.route.Parse(s.ctx, request))
	s.Equal("projectA", s.route.copyFrom)
	s.Equal("projectB", s.route.opts.CopyTo)
	s.True(s.route.opts.DryRun)
	s.False(s.route.opts.Overwrite)
	s.Require().NotNil(s.route.usr)
	s.Equal("me", s.route.usr.Id)
}

func (s *copyVariablesSuite) TestCopyAllVariables() {
	s.route.copyFrom = "projectA"
	s.route.opts = copyVariablesOptions{
		CopyTo:         "projectB",
		DryRun:         true,
		IncludePrivate: true,
	}
	// Explicitly don't set private and admin-only variables for testing.
	newProjectVar := &model.ProjectVars{
		Id:   "projectB",
		Vars: map[string]string{"banana": "yellow"},
	}
	_, err := newProjectVar.Upsert(s.ctx)
	s.NoError(err)
	resp := s.route.Run(s.ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	projectVars, err := model.FindOneProjectVars(s.ctx, "projectB")
	s.NoError(err)
	s.Len(projectVars.Vars, 1)
	events, err := model.MostRecentProjectEvents(s.ctx, s.route.opts.CopyTo, 100)
	s.NoError(err)
	s.Len(events, 0)

	s.route.opts.DryRun = false
	resp = s.route.Run(s.ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	projectVars, err = model.FindOneProjectVars(s.ctx, "projectB")
	s.NoError(err)
	s.Len(projectVars.Vars, 3)
	s.Equal("world", projectVars.Vars["hello"])
	s.Equal("red", projectVars.Vars["apple"])
	s.True(projectVars.PrivateVars["hello"])
	events, err = model.MostRecentProjectEvents(s.ctx, s.route.opts.CopyTo, 100)
	s.NoError(err)
	s.Len(events, 1)
}

func (s *copyVariablesSuite) TestCopyAllVariablesWithOverlap() {
	s.route.copyFrom = "projectA"
	s.route.opts = copyVariablesOptions{
		CopyTo:         "projectB",
		DryRun:         true,
		IncludePrivate: true,
	}
	resp := s.route.Run(s.ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	result := (resp.Data()).(*restmodel.APIProjectVars)
	s.Len(result.Vars, 2)
	s.Equal("", result.Vars["hello"]) // redacted
	s.Equal("red", result.Vars["apple"])
	events, err := model.MostRecentProjectEvents(s.ctx, s.route.opts.CopyTo, 100)
	s.NoError(err)
	s.Len(events, 0)

	s.route.opts.DryRun = false
	resp = s.route.Run(s.ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	projectVars, err := model.FindOneProjectVars(s.ctx, "projectB")
	s.NoError(err)
	s.Len(projectVars.Vars, 3)
	s.Equal("world", projectVars.Vars["hello"]) // overwrites old variable
	s.True(projectVars.PrivateVars["hello"])
	s.Equal("red", projectVars.Vars["apple"])
	s.False(projectVars.PrivateVars["apple"])
	s.Equal("yellow", projectVars.Vars["banana"]) // unchanged
	events, err = model.MostRecentProjectEvents(s.ctx, s.route.opts.CopyTo, 100)
	s.NoError(err)
	s.Len(events, 1)

}

func (s *copyVariablesSuite) TestCopyVariablesWithOverwrite() {
	s.route.copyFrom = "projectA"
	s.route.opts = copyVariablesOptions{
		CopyTo:         "projectB",
		DryRun:         true,
		IncludePrivate: true,
		Overwrite:      true,
	}
	resp := s.route.Run(s.ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	result := (resp.Data()).(*restmodel.APIProjectVars)
	s.Len(result.Vars, 2)
	s.Equal("", result.Vars["hello"]) // redacted
	s.Equal("red", result.Vars["apple"])
	events, err := model.MostRecentProjectEvents(s.ctx, s.route.opts.CopyTo, 100)
	s.NoError(err)
	s.Len(events, 0)

	s.route.opts.DryRun = false
	resp = s.route.Run(s.ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	projectVars, err := model.FindOneProjectVars(s.ctx, "projectB")
	s.NoError(err)
	s.Len(projectVars.Vars, 2)
	s.Equal("world", projectVars.Vars["hello"]) // overwrites old variable
	s.True(projectVars.PrivateVars["hello"])
	s.Equal("red", projectVars.Vars["apple"])
	s.False(projectVars.PrivateVars["apple"])
	_, ok := projectVars.Vars["banana"] // no longer exists
	s.False(ok)
	events, err = model.MostRecentProjectEvents(s.ctx, s.route.opts.CopyTo, 100)
	s.NoError(err)
	s.Len(events, 1)
}

func (s *copyVariablesSuite) TestCopyToRepo() {
	s.route.copyFrom = "projectA"
	s.route.opts = copyVariablesOptions{
		CopyTo:         "repoRef",
		IncludePrivate: true,
	}
	resp := s.route.Run(s.ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	projectVars, err := model.FindOneProjectVars(s.ctx, "repoRef")
	s.NoError(err)
	s.Len(projectVars.Vars, 3)
	s.Equal("world", projectVars.Vars["hello"])
	s.Equal("red", projectVars.Vars["apple"])
	s.Equal("cubs", projectVars.Vars["chicago"])
	s.True(projectVars.PrivateVars["hello"])
	events, err := model.MostRecentProjectEvents(s.ctx, s.route.opts.CopyTo, 100)
	s.NoError(err)
	s.Len(events, 1)
}

func (s *copyVariablesSuite) TestCopyFromRepo() {
	s.route.copyFrom = "repoRef"
	s.route.opts = copyVariablesOptions{
		CopyTo:         "projectA",
		IncludePrivate: true,
	}
	resp := s.route.Run(s.ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	projectVars, err := model.FindOneProjectVars(s.ctx, "projectA")
	s.NoError(err)
	s.Len(projectVars.Vars, 3)
	s.Equal("world", projectVars.Vars["hello"])
	s.Equal("red", projectVars.Vars["apple"])
	s.Equal("cubs", projectVars.Vars["chicago"])
	s.True(projectVars.PrivateVars["hello"])
	events, err := model.MostRecentProjectEvents(s.ctx, s.route.opts.CopyTo, 100)
	s.NoError(err)
	s.Len(events, 1)
}
