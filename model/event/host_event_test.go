package event

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
)

func TestLoggingHostEvents(t *testing.T) {
	assert.NoError(t, db.Clear(EventCollection))

	t.Run("AllEventsLoggedShouldBePersistedToTheDatabaseAndFetchingThemInOrderShouldSortByTheTimeTheyWereLogged", func(t *testing.T) {
		hostId := "host_id"
		hostTag := "host_tag"
		hostname := "hostname"
		taskId := "task_id"

		// log some events, sleeping in between to make sure the times are different
		LogHostCreated(hostId)
		time.Sleep(1 * time.Millisecond)
		LogHostStatusChanged(hostTag, evergreen.HostRunning, evergreen.HostTerminated, "user", "myLogs")
		time.Sleep(1 * time.Millisecond)
		LogHostDNSNameSet(hostId, hostname)
		time.Sleep(1 * time.Millisecond)
		LogHostProvisioned(hostTag)
		time.Sleep(1 * time.Millisecond)
		LogHostRunningTaskSet(hostId, taskId, 0)
		time.Sleep(1 * time.Millisecond)
		LogHostRunningTaskCleared(hostId, taskId, 0)
		time.Sleep(1 * time.Millisecond)

		// fetch all the events from the database, make sure they are persisted correctly
		hostEventOpts := HostEventsOpts{
			ID:      hostId,
			Tag:     hostTag,
			Limit:   50,
			SortAsc: false,
		}
		eventsForHost, err := Find(GetHostEvents(hostEventOpts))
		assert.NoError(t, err)

		assert.Len(t, eventsForHost, 6)

		event := eventsForHost[5]
		assert.Equal(t, EventHostCreated, event.EventType)

		eventData, ok := event.Data.(*HostEventData)
		assert.True(t, ok)
		assert.Empty(t, eventData.OldStatus)
		assert.Empty(t, eventData.NewStatus)
		assert.Empty(t, eventData.Logs)
		assert.Empty(t, eventData.Hostname)
		assert.Empty(t, eventData.TaskId)
		assert.Empty(t, eventData.TaskPid)

		event = eventsForHost[4]
		assert.Equal(t, EventHostStatusChanged, event.EventType)

		eventData, ok = event.Data.(*HostEventData)
		assert.True(t, ok)
		assert.Equal(t, evergreen.HostRunning, eventData.OldStatus)
		assert.Equal(t, evergreen.HostTerminated, eventData.NewStatus)
		assert.Equal(t, "myLogs", eventData.Logs)
		assert.Empty(t, eventData.Hostname)
		assert.Empty(t, eventData.TaskId)
		assert.Empty(t, eventData.TaskPid)

		event = eventsForHost[3]
		assert.Equal(t, EventHostDNSNameSet, event.EventType)

		eventData, ok = event.Data.(*HostEventData)
		assert.True(t, ok)
		assert.Empty(t, eventData.OldStatus)
		assert.Empty(t, eventData.NewStatus)
		assert.Empty(t, eventData.Logs)
		assert.Equal(t, hostname, eventData.Hostname)
		assert.Empty(t, eventData.TaskId)
		assert.Empty(t, eventData.TaskPid)

		event = eventsForHost[2]
		assert.Equal(t, EventHostProvisioned, event.EventType)

		eventData, ok = event.Data.(*HostEventData)
		assert.True(t, ok)
		assert.Empty(t, eventData.OldStatus)
		assert.Empty(t, eventData.NewStatus)
		assert.Empty(t, eventData.Logs)
		assert.Empty(t, eventData.Hostname)
		assert.Empty(t, eventData.TaskId)
		assert.Empty(t, eventData.TaskPid)

		event = eventsForHost[1]
		assert.Equal(t, EventHostRunningTaskSet, event.EventType)

		eventData, ok = event.Data.(*HostEventData)
		assert.True(t, ok)
		assert.Empty(t, eventData.OldStatus)
		assert.Empty(t, eventData.NewStatus)
		assert.Empty(t, eventData.Logs)
		assert.Empty(t, eventData.Hostname)
		assert.Equal(t, taskId, eventData.TaskId)
		assert.Empty(t, eventData.TaskPid)

		event = eventsForHost[0]
		assert.Equal(t, EventHostRunningTaskCleared, event.EventType)

		eventData, ok = event.Data.(*HostEventData)
		assert.True(t, ok)
		assert.Empty(t, eventData.OldStatus)
		assert.Empty(t, eventData.NewStatus)
		assert.Empty(t, eventData.Logs)
		assert.Empty(t, eventData.Hostname)
		assert.Equal(t, taskId, eventData.TaskId)
		assert.Empty(t, eventData.TaskPid)
	})
}
