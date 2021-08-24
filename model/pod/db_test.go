package pod

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindByNeedsTermination(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsEmptyForNoMatches": func(t *testing.T) {
			pods, err := FindByNeedsTermination()
			assert.NoError(t, err)
			assert.Empty(t, pods)
		},
		"ReturnsMatchingStaleStartingJob": func(t *testing.T) {
			stalePod := Pod{
				ID:     "pod_id0",
				Status: StatusStarting,
				TimeInfo: TimeInfo{
					Starting: time.Now().Add(-time.Hour),
				},
			}
			require.NoError(t, stalePod.Insert())
			runningPod := Pod{
				ID:     "pod_id1",
				Status: StatusRunning,
				TimeInfo: TimeInfo{
					Starting: time.Now().Add(-time.Hour),
				},
			}
			require.NoError(t, runningPod.Insert())
			startingPod := Pod{
				ID:     "pod_id2",
				Status: StatusRunning,
				TimeInfo: TimeInfo{
					Starting: time.Now(),
				},
			}
			require.NoError(t, startingPod.Insert())

			pods, err := FindByNeedsTermination()
			require.NoError(t, err)
			require.Len(t, pods, 1)
			assert.Equal(t, stalePod.ID, pods[0].ID)
		},
		"ReturnsMatchingDecommissionedPod": func(t *testing.T) {
			decommissionedPod := Pod{
				ID:     "pod_id",
				Status: StatusDecommissioned,
			}
			require.NoError(t, decommissionedPod.Insert())

			pods, err := FindByNeedsTermination()
			require.NoError(t, err)
			require.Len(t, pods, 1)
			assert.Equal(t, decommissionedPod.ID, pods[0].ID)
		},
		"ReturnsMatchingStaleInitializingPod": func(t *testing.T) {
			stalePod := Pod{
				ID:     "pod_id",
				Status: StatusInitializing,
				TimeInfo: TimeInfo{
					Initializing: time.Now().Add(-time.Hour),
				},
			}
			require.NoError(t, stalePod.Insert())

			pods, err := FindByNeedsTermination()
			require.NoError(t, err)
			require.Len(t, pods, 1)
			assert.Equal(t, stalePod.ID, pods[0].ID)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()

			tCase(t)
		})
	}
}

func TestFindByInitializing(t *testing.T) {
	require.NoError(t, db.Clear(Collection))

	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	p1 := &Pod{
		ID:     utility.RandomString(),
		Status: StatusInitializing,
	}
	require.NoError(t, p1.Insert())

	p2 := &Pod{
		ID:     utility.RandomString(),
		Status: StatusStarting,
	}
	require.NoError(t, p2.Insert())

	p3 := &Pod{
		ID:     utility.RandomString(),
		Status: StatusInitializing,
	}
	require.NoError(t, p3.Insert())

	pods, err := FindByInitializing()
	require.NoError(t, err)
	require.Len(t, pods, 2)
	assert.Equal(t, StatusInitializing, pods[0].Status)
	assert.Equal(t, StatusInitializing, pods[1].Status)

	ids := map[string]struct{}{p1.ID: {}, p3.ID: {}}
	assert.Contains(t, ids, pods[0].ID)
	assert.Contains(t, ids, pods[1].ID)
}
