package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
)

type ProjectEventSuite struct {
	suite.Suite
}

func TestProjectEventSuite(t *testing.T) {
	s := new(ProjectEventSuite)
	suite.Run(t, s)
}

func (s *ProjectEventSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(event.AllLogCollection))
}

const (
	projectId = "mci2"
	username  = "me"
)

func getMockProjectSettings() ProjectSettings {
	return ProjectSettings{
		ProjectRef: ProjectRef{
			Owner:   "admin",
			Enabled: utility.TruePtr(),
			Private: utility.TruePtr(),
			Id:      projectId,
			Admins:  []string{},
		},
		GitHubHooksEnabled: true,
		Vars: ProjectVars{
			Id:             projectId,
			Vars:           map[string]string{},
			PrivateVars:    map[string]bool{},
			RestrictedVars: map[string]bool{},
		},
		Aliases: []ProjectAlias{ProjectAlias{
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
				Target: &event.GithubPullRequestSubscriber{},
			},
		},
		},
	}
}

func (s *ProjectEventSuite) TestModifyProjectEvent() {
	before := getMockProjectSettings()
	after := getMockProjectSettings()
	after.ProjectRef.Enabled = utility.FalsePtr()

	s.NoError(LogProjectModified(projectId, username, &before, &after))

	projectEvents, err := MostRecentProjectEvents(projectId, 5)
	s.NoError(err)
	s.Require().Len(projectEvents, 1)

	s.Require().NotNil(projectEvents[0].Data)
	eventData := projectEvents[0].Data.(*ProjectChangeEvent)

	s.Equal(username, eventData.User)

	s.Equal(before.ProjectRef.Owner, eventData.Before.ProjectRef.Owner)
	s.Equal(before.ProjectRef.Repo, eventData.Before.ProjectRef.Repo)
	s.Equal(before.ProjectRef.Enabled, eventData.Before.ProjectRef.Enabled)
	s.Equal(before.ProjectRef.Private, eventData.Before.ProjectRef.Private)
	s.Equal(before.ProjectRef.Restricted, eventData.Before.ProjectRef.Restricted)
	s.Empty(before.ProjectRef.Triggers, eventData.Before.ProjectRef.Triggers)
	s.Equal(before.ProjectRef.Id, eventData.Before.ProjectRef.Id)
	s.Equal(before.ProjectRef.Admins, eventData.Before.ProjectRef.Admins)
	s.Equal(before.GitHubHooksEnabled, eventData.Before.GitHubHooksEnabled)
	s.Equal(before.Vars, eventData.Before.Vars)
	s.Equal(before.Aliases, eventData.Before.Aliases)
	s.Equal(before.Subscriptions, eventData.Before.Subscriptions)

	s.Equal(after.ProjectRef.Owner, eventData.After.ProjectRef.Owner)
	s.Equal(after.ProjectRef.Repo, eventData.After.ProjectRef.Repo)
	s.Equal(after.ProjectRef.Enabled, eventData.After.ProjectRef.Enabled)
	s.Equal(after.ProjectRef.Private, eventData.After.ProjectRef.Private)
	s.Equal(after.ProjectRef.Restricted, eventData.After.ProjectRef.Restricted)
	s.Empty(after.ProjectRef.Triggers, eventData.After.ProjectRef.Triggers)
	s.Equal(after.ProjectRef.Id, eventData.After.ProjectRef.Id)
	s.Equal(after.ProjectRef.Admins, eventData.After.ProjectRef.Admins)
	s.Equal(after.GitHubHooksEnabled, eventData.After.GitHubHooksEnabled)
	s.Equal(after.Vars, eventData.After.Vars)
	s.Equal(after.Aliases, eventData.After.Aliases)
	s.Equal(after.Subscriptions, eventData.After.Subscriptions)
}

func (s *ProjectEventSuite) TestModifyProjectNonEvent() {
	before := getMockProjectSettings()
	after := getMockProjectSettings()

	s.NoError(LogProjectModified(projectId, username, &before, &after))

	projectEvents, err := MostRecentProjectEvents(projectId, 5)
	s.NoError(err)
	s.Require().Len(projectEvents, 0)
}

func (s *ProjectEventSuite) TestAddProject() {
	s.NoError(LogProjectAdded(projectId, username))

	projectEvents, err := MostRecentProjectEvents(projectId, 5)
	s.NoError(err)

	s.Require().Len(projectEvents, 1)
	s.Equal(projectId, projectEvents[0].ResourceId)

	s.Require().NotNil(projectEvents[0].Data)
	eventData := projectEvents[0].Data.(*ProjectChangeEvent)
	s.Equal(username, eventData.User)

}
