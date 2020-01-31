package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
)

type notificationSuite struct {
	suite.Suite
	expectedTime time.Time
}

func TestNotificationSuite(t *testing.T) {
	suite.Run(t, &notificationSuite{})
}

func (s *notificationSuite) SetupSuite() {

}

func (s *notificationSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, notification.Collection))
	s.expectedTime = time.Now().Add(-time.Hour).Round(0).Truncate(time.Millisecond)

	events := []event.EventLogEntry{
		{
			ID:           mgobson.NewObjectId().Hex(),
			ResourceType: event.ResourceTypeHost,
			Data:         event.HostEventData{},
		},
		{
			ID:           mgobson.NewObjectId().Hex(),
			ResourceType: event.ResourceTypeHost,
			Data:         event.HostEventData{},
		},
		{
			ID:           mgobson.NewObjectId().Hex(),
			ResourceType: event.ResourceTypeHost,
			Data:         event.HostEventData{},
			ProcessedAt:  s.expectedTime,
		},
	}

	for i := range events {
		s.NoError(db.Insert(event.AllLogCollection, events[i]))
	}

	n := []notification.Notification{
		{
			ID: "0",
			Subscriber: event.Subscriber{
				Type: event.EmailSubscriberType,
			},
		},
		{
			ID: "1",
			Subscriber: event.Subscriber{
				Type: event.SlackSubscriberType,
			},
		},
		{
			ID: "2",
			Subscriber: event.Subscriber{
				Type: event.JIRAIssueSubscriberType,
			},
		},
		{
			ID: "3",
			Subscriber: event.Subscriber{
				Type: event.JIRACommentSubscriberType,
			},
		},
		{
			ID: "4",
			Subscriber: event.Subscriber{
				Type: event.GithubPullRequestSubscriberType,
			},
		},
		{
			ID: "5",
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
			},
		},
		{
			ID: "6",
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
			},
			SentAt: time.Now(),
		},
	}
	for i := range n {
		s.NoError(db.Insert(notification.Collection, n[i]))
	}
}

func (s *notificationSuite) TestStatsCollector() {
	sc := &data.DBConnector{}
	h := notificationsStatusHandler{sc: sc}

	resp := h.Run(context.Background())
	s.Equal(http.StatusOK, resp.Status())

	stats := resp.Data().(*model.APIEventStats)

	s.Equal(s.expectedTime, *stats.LastProcessedAt)
	s.Equal(2, stats.NumUnprocessedEvents)
	s.NotEmpty(stats.PendingNotificationsByType)
	s.Equal(1, stats.PendingNotificationsByType.Email)
	s.Equal(1, stats.PendingNotificationsByType.EvergreenWebhook)
	s.Equal(1, stats.PendingNotificationsByType.JIRAComment)
	s.Equal(1, stats.PendingNotificationsByType.JIRAIssue)
	s.Equal(1, stats.PendingNotificationsByType.Slack)
	s.Equal(1, stats.PendingNotificationsByType.GithubPullRequest)
}
