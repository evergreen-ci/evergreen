package event

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestLogVersionStateChangeEvent(t *testing.T) {
	require.NoError(t, db.ClearCollections(EventCollection))

	versionID := "test_version_123"
	status := "succeeded"

	LogVersionStateChangeEvent(context.Background(), versionID, status)

	// Allow time for the event to be logged
	time.Sleep(100 * time.Millisecond)

	events, err := Find(context.Background(), db.Query(bson.M{
		ResourceIdKey:   versionID,
		ResourceTypeKey: ResourceTypeVersion,
		TypeKey:         VersionStateChange,
	}))
	require.NoError(t, err)
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, versionID, event.ResourceId)
	assert.Equal(t, ResourceTypeVersion, event.ResourceType)
	assert.Equal(t, VersionStateChange, event.EventType)

	eventData, ok := event.Data.(*VersionEventData)
	require.True(t, ok)
	assert.Equal(t, status, eventData.Status)
}

func TestLogVersionStateChangeEventWithCanceledContext(t *testing.T) {
	require.NoError(t, db.ClearCollections(EventCollection))

	versionID := "test_version_456"
	status := "failed"

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Event should still be logged because logEventWithRetry uses detached context
	LogVersionStateChangeEvent(ctx, versionID, status)

	// Allow time for the event to be logged with retries
	time.Sleep(500 * time.Millisecond)

	events, err := Find(context.Background(), db.Query(bson.M{
		ResourceIdKey:   versionID,
		ResourceTypeKey: ResourceTypeVersion,
		TypeKey:         VersionStateChange,
	}))
	require.NoError(t, err)
	require.Len(t, events, 1, "Event should be logged even with canceled parent context")

	event := events[0]
	assert.Equal(t, versionID, event.ResourceId)
	assert.Equal(t, status, event.Data.(*VersionEventData).Status)
}
