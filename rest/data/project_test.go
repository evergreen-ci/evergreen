package data

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
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

func getMockProjectSettings() model.ProjectSettingsEvent {
	return model.ProjectSettingsEvent{
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
			ID:        mgobson.ObjectIdHex("5bedc72ee4055d31f0340b1d"),
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
}

func TestProjectConnectorGetSuite(t *testing.T) {
	s := new(ProjectConnectorGetSuite)
	s.setup = func() error {
		s.ctx = &DBConnector{}

		s.Require().NoError(db.ClearCollections(model.ProjectRefCollection))

		projects := []*model.ProjectRef{
			{
				Identifier:  "projectA",
				Private:     false,
				CommitQueue: model.CommitQueueParams{Enabled: true},
				Owner:       "evergreen-ci",
				Repo:        "gimlet",
				Branch:      "master",
			},
			{
				Identifier:  "projectB",
				Private:     true,
				CommitQueue: model.CommitQueueParams{Enabled: true},
				Owner:       "evergreen-ci",
				Repo:        "evergreen",
				Branch:      "master",
			},
			{
				Identifier:  "projectC",
				Private:     true,
				CommitQueue: model.CommitQueueParams{Enabled: true},
				Owner:       "mongodb",
				Repo:        "mongo",
				Branch:      "master",
			},
			{Identifier: "projectD", Private: false},
			{Identifier: "projectE", Private: false},
			{Identifier: "projectF", Private: true},
		}

		for _, p := range projects {
			if err := p.Insert(); err != nil {
				return err
			}
		}

		before := getMockProjectSettings()
		after := getMockProjectSettings()
		after.GitHubHooksEnabled = false

		h :=
			event.EventLogEntry{
				Timestamp:    time.Now(),
				ResourceType: model.EventResourceTypeProject,
				EventType:    model.EventTypeProjectModified,
				ResourceId:   projectId,
				Data: &model.ProjectChangeEvent{
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
		projectId := "mci2"
		beforeSettings := restModel.APIProjectSettings{
			ProjectRef: restModel.APIProjectRef{
				Owner:      restModel.ToAPIString("admin"),
				Enabled:    true,
				Private:    true,
				Identifier: restModel.ToAPIString(projectId),
				Admins:     []restModel.APIString{},
			},
			GitHubWebhooksEnabled: true,
			Vars: restModel.APIProjectVars{
				Vars:        map[string]string{},
				PrivateVars: map[string]bool{},
			},
			Aliases: []restModel.APIProjectAlias{restModel.APIProjectAlias{
				Alias:   restModel.ToAPIString("alias1"),
				Variant: restModel.ToAPIString("ubuntu"),
				Task:    restModel.ToAPIString("subcommand"),
			},
			},
			Subscriptions: []restModel.APISubscription{restModel.APISubscription{
				ID:           restModel.ToAPIString("subscription1"),
				ResourceType: restModel.ToAPIString("project"),
				Owner:        restModel.ToAPIString("admin"),
				Subscriber: restModel.APISubscriber{
					Type:   restModel.ToAPIString(event.GithubPullRequestSubscriberType),
					Target: restModel.APIGithubPRSubscriber{},
				},
			},
			},
		}

		afterSettings := beforeSettings
		afterSettings.ProjectRef.Enabled = false

		projectEvents := []restModel.APIProjectEvent{}
		for i := 0; i < projEventCount; i++ {
			projectEvents = append(projectEvents, restModel.APIProjectEvent{
				Timestamp: time.Now().Add(time.Second * time.Duration(-i)),
				User:      restModel.ToAPIString("me"),
				Before:    beforeSettings,
				After:     afterSettings,
			})
		}

		s.ctx = &MockConnector{MockProjectConnector: MockProjectConnector{
			CachedProjects: []model.ProjectRef{
				{
					Identifier:  "projectA",
					Private:     false,
					CommitQueue: model.CommitQueueParams{Enabled: true},
					Owner:       "evergreen-ci",
					Repo:        "gimlet",
					Branch:      "master",
				},
				{
					Identifier:  "projectB",
					Private:     true,
					CommitQueue: model.CommitQueueParams{Enabled: true},
					Owner:       "evergreen-ci",
					Repo:        "evergreen",
					Branch:      "master",
				},
				{
					Identifier:  "projectC",
					Private:     true,
					CommitQueue: model.CommitQueueParams{Enabled: true},
					Owner:       "evergreen-ci",
					Repo:        "evergreen",
					Branch:      "master",
				},
				{Identifier: "projectD", Private: false},
				{Identifier: "projectE", Private: false},
				{Identifier: "projectF", Private: true},
			},
			CachedEvents: projectEvents,
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

func (s *ProjectConnectorGetSuite) TestGetProjectWithCommitQueueByOwnerRepoAndBranch() {
	projRef, err := s.ctx.GetProjectWithCommitQueueByOwnerRepoAndBranch("octocat", "hello-world", "master")
	s.Error(err)

	projRef, err = s.ctx.GetProjectWithCommitQueueByOwnerRepoAndBranch("evergreen-ci", "evergreen", "master")
	s.NoError(err)
	s.Equal("projectB", projRef.Identifier)
}

func (s *ProjectConnectorGetSuite) TestFindProjectVarsById() {
	s.Require().NoError(db.ClearCollections(model.ProjectVarsCollection))

	vars := &model.ProjectVars{
		Id:          projectId,
		Vars:        map[string]string{"a": "1", "b": "3"},
		PrivateVars: map[string]bool{"b": true},
	}
	s.NoError(vars.Insert())

	res, err := s.ctx.FindProjectVarsById(projectId)
	s.NoError(err)
	s.Equal(vars, res)
}

func (s *ProjectConnectorGetSuite) TestUpdateProjectVars() {
	s.Require().NoError(db.ClearCollections(model.ProjectVarsCollection))

	vars := model.ProjectVars{
		Id:          projectId,
		Vars:        map[string]string{"a": "1", "b": "3"},
		PrivateVars: map[string]bool{"b": true},
	}

	s.NoError(vars.Insert())

	//successful update
	newVars2 := model.ProjectVars{
		Id:          projectId,
		Vars:        map[string]string{"b": "2", "c": "3"},
		PrivateVars: map[string]bool{"b": false, "c": true},
	}
	varsToDelete := []string{"a"}
	s.NoError(s.ctx.UpdateProjectVars(&newVars2, varsToDelete))
	s.Equal(newVars2.Vars["b"], "2")
	s.Equal(newVars2.Vars["c"], "3")
	_, ok := newVars2.Vars["a"]
	s.False(ok)

	s.Equal(newVars2.PrivateVars["b"], false)
	s.Equal(newVars2.PrivateVars["c"], true)
	_, ok = newVars2.PrivateVars["a"]
	s.False(ok)

	//unsuccessful update
	newVars2.Id = "not-an-id"
	s.Error(s.ctx.UpdateProjectVars(&newVars2, varsToDelete))
}
