package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodEvents(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"LogPodStatusChanged": func(t *testing.T) {
			id := "pod_id"
			oldStatus := "initializing"
			newStatus := "starting"
			reason := "some reason"
			LogPodStatusChanged(id, oldStatus, newStatus, reason)

			events, err := Find(MostRecentPodEvents(id, 10))
			require.NoError(t, err)
			require.Len(t, events, 1)

			assert.Equal(t, id, events[0].ResourceId)
			require.NotZero(t, events[0].Data)
			data, ok := events[0].Data.(*PodData)
			require.True(t, ok)
			assert.Equal(t, oldStatus, data.OldStatus)
			assert.Equal(t, newStatus, data.NewStatus)
			assert.Equal(t, reason, data.Reason)
		},
		"LogPodAssignedTask": func(t *testing.T) {
			podID := "pod_id"
			taskID := "task_id"
			execution := 5
			LogPodAssignedTask(podID, taskID, 5)

			events, err := Find(MostRecentPodEvents(podID, 10))
			require.NoError(t, err)
			require.Len(t, events, 1)

			assert.Equal(t, podID, events[0].ResourceId)
			require.NotZero(t, events[0].Data)
			data, ok := events[0].Data.(*PodData)
			require.True(t, ok)
			assert.Equal(t, taskID, data.TaskID)
			assert.Equal(t, execution, data.TaskExecution)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(EventCollection))
			defer func() {
				assert.NoError(t, db.Clear(EventCollection))
			}()
			tCase(t)
		})
	}
}
