package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type ProjectEventSuite struct {
	suite.Suite
}

func TestProjectEventSuite(t *testing.T) {
	s := new(ProjectEventSuite)
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	suite.Run(t, s)
}

func (s *ProjectEventSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(event.AllLogCollection))
}

const (
	projectId = "mci2"
	username  = "me"
)

var sampleProjectSettings = ProjectSettings{
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

func (s *ProjectEventSuite) TestModifyProjectEvent() {
	before := sampleProjectSettings
	after := before
	after.ProjectRef.Enabled = false

	s.NoError(LogProjectModified(projectId, username, before, after))

	projectEvents := []ProjectChangeEvent{}
	query := ProjectEventsForId(projectId)
	s.NoError(db.FindAllQ(event.AllLogCollection, query, &projectEvents))
	s.Require().Len(projectEvents, 1)

	eventData := projectEvents[0].Data.(*ProjectChange)

	s.Equal(username, eventData.User)
	s.Equal(before, eventData.Before)
	s.Equal(after, eventData.After)
}

func (s *ProjectEventSuite) TestModifyProjectNonEvent() {
	before := sampleProjectSettings
	after := before

	s.NoError(LogProjectModified(projectId, username, before, after))

	projectEvents := []ProjectChangeEvent{}
	query := ProjectEventsForId(projectId)
	s.NoError(db.FindAllQ(event.AllLogCollection, query, &projectEvents))
	s.Require().Len(projectEvents, 0)
}

func (s *ProjectEventSuite) TestAddProject() {
	s.NoError(LogProjectAdded(projectId, username))

	projectEvents := []ProjectChangeEvent{}
	query := ProjectEventsForId(projectId)
	s.NoError(db.FindAllQ(event.AllLogCollection, query, &projectEvents))

	s.Require().Len(projectEvents, 1)
	s.Equal(projectId, projectEvents[0].ResourceId)

	eventData := projectEvents[0].Data.(*ProjectChange)
	s.Equal(username, eventData.User)

}
