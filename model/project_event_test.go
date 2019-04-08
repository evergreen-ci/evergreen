package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
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

func getMockProjectSettings() ProjectSettingsEvent {
	return ProjectSettingsEvent{
		ProjectRef: ProjectRef{
			Owner:      "admin",
			Enabled:    true,
			Private:    true,
			Identifier: projectId,
			Admins:     []string{},
		},
		GitHubHooksEnabled: true,
		Vars: ProjectVars{
			Id:          projectId,
			Vars:        map[string]string{},
			PrivateVars: map[string]bool{},
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
	after.ProjectRef.Enabled = false

	s.NoError(LogProjectModified(projectId, username, before, after))

	projectEvents, err := MostRecentProjectEvents(projectId, 5)
	s.NoError(err)
	s.Require().Len(projectEvents, 1)

	s.Require().NotNil(projectEvents[0].Data)
	eventData := projectEvents[0].Data.(*ProjectChangeEvent)

	s.Equal(username, eventData.User)
	s.Equal(before, eventData.Before)
	s.Equal(after, eventData.After)
}

func (s *ProjectEventSuite) TestModifyProjectNonEvent() {
	before := getMockProjectSettings()
	after := getMockProjectSettings()

	s.NoError(LogProjectModified(projectId, username, before, after))

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
