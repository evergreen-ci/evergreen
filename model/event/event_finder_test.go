package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
)

func TestMostRecentPaginatedPodEvents(t *testing.T) {
	assert.NoError(t, db.ClearCollections(EventCollection))
	for i := 0; i < 20; i++ {
		podId := "pod1"
		if i%2 == 0 {
			podId = "pod2"
		}
		LogPodAssignedTask(podId, "task", i)
	}

	// Query for pod1 events, limit 10, page 0
	events, count, err := MostRecentPaginatedPodEvents("pod1", 10, 0)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)
	assert.Len(t, events, 10)
	for i := 0; i < 10; i++ {
		assert.Equal(t, "pod1", events[i].ResourceId)
		assert.Equal(t, 19-(2*i), events[i].Data.(*PodData).TaskExecution)

	}

	// Query for pod1 events, limit 10, page 1
	events, count, err = MostRecentPaginatedPodEvents("pod1", 10, 1)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)
	assert.Len(t, events, 0)

	// Query for pod1 events, limit 5, page 1
	events, count, err = MostRecentPaginatedPodEvents("pod1", 5, 1)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)
	assert.Len(t, events, 5)
	for i := 0; i < 5; i++ {
		assert.Equal(t, "pod1", events[i].ResourceId)
		assert.Equal(t, 9-(2*i), events[i].Data.(*PodData).TaskExecution)
	}

	// Query for pod1 events, limit 11, page 0
	events, count, err = MostRecentPaginatedPodEvents("pod1", 11, 0)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)
	assert.Len(t, events, 10)
	for i := 0; i < 10; i++ {
		assert.Equal(t, "pod1", events[i].ResourceId)
		assert.Equal(t, 19-(2*i), events[i].Data.(*PodData).TaskExecution)
	}
}
