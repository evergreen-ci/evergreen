package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/stretchr/testify/suite"
)

type ProjectEventSuite struct {
	suite.Suite
}

func TestProjectEventSuite(t *testing.T) {
	s := new(ProjectEventSuite)
	suite.Run(t, s)
}

func (s *ProjectEventSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(event.EventCollection))
}

const (
	projectId = "mci2"
	username  = "me"
)

func getMockProjectSettings() ProjectSettings {
	return ProjectSettings{
		ProjectRef: ProjectRef{
			Owner:          "admin",
			Enabled:        true,
			Id:             projectId,
			Admins:         []string{},
			PeriodicBuilds: nil,
		},
		GithubHooksEnabled: true,
		Vars: ProjectVars{
			Id:            projectId,
			Vars:          map[string]string{},
			PrivateVars:   map[string]bool{},
			AdminOnlyVars: map[string]bool{},
		},
		Aliases: []ProjectAlias{{
			ID:          mgobson.ObjectIdHex("5bedc72ee4055d31f0340b1d"),
			ProjectID:   projectId,
			Alias:       "alias1",
			Variant:     "ubuntu",
			Task:        "subcommand",
			Description: "Description Here",
		},
		},
		Subscriptions: []event.Subscription{{
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
	after.ProjectRef.Enabled = false

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
	s.Equal(before.ProjectRef.Restricted, eventData.Before.ProjectRef.Restricted)
	s.Empty(before.ProjectRef.Triggers, eventData.Before.ProjectRef.Triggers)
	s.Equal(before.ProjectRef.Id, eventData.Before.ProjectRef.Id)
	s.Equal(before.ProjectRef.Admins, eventData.Before.ProjectRef.Admins)
	s.True(eventData.Before.PeriodicBuildsDefault)
	s.Equal(before.GithubHooksEnabled, eventData.Before.GithubHooksEnabled)
	s.Equal(before.Vars.Vars, eventData.Before.Vars.Vars)
	s.Equal(before.Vars.PrivateVars, eventData.Before.Vars.PrivateVars)
	s.Equal(before.Vars.AdminOnlyVars, eventData.Before.Vars.AdminOnlyVars)
	s.Equal(before.Aliases, eventData.Before.Aliases)
	s.Equal(before.Subscriptions, eventData.Before.Subscriptions)

	s.Equal(after.ProjectRef.Owner, eventData.After.ProjectRef.Owner)
	s.Equal(after.ProjectRef.Repo, eventData.After.ProjectRef.Repo)
	s.Equal(after.ProjectRef.Enabled, eventData.After.ProjectRef.Enabled)
	s.Equal(after.ProjectRef.Restricted, eventData.After.ProjectRef.Restricted)
	s.Empty(after.ProjectRef.Triggers, eventData.After.ProjectRef.Triggers)
	s.Equal(after.ProjectRef.Id, eventData.After.ProjectRef.Id)
	s.Equal(after.ProjectRef.Admins, eventData.After.ProjectRef.Admins)
	s.Equal(after.GithubHooksEnabled, eventData.After.GithubHooksEnabled)
	s.Equal(after.Vars.Vars, eventData.After.Vars.Vars)
	s.Equal(after.Vars.PrivateVars, eventData.After.Vars.PrivateVars)
	s.Equal(after.Vars.AdminOnlyVars, eventData.After.Vars.AdminOnlyVars)
	s.Equal(after.Aliases, eventData.After.Aliases)
	s.Equal(after.Subscriptions, eventData.After.Subscriptions)
}

func (s *ProjectEventSuite) TestModifyProjectEventRedactsAllVars() {
	before := getMockProjectSettings()
	before.Vars.Vars["deleted"] = "deleted_value"
	before.Vars.Vars["modified"] = "old_value"
	before.Vars.Vars["unmodified"] = "same_value"
	after := getMockProjectSettings()
	after.Vars.Vars["added"] = "added_value"
	after.Vars.Vars["modified"] = "new_value"
	after.Vars.Vars["unmodified"] = "same_value"
	after.GitHubAppAuth.AppID = 12345
	after.GitHubAppAuth.PrivateKey = []byte("secret")

	s.NoError(LogProjectModified(projectId, username, &before, &after))

	projectEvents, err := MostRecentProjectEvents(projectId, 5)
	s.NoError(err)
	s.Require().Len(projectEvents, 1)

	s.Require().NotNil(projectEvents[0].Data)
	eventData, ok := projectEvents[0].Data.(*ProjectChangeEvent)
	s.Require().True(ok)

	s.Equal(username, eventData.User)

	s.Equal(before.ProjectRef.Owner, eventData.Before.ProjectRef.Owner)
	s.Equal(before.ProjectRef.Repo, eventData.Before.ProjectRef.Repo)
	s.Equal(before.ProjectRef.Enabled, eventData.Before.ProjectRef.Enabled)
	s.Equal(before.ProjectRef.Restricted, eventData.Before.ProjectRef.Restricted)
	s.Empty(before.ProjectRef.Triggers, eventData.Before.ProjectRef.Triggers)
	s.Equal(before.ProjectRef.Id, eventData.Before.ProjectRef.Id)
	s.Equal(before.ProjectRef.Admins, eventData.Before.ProjectRef.Admins)
	s.True(eventData.Before.PeriodicBuildsDefault)
	s.Equal(before.GithubHooksEnabled, eventData.Before.GithubHooksEnabled)
	s.Len(eventData.Before.Vars.Vars, len(before.Vars.Vars))
	s.Equal(evergreen.RedactedBeforeValue, eventData.Before.Vars.Vars["deleted"], "deleted var should be redacted")
	s.Equal(evergreen.RedactedBeforeValue, eventData.Before.Vars.Vars["modified"], "modified var should be redacted")
	s.Empty(eventData.Before.Vars.Vars["unmodified"], "unmodified var should be present but empty to indicate it wasn't changed")
	s.Empty(eventData.Before.GitHubAppAuth.AppID)
	s.Empty(eventData.Before.GitHubAppAuth.PrivateKey)
	s.Equal(before.Aliases, eventData.Before.Aliases)
	s.Equal(before.Subscriptions, eventData.Before.Subscriptions)

	s.Equal(after.ProjectRef.Owner, eventData.After.ProjectRef.Owner)
	s.Equal(after.ProjectRef.Repo, eventData.After.ProjectRef.Repo)
	s.Equal(after.ProjectRef.Enabled, eventData.After.ProjectRef.Enabled)
	s.Equal(after.ProjectRef.Restricted, eventData.After.ProjectRef.Restricted)
	s.Empty(after.ProjectRef.Triggers, eventData.After.ProjectRef.Triggers)
	s.Equal(after.ProjectRef.Id, eventData.After.ProjectRef.Id)
	s.Equal(after.ProjectRef.Admins, eventData.After.ProjectRef.Admins)
	s.Equal(after.GithubHooksEnabled, eventData.After.GithubHooksEnabled)
	s.Len(eventData.After.Vars.Vars, len(after.Vars.Vars))
	s.Equal(evergreen.RedactedAfterValue, eventData.After.Vars.Vars["added"], "newly-added var should be redacted")
	s.Equal(evergreen.RedactedAfterValue, eventData.After.Vars.Vars["modified"], "modified var should be redacted")
	s.Empty(eventData.After.Vars.Vars["unmodified"], "unmodified var should be present but empty to indicate it wasn't changed")
	s.Equal(after.GitHubAppAuth.AppID, eventData.After.GitHubAppAuth.AppID)
	s.Equal(evergreen.RedactedAfterValue, string(eventData.After.GitHubAppAuth.PrivateKey))
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
