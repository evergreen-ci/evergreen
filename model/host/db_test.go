package host

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsolidateHostsForUser(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection, VolumesCollection))
	h1 := Host{
		Id:        "h1",
		StartedBy: "me",
		Status:    evergreen.HostRunning,
	}
	h2 := Host{
		Id:        "h2",
		StartedBy: "me",
		Status:    evergreen.HostTerminated,
	}
	h3 := Host{
		Id:        "h3",
		StartedBy: "me",
		Status:    evergreen.HostStopped,
	}
	h4 := Host{
		Id:        "h4",
		StartedBy: "NOT me",
		Status:    evergreen.HostRunning,
	}
	assert.NoError(t, db.InsertMany(Collection, h1, h2, h3, h4))

	v1 := Volume{
		ID:        "v1",
		CreatedBy: "me",
	}
	v2 := Volume{
		ID:        "v2",
		CreatedBy: "NOT me",
	}
	assert.NoError(t, db.InsertMany(VolumesCollection, v1, v2))

	ctx := context.TODO()
	assert.NoError(t, ConsolidateHostsForUser(ctx, "me", "new_me"))

	hostFromDB, err := FindOneId(ctx, "h1")
	assert.NoError(t, err)
	assert.Equal(t, "new_me", hostFromDB.StartedBy)

	hostFromDB, err = FindOneId(ctx, "h2")
	assert.NoError(t, err)
	assert.NotEqual(t, "new_me", hostFromDB.StartedBy)

	hostFromDB, err = FindOneId(ctx, "h3")
	assert.NoError(t, err)
	assert.Equal(t, "new_me", hostFromDB.StartedBy)

	hostFromDB, err = FindOneId(ctx, "h4")
	assert.NoError(t, err)
	assert.NotEqual(t, "new_me", hostFromDB.StartedBy)

	volumes, err := FindVolumesByUser("me")
	assert.Len(t, volumes, 0)

	volumes, err = FindVolumesByUser("new_me")
	require.Len(t, volumes, 1)
	assert.Equal(t, volumes[0].ID, "v1")

	volumes, err = FindVolumesByUser("NOT me")
	require.Len(t, volumes, 1)
	assert.Equal(t, volumes[0].ID, "v2")

}
