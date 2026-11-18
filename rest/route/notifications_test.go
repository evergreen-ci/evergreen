package route

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
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

func TestRetryFailedWebhooksHandlerParse(t *testing.T) {
	for tName, tCase := range map[string]struct {
		body         string
		expectedDry  bool
		expectsError bool
	}{
		"MissingBodyUsesDefaults":      {},
		"MissingDryRunDefaultsToFalse": {body: `{}`},
		"DryRunTrue":                   {body: `{"dry_run": true}`, expectedDry: true},
		"MalformedBodyIsRejected":      {body: `{`, expectsError: true},
		"OversizedBodyIsRejected":      {body: `{"dry_run": true` + strings.Repeat(" ", maxRetryFailedWebhooksBodySize) + `}`, expectsError: true},
	} {
		t.Run(tName, func(t *testing.T) {
			h := &retryFailedWebhooksHandler{}
			req, err := http.NewRequest(http.MethodPost, "https://example.com/projects/evergreen/retry_failed_webhooks", strings.NewReader(tCase.body))
			require.NoError(t, err)
			err = h.Parse(t.Context(), req)
			if tCase.expectsError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tCase.expectedDry, h.opts.DryRun)
		})
	}
}

func TestRetryFailedWebhooksHandlerRun(t *testing.T) {
	require.NoError(t, db.ClearCollections(notification.FailedWebhookCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(notification.FailedWebhookCollection))
	})

	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))

	require.NoError(t, db.Insert(t.Context(), notification.FailedWebhookCollection, &notification.FailedWebhook{
		NotificationID: "webhook-1",
		Project:        "evergreen",
	}))
	// Repo-level subscriptions tag failures with the repo ref ID.
	require.NoError(t, db.Insert(t.Context(), notification.FailedWebhookCollection, &notification.FailedWebhook{
		NotificationID: "webhook-repo",
		Project:        "repo",
	}))

	ctx := context.WithValue(t.Context(), RequestContext, &dbModel.Context{
		ProjectRef: &dbModel.ProjectRef{Id: "evergreen", RepoRefId: "repo"},
	})

	t.Run("DryRunReturnsFailedWebhooksWithoutRetrying", func(t *testing.T) {
		h := &retryFailedWebhooksHandler{env: env, opts: retryFailedWebhooksOptions{DryRun: true}}
		resp := h.Run(ctx)
		require.Equal(t, http.StatusOK, resp.Status())

		failedWebhooks, ok := resp.Data().([]model.APIFailedWebhook)
		require.True(t, ok)
		require.Len(t, failedWebhooks, 2)
		assert.Zero(t, env.RemoteQueue().Stats(ctx).Total)
	})

	t.Run("RetriesFailedWebhooks", func(t *testing.T) {
		h := &retryFailedWebhooksHandler{env: env}
		resp := h.Run(ctx)
		require.Equal(t, http.StatusOK, resp.Status())

		failedWebhooks, ok := resp.Data().([]model.APIFailedWebhook)
		require.True(t, ok)
		require.Len(t, failedWebhooks, 2)
		assert.Equal(t, 2, env.RemoteQueue().Stats(ctx).Total)
	})

	t.Run("RetryJobIsKeyedOnFailureTime", func(t *testing.T) {
		failedAt := time.Now().Truncate(time.Millisecond)
		require.NoError(t, db.Insert(t.Context(), notification.FailedWebhookCollection, &notification.FailedWebhook{
			NotificationID: "webhook-refailed",
			Project:        "evergreen",
			FailedAt:       failedAt,
		}))
		h := &retryFailedWebhooksHandler{env: env}
		require.Equal(t, http.StatusOK, h.Run(ctx).Status())
		_, found := env.RemoteQueue().Get(ctx, units.NewRetryEventSendJob("webhook-refailed", strconv.FormatInt(failedAt.UnixMilli(), 10)).ID())
		assert.True(t, found)
	})
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
