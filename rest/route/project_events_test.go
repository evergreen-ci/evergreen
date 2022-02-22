package route

import (
	"context"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/testutil"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

type ProjectEventsTestSuite struct {
	suite.Suite
	sc        *data.DBConnector
	data      data.DBProjectConnector
	route     projectEventsGet
	projectId string
	event     model.ProjectChangeEvent
}

func TestProjectEventsTestSuite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	suite.Run(t, new(ProjectEventsTestSuite))
}

func getMockProjectSettings(projectId string) model.ProjectSettings {
	return model.ProjectSettings{
		ProjectRef: model.ProjectRef{
			Owner:      "admin",
			Enabled:    utility.TruePtr(),
			Private:    utility.TruePtr(),
			Identifier: projectId,
			Admins:     []string{},
		},
		Vars: model.ProjectVars{
			Vars:        map[string]string{},
			PrivateVars: map[string]bool{},
		},
		Aliases: []model.ProjectAlias{model.ProjectAlias{
			Alias:   "alias1",
			Variant: "ubuntu",
			Task:    "subcommand",
		},
		},
		Subscriptions: []event.Subscription{event.Subscription{
			ID:           "subscription1",
			ResourceType: "project",
			Owner:        "admin",
			Subscriber: event.Subscriber{
				Type:   event.GithubPullRequestSubscriberType,
				Target: restModel.APIGithubPRSubscriber{},
			},
		},
		},
	}
}

func (s *ProjectEventsTestSuite) SetupSuite() {
	s.projectId = "mci2"
	beforeSettings := getMockProjectSettings(s.projectId)

	afterSettings := getMockProjectSettings(s.projectId)
	afterSettings.ProjectRef.Enabled = utility.FalsePtr()

	s.event = model.ProjectChangeEvent{
		User:   "me",
		Before: beforeSettings,
		After:  afterSettings,
	}

	s.NoError(db.ClearCollections(event.AllLogCollection, model.ProjectRefCollection))

	s.data = data.DBProjectConnector{}

	s.sc = &data.DBConnector{
		URL:                "https://evergreen.example.net",
		DBProjectConnector: s.data,
	}

	projectRef := &model.ProjectRef{
		Id:      "mci2",
		Enabled: utility.TruePtr(),
	}
	s.NoError(projectRef.Insert())

	s.NoError(model.LogProjectEvent("PROJECT_ADDED", "mci2", s.event))
}

func (s *ProjectEventsTestSuite) TestGetProjectEvents() {
	s.route.Id = s.projectId
	s.route.Limit = 100
	s.route.Timestamp = time.Now().Add(time.Second * 10)
	s.route.sc = s.sc

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	responseData, ok := resp.Data().([]interface{})
	s.Require().True(ok)
	apiEvent := responseData[0].(*restModel.APIProjectEvent)
	s.Equal(s.event.Before.ProjectRef.Identifier, *apiEvent.Before.ProjectRef.Identifier)
	s.Equal(s.event.Before.Aliases[0].Alias, *apiEvent.Before.Aliases[0].Alias)
	s.Equal(s.event.Before.Subscriptions[0].ID, *apiEvent.Before.Subscriptions[0].ID)
	s.Equal(s.event.After.ProjectRef.Identifier, *apiEvent.After.ProjectRef.Identifier)
	s.Equal(s.event.After.Aliases[0].Alias, *apiEvent.After.Aliases[0].Alias)
	s.Equal(s.event.After.Subscriptions[0].ID, *apiEvent.After.Subscriptions[0].ID)
}
