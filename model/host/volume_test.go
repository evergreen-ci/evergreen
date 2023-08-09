package host

import (
	"context"
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
		{ID: "v1", Expiration: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC), Host: "h0"},
		{ID: "v2", Expiration: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)},
	}
	for _, vol := range volumes {
		require.NoError(t, vol.Insert())
	}

	toDelete, err := FindVolumesToDelete(time.Date(2010, time.November, 10, 23, 0, 0, 0, time.UTC))
	assert.NoError(t, err)
	assert.Len(t, toDelete, 1)
	assert.Equal(t, "v2", toDelete[0].ID)
}

func TestFindVolumesWithNoExpirationToExtend(t *testing.T) {
	require.NoError(t, db.Clear(VolumesCollection))

	volumes := []Volume{
		{ID: "v0", Expiration: time.Date(2009, time.December, 10, 23, 0, 0, 0, time.UTC), NoExpiration: true},
		{ID: "v1", Expiration: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{ID: "v2", Expiration: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC), NoExpiration: true, Host: "h1"},
		{ID: "v3", Expiration: time.Now().Add(time.Hour * 48), NoExpiration: true},
	}

	for _, vol := range volumes {
		require.NoError(t, vol.Insert())
	}

	volumesToExtend, err := FindVolumesWithNoExpirationToExtend()
	assert.NoError(t, err)
	assert.Len(t, volumesToExtend, 1)
	assert.Equal(t, "v0", volumesToExtend[0].ID)
}

func TestCountNoExpirationVolumesForUser(t *testing.T) {
	require.NoError(t, db.Clear(VolumesCollection))

	volumes := []Volume{
		{ID: "v0", NoExpiration: true},
		{ID: "v1", NoExpiration: true, CreatedBy: "me"},
		{ID: "v2", CreatedBy: "me"},
	}
	for _, vol := range volumes {
		require.NoError(t, vol.Insert())
	}

	count, err := CountNoExpirationVolumesForUser("me")
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestFindVolumesWithTerminatedHost(t *testing.T) {
	require.NoError(t, db.ClearCollections(VolumesCollection, Collection))

	volumes := []Volume{
		{ID: "v0", Host: "real_host"},
		{ID: "v1", Host: "terminated_host"},
		{ID: "v2"}, // No host
	}
	for _, vol := range volumes {
		require.NoError(t, vol.Insert())
	}
	hosts := []Host{
		{Id: "real_host", Status: evergreen.HostStopped},
		{Id: "terminated_host", Status: evergreen.HostTerminated},
	}
	for _, h := range hosts {
		require.NoError(t, h.Insert(context.Background()))
	}
	volumes, err := FindVolumesWithTerminatedHost()
	assert.NoError(t, err)
	require.Len(t, volumes, 1)
	assert.Equal(t, volumes[0].ID, "v1")

}
