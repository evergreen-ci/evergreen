package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVolumeExpiration(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, event.AllLogCollection, alertrecord.Collection))
	volumes := []host.Volume{
		{ID: "v0", Expiration: time.Now().Add(2 * time.Hour)},
		{ID: "v1", Expiration: time.Now().Add(10 * time.Hour)},
		{ID: "v2", Expiration: time.Now().Add(15 * time.Hour)},
		{ID: "v3", Expiration: time.Now().Add(30 * 24 * time.Hour)},
	}
	for _, v := range volumes {
		require.NoError(t, v.Insert())
	}

	j := makeVolumeExpirationWarningsJob()
	j.Run(context.Background())

	events, err := event.FindUnprocessedEvents(evergreen.DefaultEventProcessingLimit)
	assert.NoError(t, err)
	// one event each for v0, v1, v2
	assert.Len(t, events, 3)

	expiringSoonVolumes := []string{"v0", "v1", "v2"}
	for _, e := range events {
		assert.Contains(t, expiringSoonVolumes, e.ResourceId)
	}
}
