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

		s.Require().NoError(db.ClearCollections(model.ProjectRefCollection, model.ProjectVarsCollection))

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
			if _, err := model.GetNewRevisionOrderNumber(p.Identifier); err != nil {
				return err
			}
		}

		vars := &model.ProjectVars{
			Id:          projectId,
			Vars:        map[string]string{"a": "1", "b": "3"},
			PrivateVars: map[string]bool{"b": true},
		}
		s.NoError(vars.Insert())

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
			CachedVars: []*model.ProjectVars{
				{
					Id:          projectId,
					Vars:        map[string]string{"a": "1", "b": "3"},
					PrivateVars: map[string]bool{"b": true},
				},
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
	projects, err := s.ctx.FindProjects("", 7, 1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 6)
}

func (s *ProjectConnectorGetSuite) TestFetchTooManyDesc() {
	projects, err := s.ctx.FindProjects("zzz", 7, -1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 6)
}

func (s *ProjectConnectorGetSuite) TestFetchExactNumber() {
	projects, err := s.ctx.FindProjects("", 3, 1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 3)
}

func (s *ProjectConnectorGetSuite) TestFetchTooFewAsc() {
	projects, err := s.ctx.FindProjects("", 2, 1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 2)
}

func (s *ProjectConnectorGetSuite) TestFetchTooFewDesc() {
	projects, err := s.ctx.FindProjects("zzz", 2, -1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 2)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyWithinBoundAsc() {
	projects, err := s.ctx.FindProjects("projectB", 1, 1)
	s.NoError(err)
	s.Len(projects, 1)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyWithinBoundDesc() {
	projects, err := s.ctx.FindProjects("projectD", 1, -1)
	s.NoError(err)
	s.Len(projects, 1)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyOutOfBoundAsc() {
	projects, err := s.ctx.FindProjects("zzz", 1, 1)
	s.NoError(err)
	s.Len(projects, 0)
}

func (s *ProjectConnectorGetSuite) TestFetchKeyOutOfBoundDesc() {
	projects, err := s.ctx.FindProjects("aaa", 1, -1)
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
	s.NoError(err)
	s.Nil(projRef)

	projRef, err = s.ctx.GetProjectWithCommitQueueByOwnerRepoAndBranch("evergreen-ci", "evergreen", "master")
	s.NoError(err)
	s.NotNil(projRef)
}

func (s *ProjectConnectorGetSuite) TestFindProjectVarsById() {
	res, err := s.ctx.FindProjectVarsById(projectId)
	s.NoError(err)
	s.Require().NotNil(res)
	s.Equal("1", res.Vars["a"])
	s.Equal("", res.Vars["b"])
	s.True(res.PrivateVars["b"])
}

func (s *ProjectConnectorGetSuite) TestUpdateProjectVars() {
	//successful update
	varsToDelete := []string{"a"}
	newVars := restModel.APIProjectVars{
		Vars:         map[string]string{"b": "2", "c": "3"},
		PrivateVars:  map[string]bool{"b": false, "c": true},
		VarsToDelete: varsToDelete,
	}
	s.NoError(s.ctx.UpdateProjectVars(projectId, &newVars))
	s.Equal(newVars.Vars["b"], "") // can't unredact previously redacted  variables
	s.Equal(newVars.Vars["c"], "")
	_, ok := newVars.Vars["a"]
	s.False(ok)

	s.Equal(newVars.PrivateVars["b"], true)
	s.Equal(newVars.PrivateVars["c"], true)
	_, ok = newVars.PrivateVars["a"]
	s.False(ok)

	// successful upsert
	s.NoError(s.ctx.UpdateProjectVars("not-an-id", &newVars))
}

func (s *ProjectConnectorGetSuite) TestCopyProjectVars() {
	s.NoError(s.ctx.CopyProjectVars(projectId, "project-copy"))
	origProj, err := s.ctx.FindProjectVarsById(projectId)
	s.NoError(err)

	newProj, err := s.ctx.FindProjectVarsById("project-copy")
	s.NoError(err)

	s.Equal(origProj.PrivateVars, newProj.PrivateVars)
	s.Equal(origProj.Vars, newProj.Vars)
}
