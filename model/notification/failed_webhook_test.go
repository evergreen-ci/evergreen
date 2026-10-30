package notification

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeFailedWebhookNotification(id string) *Notification {
	return &Notification{
		ID: id,
		Subscriber: event.Subscriber{
			Type: event.EvergreenWebhookSubscriberType,
			Target: &event.WebhookSubscriber{
				URL: "https://example.com/hook",
				Headers: []event.WebhookHeader{
					{Key: "X-Test", Value: "value"},
					{Key: event.WebhookAuthorizationHeader, Value: "super-secret-credential"},
				},
			},
		},
		Payload: &util.EvergreenWebhook{
			Body: []byte(`{"notification":"failed"}`),
		},
		Project: "evergreen",
	}
}

func TestInsertFailedWebhook(t *testing.T) {
	require.NoError(t, db.Clear(FailedWebhookCollection))
	t.Cleanup(func() { assert.NoError(t, db.Clear(FailedWebhookCollection)) })

	t.Run("NonWebhookNotificationIsIgnored", func(t *testing.T) {
		n := &Notification{
			ID:         "jira",
			Subscriber: event.Subscriber{Type: event.JIRAIssueSubscriberType},
		}
		require.NoError(t, InsertFailedWebhook(t.Context(), n, errors.New("delivery failed")))
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"}, "", 10)
		require.NoError(t, err)
		assert.Empty(t, failedWebhooks)
	})

	t.Run("NilErrorIsIgnored", func(t *testing.T) {
		n := makeFailedWebhookNotification("no-error")
		require.NoError(t, InsertFailedWebhook(t.Context(), n, nil))
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"}, "", 10)
		require.NoError(t, err)
		assert.Empty(t, failedWebhooks)
	})

	t.Run("WebhookNotificationIsRecorded", func(t *testing.T) {
		n := makeFailedWebhookNotification("webhook-1")
		require.NoError(t, InsertFailedWebhook(t.Context(), n, errors.New("webhook response was 404 (Not Found)")))

		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"}, "", 10)
		require.NoError(t, err)
		require.Len(t, failedWebhooks, 1)
		failedWebhook := failedWebhooks[0]
		assert.Equal(t, "webhook-1", failedWebhook.NotificationID)
		assert.Equal(t, "evergreen", failedWebhook.Project)
		assert.Equal(t, "https://example.com/hook", failedWebhook.URL)
		assert.Equal(t, "webhook response was 404 (Not Found)", failedWebhook.Error)
		assert.Equal(t, []byte(`{"notification":"failed"}`), failedWebhook.Body)
		assert.Equal(t, "value", failedWebhook.Headers.Get("X-Test"))
		// The Authorization header carries a credential and must not be
		// retained.
		assert.Empty(t, failedWebhook.Headers.Get(event.WebhookAuthorizationHeader))
		assert.NotZero(t, failedWebhook.SentAt)
	})

	t.Run("NonProjectNotificationIsIgnored", func(t *testing.T) {
		require.NoError(t, db.Clear(FailedWebhookCollection))
		n := makeFailedWebhookNotification("no-project")
		n.Project = ""
		require.NoError(t, InsertFailedWebhook(t.Context(), n, errors.New("webhook response was 404 (Not Found)")))
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"}, "", 10)
		require.NoError(t, err)
		assert.Empty(t, failedWebhooks)
	})

	t.Run("DuplicateInsertIsANoop", func(t *testing.T) {
		require.NoError(t, db.Clear(FailedWebhookCollection))
		n := makeFailedWebhookNotification("webhook-dup")
		require.NoError(t, InsertFailedWebhook(t.Context(), n, errors.New("webhook response was 404 (Not Found)")))
		require.NoError(t, InsertFailedWebhook(t.Context(), n, errors.New("webhook response was 404 (Not Found)")))

		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"}, "", 10)
		require.NoError(t, err)
		assert.Len(t, failedWebhooks, 1)
	})
}

func TestFindFailedWebhooks(t *testing.T) {
	require.NoError(t, db.Clear(FailedWebhookCollection))
	t.Cleanup(func() { assert.NoError(t, db.Clear(FailedWebhookCollection)) })

	now := time.Now().Truncate(time.Millisecond)
	for _, failure := range []FailedWebhook{
		{
			NotificationID: "webhook-old",
			Project:        "evergreen",
			URL:            "https://example.com/hook",
			Error:          "old failure",
			SentAt:         now.Add(-2 * time.Hour),
		},
		{
			NotificationID: "webhook-new",
			Project:        "evergreen",
			URL:            "https://example.com/other-hook",
			Error:          "new failure",
			SentAt:         now.Add(-1 * time.Hour),
		},
		{
			NotificationID: "webhook-other-project",
			Project:        "spruce",
			URL:            "https://example.com/hook",
			Error:          "other project failure",
			SentAt:         now,
		},
	} {
		require.NoError(t, db.Insert(t.Context(), FailedWebhookCollection, &failure))
	}

	t.Run("FiltersByProject", func(t *testing.T) {
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"}, "", 10)
		require.NoError(t, err)
		require.Len(t, failedWebhooks, 2)
		// Sorted newest first.
		assert.Equal(t, "webhook-new", failedWebhooks[0].NotificationID)
		assert.Equal(t, "webhook-old", failedWebhooks[1].NotificationID)
	})

	t.Run("FiltersByProjectAndURL", func(t *testing.T) {
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen", "spruce"}, "https://example.com/hook", 10)
		require.NoError(t, err)
		require.Len(t, failedWebhooks, 2)
		assert.Equal(t, "webhook-other-project", failedWebhooks[0].NotificationID)
		assert.Equal(t, "webhook-old", failedWebhooks[1].NotificationID)
	})

	t.Run("LimitsResults", func(t *testing.T) {
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen", "spruce"}, "", 1)
		require.NoError(t, err)
		require.Len(t, failedWebhooks, 1)
		assert.Equal(t, "webhook-other-project", failedWebhooks[0].NotificationID)
	})

	t.Run("UnknownProjectReturnsNothing", func(t *testing.T) {
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"unknown-project"}, "", 10)
		require.NoError(t, err)
		assert.Empty(t, failedWebhooks)
	})
}
