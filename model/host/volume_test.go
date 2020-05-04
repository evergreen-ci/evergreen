package host

import (
	"testing"
	"time"

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
