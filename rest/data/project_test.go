package data

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by project route

type ProjectConnectorGetSuite struct {
	ctx      Connector
	setup    func() error
	teardown func() error
	suite.Suite
}

const (
	projectId      = "mci2"
	username       = "me"
	projEventCount = 10
)

var sampleProjectSettings = model.ProjectSettings{
	ProjectRef: model.ProjectRef{
		Owner:      "admin",
		Enabled:    true,
		Private:    true,
		Identifier: projectId,
		Admins:     []string{},
	},
	GitHubHooksEnabled: true,
	Vars: model.ProjectVars{
		Id:          projectId,
		Vars:        map[string]string{},
		PrivateVars: map[string]bool{},
	},
	Aliases: []model.ProjectAlias{model.ProjectAlias{
		ID:        bson.NewObjectId(),
		ProjectID: projectId,
		Alias:     "alias1",
		Variant:   "ubuntu",
		Task:      "subcommand",
	},
	},
	Subscriptions: []event.Subscription{event.Subscription{
		ID:           "subscription1",
		ResourceType: "project",
		Owner:        "admin",
		Subscriber: event.Subscriber{
			Type:   event.GithubPullRequestSubscriberType,
			Target: event.GithubPullRequestSubscriber{},
		},
	},
	},
}

func TestProjectConnectorGetSuite(t *testing.T) {
	s := new(ProjectConnectorGetSuite)
	s.setup = func() error {
		s.ctx = &DBConnector{}

		testutil.ConfigureIntegrationTest(t, testConfig, "TestProjectConnectorGetSuite")
		db.SetGlobalSessionProvider(testConfig.SessionFactory())

		projects := []*model.ProjectRef{
			{Identifier: "projectA", Private: false},
			{Identifier: "projectB", Private: true},
			{Identifier: "projectC", Private: true},
			{Identifier: "projectD", Private: false},
			{Identifier: "projectE", Private: false},
			{Identifier: "projectF", Private: true},
		}

		for _, p := range projects {
			if err := p.Insert(); err != nil {
				return err
			}
		}

		before := sampleProjectSettings
		after := before
		after.GitHubHooksEnabled = false

		h :=
			event.EventLogEntry{
				Timestamp:    time.Now(),
				ResourceType: model.ResourceTypeProject,
				EventType:    model.EventTypeProjectModified,
				ResourceId:   projectId,
				Data: &model.ProjectChange{
					User:   username,
					Before: before,
					After:  after,
				},
			}

		s.Require().NoError(db.ClearCollections(event.AllLogCollection))
		logger := event.NewDBEventLogger(event.AllLogCollection)
		for i := 0; i < projEventCount; i++ {
			eventShallowCpy := h
			s.NoError(logger.LogEvent(&eventShallowCpy))
		}

		return nil
	}

	s.teardown = func() error {
		return db.Clear(model.ProjectRefCollection)
	}

	suite.Run(t, s)
}

func TestMockProjectConnectorGetSuite(t *testing.T) {
	s := new(ProjectConnectorGetSuite)
	s.setup = func() error {

		s.ctx = &MockConnector{MockProjectConnector: MockProjectConnector{
			CachedProjects: []model.ProjectRef{
				{Identifier: "projectA", Private: false},
				{Identifier: "projectB", Private: true},
				{Identifier: "projectC", Private: true},
				{Identifier: "projectD", Private: false},
				{Identifier: "projectE", Private: false},
				{Identifier: "projectF", Private: true},
			},
		}}

		return nil
	}

	s.teardown = func() error { return nil }

	suite.Run(t, s)
}

func (s *ProjectConnectorGetSuite) SetupSuite() { s.Require().NoError(s.setup()) }

func (s *ProjectConnectorGetSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *ProjectConnectorGetSuite) TestFetchTooManyAsc() {
	isAuthenticated := false
	projects, err := s.ctx.FindProjects("", 7, 1, isAuthenticated)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 3)

	s.Equal("projectA", projects[0].Identifier)
	s.Equal("projectD", projects[1].Identifier)
	s.Equal("projectE", projects[2].Identifier)

	s.False(projects[0].Private)
	s.False(projects[1].Private)
	s.False(projects[2].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchTooManyAscAuth() {
	isAuthenticated := true
	projects, err := s.ctx.FindProjects("", 7, 1, isAuthenticated)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 6)

	s.Equal("projectA", projects[0].Identifier)
	s.Equal("projectB", projects[1].Identifier)
	s.Equal("projectC", projects[2].Identifier)

	s.False(projects[0].Private)
	s.True(projects[1].Private)
	s.True(projects[2].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchTooManyDesc() {
	isAuthenticated := false
	projects, err := s.ctx.FindProjects("zzz", 7, -1, isAuthenticated)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 3)

	s.Equal("projectE", projects[0].Identifier)
	s.Equal("projectD", projects[1].Identifier)
	s.Equal("projectA", projects[2].Identifier)

	s.False(projects[0].Private)
	s.False(projects[1].Private)
	s.False(projects[2].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchTooManyDescAuth() {
	isAuthenticated := true
	projects, err := s.ctx.FindProjects("zzz", 7, -1, isAuthenticated)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 6)

	s.Equal("projectF", projects[0].Identifier)
	s.Equal("projectE", projects[1].Identifier)
	s.Equal("projectD", projects[2].Identifier)

	s.True(projects[0].Private)
	s.False(projects[1].Private)
	s.False(projects[2].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchExactNumber() {
	isAuthenticated := false
	projects, err := s.ctx.FindProjects("", 3, 1, isAuthenticated)
	s.NoError(err)
	s.NotNil(projects)

	s.Len(projects, 3)
	s.Equal("projectA", projects[0].Identifier)
	s.Equal("projectD", projects[1].Identifier)
	s.Equal("projectE", projects[2].Identifier)

	s.False(projects[0].Private)
	s.False(projects[1].Private)
	s.False(projects[2].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchExactNumberAuth() {
	isAuthenticated := true
	projects, err := s.ctx.FindProjects("", 6, 1, isAuthenticated)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 6)

	s.Equal("projectA", projects[0].Identifier)
	s.Equal("projectB", projects[1].Identifier)
	s.Equal("projectC", projects[2].Identifier)

	s.False(projects[0].Private)
	s.True(projects[1].Private)
	s.True(projects[2].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchTooFewAsc() {
	isAuthenticated := false
	projects, err := s.ctx.FindProjects("", 2, 1, isAuthenticated)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 2)

	s.Equal("projectA", projects[0].Identifier)
	s.Equal("projectD", projects[1].Identifier)

	s.False(projects[0].Private)
	s.False(projects[1].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchTooFewAscAuth() {
	isAuthenticated := true
	projects, err := s.ctx.FindProjects("", 2, 1, isAuthenticated)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 2)

	s.Equal("projectA", projects[0].Identifier)
	s.Equal("projectB", projects[1].Identifier)

	s.False(projects[0].Private)
	s.True(projects[1].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchTooFewDesc() {
	isAuthenticated := false
	projects, err := s.ctx.FindProjects("zzz", 2, -1, isAuthenticated)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 2)

	s.Equal("projectE", projects[0].Identifier)
	s.Equal("projectD", projects[1].Identifier)

	s.False(projects[0].Private)
	s.False(projects[1].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchTooFewDescAuth() {
	isAuthenticated := true
	projects, err := s.ctx.FindProjects("zzz", 2, -1, isAuthenticated)
	s.NoError(err)
	s.NotNil(projects)

	s.Len(projects, 2)
	s.Equal("projectF", projects[0].Identifier)
	s.Equal("projectE", projects[1].Identifier)

	s.True(projects[0].Private)
	s.False(projects[1].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyWithinBoundAsc() {
	isAuthenticated := false
	projects, err := s.ctx.FindProjects("projectB", 1, 1, isAuthenticated)
	s.NoError(err)
	s.Len(projects, 1)
	s.Equal("projectD", projects[0].Identifier)
	s.False(projects[0].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyWithinBoundAscAuth() {
	isAuthenticated := true
	projects, err := s.ctx.FindProjects("projectB", 1, 1, isAuthenticated)
	s.NoError(err)
	s.Len(projects, 1)
	s.Equal("projectB", projects[0].Identifier)
	s.True(projects[0].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyWithinBoundDesc() {
	isAuthenticated := false
	projects, err := s.ctx.FindProjects("projectD", 1, -1, isAuthenticated)
	s.NoError(err)
	s.Len(projects, 1)
	s.Equal("projectA", projects[0].Identifier)
	s.False(projects[0].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyWithinBoundDescAuth() {
	isAuthenticated := true
	projects, err := s.ctx.FindProjects("projectD", 1, -1, isAuthenticated)
	s.NoError(err)
	s.Len(projects, 1)
	s.Equal("projectC", projects[0].Identifier)
	s.True(projects[0].Private)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyOutOfBoundAsc() {
	isAuthenticated := false
	projects, err := s.ctx.FindProjects("zzz", 1, 1, isAuthenticated)
	s.NoError(err)
	s.Len(projects, 0)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyOutOfBoundDesc() {
	isAuthenticated := false
	projects, err := s.ctx.FindProjects("aaa", 1, -1, isAuthenticated)
	s.NoError(err)
	s.Len(projects, 0)
}

func (s *ProjectConnectorGetSuite) TestGetProjectEvents() {
	events, err := s.ctx.GetProjectEventLog(projectId, time.Now(), 0)
	s.NoError(err)
	s.Equal(projEventCount, len(events))
}

////////////////////////////////////////////////////////////////////////
//
// Tests project create action
type ProjectConnectorCreateUpdateSuite struct {
	sc Connector
	suite.Suite
}

func TestProjectConnectorCreateUpdateSuite(t *testing.T) {
	suite.Run(t, new(ProjectConnectorCreateUpdateSuite))
}

func (s *ProjectConnectorCreateUpdateSuite) SetupSuite() {
	s.sc = &DBConnector{}
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

func (s *ProjectConnectorCreateUpdateSuite) TearDownSuite() {
	s.Require().NoError(db.Clear(model.ProjectRefCollection))
}

func (s *ProjectConnectorCreateUpdateSuite) TestCreateProject() {
	projectRef, err := s.sc.CreateProject(&restModel.APIProjectRef{
		Identifier: restModel.ToAPIString("id"),
		Branch:     restModel.ToAPIString("branch"),
		Admins: []restModel.APIString{
			restModel.ToAPIString("a"),
			restModel.ToAPIString("b"),
		},
	})

	s.NoError(err)
	s.NotNil(projectRef)

	s.Equal("id", restModel.FromAPIString(projectRef.Identifier))
	s.Equal("branch", restModel.FromAPIString(projectRef.Branch))
	s.Len(projectRef.Admins, 2)
	s.Equal("a", restModel.FromAPIString(projectRef.Admins[0]))
	s.Equal("b", restModel.FromAPIString(projectRef.Admins[1]))
}

func (s *ProjectConnectorCreateUpdateSuite) TestUpdateProject() {
	// Create sample project ref
	createdProject, err := s.sc.CreateProject(&restModel.APIProjectRef{
		Identifier: restModel.ToAPIString("id"),
		Admins: []restModel.APIString{
			restModel.ToAPIString("a"),
			restModel.ToAPIString("b"),
		},
	})

	s.NoError(err)
	s.NotNil(createdProject)

	// Test set up
	updatedProject, err := s.sc.UpdateProject(&restModel.APIProjectRef{
		Identifier: createdProject.Identifier,
		Owner:      restModel.ToAPIString("owner"),
		Admins: []restModel.APIString{
			restModel.ToAPIString("a"),
			restModel.ToAPIString("c"),
		},
	})

	// Test assertion
	s.NoError(err)
	s.NotNil(updatedProject)

	s.Equal("id", restModel.FromAPIString(updatedProject.Identifier))
	s.Equal("owner", restModel.FromAPIString(updatedProject.Owner))
	s.Len(updatedProject.Admins, 2)
	s.Equal("a", restModel.FromAPIString(updatedProject.Admins[0]))
	s.Equal("c", restModel.FromAPIString(updatedProject.Admins[1]))
}
