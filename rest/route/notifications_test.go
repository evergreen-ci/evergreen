package route

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type notificationSuite struct {
	suite.Suite
	expectedTime time.Time
}

func TestNotificationSuite(t *testing.T) {
	suite.Run(t, &notificationSuite{})
}

func (s *notificationSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *notificationSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, notification.NotificationsCollection))
	s.expectedTime = time.Now().Add(-time.Hour).Round(0).Truncate(time.Millisecond)

	events := []event.EventLogEntry{
		{
			ID:           bson.NewObjectId(),
			ResourceType: event.ResourceTypeHost,
			Data:         event.HostEventData{},
		},
		{
			ID:           bson.NewObjectId(),
			ResourceType: event.ResourceTypeHost,
			Data:         event.HostEventData{},
		},
		{
			ID:           bson.NewObjectId(),
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
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.EmailSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.SlackSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.JIRAIssueSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.JIRACommentSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.GithubPullRequestSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
			},
			SentAt: time.Now(),
		},
	}
	for i := range n {
		s.NoError(db.Insert(notification.NotificationsCollection, n[i]))
	}
}

func (s *notificationSuite) TestStatsCollector() {
	h := notificationsStatusHandler{}
	sc := &data.DBConnector{}

	resp, err := h.Execute(context.Background(), sc)
	s.NoError(err)
	s.Require().Len(resp.Result, 1)

	stats := resp.Result[0].(*model.APIEventStats)

	s.Equal(s.expectedTime, stats.LastProcessedAt)
	s.Equal(2, stats.NumUnprocessedEvents)
	s.NotEmpty(stats.PendingNotificationsByType)
	s.Equal(1, stats.PendingNotificationsByType.Email)
	s.Equal(1, stats.PendingNotificationsByType.EvergreenWebhook)
	s.Equal(1, stats.PendingNotificationsByType.JIRAComment)
	s.Equal(1, stats.PendingNotificationsByType.JIRAIssue)
	s.Equal(1, stats.PendingNotificationsByType.Slack)
	s.Equal(1, stats.PendingNotificationsByType.GithubPullRequest)
}
