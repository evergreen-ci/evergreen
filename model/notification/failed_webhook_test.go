package notification

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func makeFailedWebhookNotification(id string) *Notification {
	return &Notification{
		ID: id,
		Subscriber: event.Subscriber{
			Type: event.EvergreenWebhookSubscriberType,
			Target: &event.WebhookSubscriber{
				URL: "https://example.com/hook",
			},
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
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"})
		require.NoError(t, err)
		assert.Empty(t, failedWebhooks)
	})

	t.Run("NilErrorIsIgnored", func(t *testing.T) {
		n := makeFailedWebhookNotification("no-error")
		require.NoError(t, InsertFailedWebhook(t.Context(), n, nil))
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"})
		require.NoError(t, err)
		assert.Empty(t, failedWebhooks)
	})

	t.Run("WebhookNotificationIsRecorded", func(t *testing.T) {
		n := makeFailedWebhookNotification("webhook-1")
		require.NoError(t, InsertFailedWebhook(t.Context(), n, errors.New("webhook response was 404 (Not Found)")))

		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"})
		require.NoError(t, err)
		require.Len(t, failedWebhooks, 1)
		failedWebhook := failedWebhooks[0]
		assert.Equal(t, "webhook-1", failedWebhook.NotificationID)
		assert.Equal(t, "evergreen", failedWebhook.Project)
		assert.Equal(t, "webhook response was 404 (Not Found)", failedWebhook.Error)
		assert.NotZero(t, failedWebhook.FailedAt)
	})

	t.Run("NonProjectNotificationIsIgnored", func(t *testing.T) {
		require.NoError(t, db.Clear(FailedWebhookCollection))
		n := makeFailedWebhookNotification("no-project")
		n.Project = ""
		require.NoError(t, InsertFailedWebhook(t.Context(), n, errors.New("webhook response was 404 (Not Found)")))
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"})
		require.NoError(t, err)
		assert.Empty(t, failedWebhooks)
	})

	t.Run("RefailedRetriedWebhookIsRecordedAgain", func(t *testing.T) {
		require.NoError(t, db.Clear(FailedWebhookCollection))
		n := makeFailedWebhookNotification("webhook-dup")
		require.NoError(t, InsertFailedWebhook(t.Context(), n, errors.New("first failure")))
		claimed, err := MarkFailedWebhookRetried(t.Context(), n.ID)
		require.NoError(t, err)
		require.True(t, claimed)
		require.NoError(t, InsertFailedWebhook(t.Context(), n, errors.New("second failure")))

		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"})
		require.NoError(t, err)
		require.Len(t, failedWebhooks, 1)
		assert.Equal(t, "second failure", failedWebhooks[0].Error)
	})
}

func TestMarkFailedWebhookRetried(t *testing.T) {
	require.NoError(t, db.Clear(FailedWebhookCollection))
	t.Cleanup(func() { assert.NoError(t, db.Clear(FailedWebhookCollection)) })

	n := makeFailedWebhookNotification("webhook-retried")
	require.NoError(t, InsertFailedWebhook(t.Context(), n, errors.New("delivery failed")))

	claimed, err := MarkFailedWebhookRetried(t.Context(), n.ID)
	require.NoError(t, err)
	assert.True(t, claimed)

	claimed, err = MarkFailedWebhookRetried(t.Context(), n.ID)
	require.NoError(t, err)
	assert.False(t, claimed)

	failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"})
	require.NoError(t, err)
	assert.Empty(t, failedWebhooks)
}

func TestMarkFailedWebhookRetriedExpiredClaimIsReclaimable(t *testing.T) {
	require.NoError(t, db.Clear(FailedWebhookCollection))
	t.Cleanup(func() { assert.NoError(t, db.Clear(FailedWebhookCollection)) })

	require.NoError(t, db.Insert(t.Context(), FailedWebhookCollection, &FailedWebhook{
		NotificationID: "webhook-stale-claim",
		Project:        "evergreen",
		FailedAt:       time.Now().Add(-time.Hour),
		RetriedAt:      time.Now().Add(-failedWebhookRetryLease - time.Minute),
	}))

	failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"})
	require.NoError(t, err)
	require.Len(t, failedWebhooks, 1)

	claimed, err := MarkFailedWebhookRetried(t.Context(), "webhook-stale-claim")
	require.NoError(t, err)
	assert.True(t, claimed)

	failedWebhooks, err = FindFailedWebhooks(t.Context(), []string{"evergreen"})
	require.NoError(t, err)
	assert.Empty(t, failedWebhooks)
}

func TestRemoveFailedWebhook(t *testing.T) {
	require.NoError(t, db.Clear(FailedWebhookCollection))
	t.Cleanup(func() { assert.NoError(t, db.Clear(FailedWebhookCollection)) })

	n := makeFailedWebhookNotification("webhook-removed")
	require.NoError(t, InsertFailedWebhook(t.Context(), n, errors.New("delivery failed")))
	require.NoError(t, RemoveFailedWebhook(t.Context(), n.ID))

	count, err := db.Count(t.Context(), FailedWebhookCollection, bson.M{})
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestMarkFailedWebhookRetriedMissingRecord(t *testing.T) {
	require.NoError(t, db.Clear(FailedWebhookCollection))
	t.Cleanup(func() { assert.NoError(t, db.Clear(FailedWebhookCollection)) })

	claimed, err := MarkFailedWebhookRetried(t.Context(), "missing")
	require.NoError(t, err)
	assert.False(t, claimed)
}

func TestFindFailedWebhooks(t *testing.T) {
	require.NoError(t, db.Clear(FailedWebhookCollection))
	t.Cleanup(func() { assert.NoError(t, db.Clear(FailedWebhookCollection)) })

	now := time.Now().Truncate(time.Millisecond)
	for _, failure := range []FailedWebhook{
		{
			NotificationID: "webhook-old",
			Project:        "evergreen",
			Error:          "old failure",
			FailedAt:       now.Add(-2 * time.Hour),
		},
		{
			NotificationID: "webhook-new",
			Project:        "evergreen",
			Error:          "new failure",
			FailedAt:       now.Add(-1 * time.Hour),
		},
		{
			NotificationID: "webhook-other-project",
			Project:        "spruce",
			Error:          "other project failure",
			FailedAt:       now,
		},
	} {
		require.NoError(t, db.Insert(t.Context(), FailedWebhookCollection, &failure))
	}

	t.Run("FiltersByProject", func(t *testing.T) {
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"})
		require.NoError(t, err)
		require.Len(t, failedWebhooks, 2)
		// Sorted newest first.
		assert.Equal(t, "webhook-new", failedWebhooks[0].NotificationID)
		assert.Equal(t, "webhook-old", failedWebhooks[1].NotificationID)
	})

	t.Run("FiltersByProjects", func(t *testing.T) {
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen", "spruce"})
		require.NoError(t, err)
		require.Len(t, failedWebhooks, 3)
		assert.Equal(t, "webhook-other-project", failedWebhooks[0].NotificationID)
		assert.Equal(t, "webhook-new", failedWebhooks[1].NotificationID)
		assert.Equal(t, "webhook-old", failedWebhooks[2].NotificationID)
	})

	t.Run("UnknownProjectReturnsNothing", func(t *testing.T) {
		failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"unknown-project"})
		require.NoError(t, err)
		assert.Empty(t, failedWebhooks)
	})
}

func TestFindFailedWebhooksCapsLimit(t *testing.T) {
	require.NoError(t, db.Clear(FailedWebhookCollection))
	t.Cleanup(func() { assert.NoError(t, db.Clear(FailedWebhookCollection)) })

	failures := make([]any, 0, failedWebhookMaxLimit+1)
	for i := 0; i <= failedWebhookMaxLimit; i++ {
		failures = append(failures, &FailedWebhook{
			NotificationID: fmt.Sprintf("webhook-%d", i),
			Project:        "evergreen",
			FailedAt:       time.Now(),
		})
	}
	require.NoError(t, db.InsertManyUnordered(t.Context(), FailedWebhookCollection, failures...))

	failedWebhooks, err := FindFailedWebhooks(t.Context(), []string{"evergreen"})
	require.NoError(t, err)
	assert.Len(t, failedWebhooks, failedWebhookMaxLimit)
}
