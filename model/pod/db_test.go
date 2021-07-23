package pod

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindByStaleStarting(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsEmptyForNoMatches": func(t *testing.T) {
			pods, err := FindByStaleStarting()
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

			pods, err := FindByStaleStarting()
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
	p := &Pod{
		ID:     utility.RandomString(),
		Status: StatusInitializing,
		Secret: "secret",
	}
	require.NoError(t, p.Insert())

	p = &Pod{
		ID:     utility.RandomString(),
		Status: StatusStarting,
		Secret: "secret",
	}
	require.NoError(t, p.Insert())

	p = &Pod{
		ID:     utility.RandomString(),
		Status: StatusInitializing,
		Secret: "secret",
	}
	require.NoError(t, p.Insert())

	pods, err := FindByInitializing()
	require.NoError(t, err)
	require.Len(t, pods, 2)
	assert.Equal(t, StatusInitializing, pods[0].Status)
	assert.Equal(t, StatusInitializing, pods[1].Status)
}
