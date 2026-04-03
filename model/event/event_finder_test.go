package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPaginatedHostEvents(t *testing.T) {
	assert.NoError(t, db.ClearCollections(EventCollection))

	hostID := "host"
	tag := "host-tag"

	// Log various events for the host.
	LogHostCreated(t.Context(), hostID)                                               // HOST_CREATED
	LogHostAgentDeployed(t.Context(), hostID)                                         // HOST_AGENT_DEPLOYED
	LogHostDNSNameSet(t.Context(), hostID, "dns-name")                                // HOST_DNS_NAME_SET
	LogHostTaskFinished(t.Context(), "task-1", 0, hostID, evergreen.TaskSystemFailed) // HOST_TASK_FINISHED
	LogHostModifySucceeded(t.Context(), hostID, evergreen.User)                       // HOST_MODIFIED
	LogHostTaskFinished(t.Context(), "task-2", 0, tag, evergreen.TaskSucceeded)       // HOST_TASK_FINISHED

	// Filters by tag correctly.
	opts := PaginatedHostEventsOpts{
		ID:         hostID,
		Tag:        "",
		Limit:      1,
		Page:       0,
		SortAsc:    false,
		EventTypes: []string{},
	}
	entries, totalCount, err := GetPaginatedHostEvents(t.Context(), opts)
	require.NoError(t, err)
	require.Equal(t, 5, totalCount)
	require.NotNil(t, entries[0])
	assert.Equal(t, EventHostModified, entries[0].EventType)

	opts = PaginatedHostEventsOpts{
		ID:         hostID,
		Tag:        tag,
		Limit:      1,
		Page:       0,
		SortAsc:    false,
		EventTypes: []string{},
	}
	entries, totalCount, err = GetPaginatedHostEvents(t.Context(), opts)
	require.NoError(t, err)
	require.Equal(t, 6, totalCount)
	require.NotNil(t, entries[0])
	assert.Equal(t, EventHostTaskFinished, entries[0].EventType)

	// Filters by event types correctly.
	opts = PaginatedHostEventsOpts{
		ID:         hostID,
		Tag:        tag,
		Limit:      2,
		Page:       0,
		SortAsc:    false,
		EventTypes: []string{EventHostTaskFinished},
	}
	entries, totalCount, err = GetPaginatedHostEvents(t.Context(), opts)
	require.NoError(t, err)
	require.Equal(t, 2, totalCount)
	require.NotNil(t, entries[0])
	assert.Equal(t, EventHostTaskFinished, entries[0].EventType)
	require.NotNil(t, entries[1])
	assert.Equal(t, EventHostTaskFinished, entries[1].EventType)

	// Uses correct sort method.
	opts = PaginatedHostEventsOpts{
		ID:         hostID,
		Tag:        tag,
		Limit:      1,
		Page:       0,
		SortAsc:    true,
		EventTypes: []string{},
	}
	entries, totalCount, err = GetPaginatedHostEvents(t.Context(), opts)
	require.NoError(t, err)
	require.Equal(t, 6, totalCount)
	require.NotNil(t, entries[0])
	assert.Equal(t, EventHostCreated, entries[0].EventType)
}

func TestGetEventTypesForHost(t *testing.T) {
	assert.NoError(t, db.ClearCollections(EventCollection))

	hostID := "host"
	tag := "host-tag"

	// Log various events for the host.
	LogHostCreated(t.Context(), hostID)                                               // HOST_CREATED
	LogHostAgentDeployed(t.Context(), hostID)                                         // HOST_AGENT_DEPLOYED
	LogHostDNSNameSet(t.Context(), hostID, "dns-name")                                // HOST_DNS_NAME_SET
	LogHostTaskFinished(t.Context(), "task-1", 0, hostID, evergreen.TaskSystemFailed) // HOST_TASK_FINISHED
	LogHostModifySucceeded(t.Context(), hostID, evergreen.User)                       // HOST_MODIFIED
	LogHostTaskFinished(t.Context(), "task-2", 0, tag, evergreen.TaskSucceeded)       // HOST_TASK_FINISHED

	// Should return non-duplicate host event types.
	eventTypes, err := GetEventTypesForHost(t.Context(), hostID, tag)
	require.NoError(t, err)
	require.NotNil(t, eventTypes)
	require.Len(t, eventTypes, 5)
	// Event types should be sorted alphabetically.
	for i := 0; i < len(eventTypes)-1; i++ {
		assert.Less(t, eventTypes[i], eventTypes[i+1])
	}

	// Should return 0 event types if a host has no events.
	eventTypes, err = GetEventTypesForHost(t.Context(), "host-with-no-events", "")
	require.NoError(t, err)
	require.NotNil(t, eventTypes)
	require.Empty(t, eventTypes)
}
