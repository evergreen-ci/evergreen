package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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
	s.NoError(db.ClearCollections(event.EventCollection, notification.Collection))
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
		s.NoError(db.Insert(s.T().Context(), event.EventCollection, events[i]))
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
		s.NoError(db.Insert(s.T().Context(), notification.Collection, n[i]))
	}
}

func (s *notificationSuite) TestStatsCollector() {
	h := notificationsStatusHandler{}

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

func TestFailedWebhookNotificationsHandlerParse(t *testing.T) {
	require.NoError(t, db.ClearCollections(dbModel.ProjectRefCollection))
	t.Cleanup(func() { assert.NoError(t, db.Clear(dbModel.ProjectRefCollection)) })
	for _, ref := range []dbModel.ProjectRef{
		{Id: "evergreen"},
		{Id: "spruce"},
		{Id: "mongo-master", Identifier: "mongodb-mongo-master"},
	} {
		require.NoError(t, ref.Insert(t.Context()))
	}

	parse := func(t *testing.T, query string) *failedWebhookNotificationsHandler {
		h := &failedWebhookNotificationsHandler{}
		req, err := http.NewRequest(http.MethodGet, "https://example.com/failed_webhook_notifications?"+query, nil)
		require.NoError(t, err)
		require.NoError(t, h.Parse(t.Context(), req))
		return h
	}

	t.Run("MissingProjectsIsRejected", func(t *testing.T) {
		h := &failedWebhookNotificationsHandler{}
		req, err := http.NewRequest(http.MethodGet, "https://example.com/failed_webhook_notifications", nil)
		require.NoError(t, err)
		err = h.Parse(t.Context(), req)
		assert.Error(t, err)
	})

	t.Run("ValidParametersAreParsed", func(t *testing.T) {
		h := parse(t, "url=https://example.com/hook&projects=evergreen,spruce&limit=5")
		assert.Equal(t, "https://example.com/hook", h.url)
		assert.Equal(t, []string{"evergreen", "spruce"}, h.projects)
		assert.Equal(t, 5, h.limit)
	})

	t.Run("IdentifiersResolveToProjectIDs", func(t *testing.T) {
		h := parse(t, "projects=mongodb-mongo-master")
		assert.Equal(t, []string{"mongo-master"}, h.projects)
	})

	t.Run("UnknownProjectIsRejected", func(t *testing.T) {
		h := &failedWebhookNotificationsHandler{}
		req, err := http.NewRequest(http.MethodGet, "https://example.com/failed_webhook_notifications?projects=nonexistent", nil)
		require.NoError(t, err)
		err = h.Parse(t.Context(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("LimitDefaultsAndCaps", func(t *testing.T) {
		h := parse(t, "projects=evergreen")
		assert.Equal(t, defaultFailedWebhookNotificationsLimit, h.limit)

		h = parse(t, "projects=evergreen&limit=10000")
		assert.Equal(t, maxFailedWebhookNotificationsLimit, h.limit)
	})

	t.Run("InvalidLimitIsRejected", func(t *testing.T) {
		h := &failedWebhookNotificationsHandler{}
		req, err := http.NewRequest(http.MethodGet, "https://example.com/failed_webhook_notifications?projects=evergreen&limit=zero", nil)
		require.NoError(t, err)
		assert.Error(t, h.Parse(t.Context(), req))
	})
}

func TestFailedWebhookNotificationsHandlerRun(t *testing.T) {
	require.NoError(t, db.Clear(notification.FailedWebhookCollection))
	t.Cleanup(func() { assert.NoError(t, db.Clear(notification.FailedWebhookCollection)) })

	require.NoError(t, db.Insert(t.Context(), notification.FailedWebhookCollection, &notification.FailedWebhook{
		NotificationID: "webhook-1",
		Project:        "evergreen",
		URL:            "https://example.com/hook",
		Error:          "webhook response was 404 (Not Found)",
		SentAt:         time.Now().Truncate(time.Millisecond),
	}))

	t.Run("ForbiddenWithoutProjectPermission", func(t *testing.T) {
		ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "user"})
		h := &failedWebhookNotificationsHandler{projects: []string{"evergreen"}, limit: 10}
		resp := h.Run(ctx)
		assert.Equal(t, http.StatusForbidden, resp.Status())
	})

	t.Run("ReturnsFailedWebhooksForPermittedProjects", func(t *testing.T) {
		testutil.DisablePermissionsForTests()
		t.Cleanup(testutil.EnablePermissionsForTests)

		ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "user"})
		h := &failedWebhookNotificationsHandler{projects: []string{"evergreen"}, url: "https://example.com/hook", limit: 10}
		resp := h.Run(ctx)
		require.Equal(t, http.StatusOK, resp.Status())

		failedWebhooks, ok := resp.Data().([]model.APIFailedWebhook)
		require.True(t, ok)
		require.Len(t, failedWebhooks, 1)
		assert.Equal(t, "webhook-1", *failedWebhooks[0].NotificationID)
		assert.Equal(t, "evergreen", *failedWebhooks[0].Project)
		assert.Equal(t, "https://example.com/hook", *failedWebhooks[0].URL)
		assert.Equal(t, "webhook response was 404 (Not Found)", *failedWebhooks[0].Error)
	})
}
