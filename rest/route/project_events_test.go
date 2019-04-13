package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

type ProjectEventsTestSuite struct {
	suite.Suite
	sc        *data.MockConnector
	data      data.MockProjectConnector
	route     projectEventsGet
	projectId string
	event     restModel.APIProjectEvent
}

func TestProjectEventsTestSuite(t *testing.T) {

	suite.Run(t, new(ProjectEventsTestSuite))
}

func getMockProjectSettings(projectId string) restModel.APIProjectSettings {
	return restModel.APIProjectSettings{
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
}

func (s *ProjectEventsTestSuite) SetupSuite() {
	s.projectId = "mci2"
	beforeSettings := getMockProjectSettings(s.projectId)

	afterSettings := getMockProjectSettings(s.projectId)
	afterSettings.ProjectRef.Enabled = false

	s.event = restModel.APIProjectEvent{
		Timestamp: time.Now(),
		User:      restModel.ToAPIString("me"),
		Before:    beforeSettings,
		After:     afterSettings,
	}

	s.data = data.MockProjectConnector{
		CachedEvents: []restModel.APIProjectEvent{s.event},
	}

	s.sc = &data.MockConnector{
		URL:                  "https://evergreen.example.net",
		MockProjectConnector: s.data,
	}
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
	s.Equal(&s.event, responseData[0])
}
