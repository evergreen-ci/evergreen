package host

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindVolumesToDelete(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, VolumesCollection))

	volumes := []Volume{
		{ID: "v0", Expiration: time.Date(2010, time.December, 10, 23, 0, 0, 0, time.UTC)},
		{ID: "v1", Expiration: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{ID: "v2", Expiration: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{ID: "v3", Expiration: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)},
	}
	for _, vol := range volumes {
		require.NoError(t, vol.Insert())
	}

	hosts := []Host{
		{
			Id:      "h0",
			Status:  evergreen.HostRunning,
			Volumes: []VolumeAttachment{{VolumeID: "v1"}, {VolumeID: "other"}},
		},
		{
			Id:      "h1",
			Status:  evergreen.HostTerminated,
			Volumes: []VolumeAttachment{{VolumeID: "v2"}, {VolumeID: "other"}},
		},
	}
	for _, h := range hosts {
		require.NoError(t, h.Insert())
	}

	toDelete, err := FindVolumesToDelete(time.Date(2010, time.November, 10, 23, 0, 0, 0, time.UTC))
	assert.NoError(t, err)
	assert.Len(t, toDelete, 2)
	expectedVolumeIDs := []string{"v2", "v3"}
	for _, vol := range toDelete {
		assert.Contains(t, expectedVolumeIDs, vol.ID)
	}
}
