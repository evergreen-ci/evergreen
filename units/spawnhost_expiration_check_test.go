package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnhostExpirationCheckJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := testutil.TestConfig()
	assert.NoError(t, evergreen.UpdateConfig(config))
	assert.NoError(t, db.ClearCollections(host.Collection))
	mock := cloud.GetMockProvider()

	h := host.Host{
		Id:       "test-host",
		Status:   evergreen.HostRunning,
		UserHost: true,
		Provider: evergreen.ProviderNameMock,
		Distro: distro.Distro{
			Provider: evergreen.ProviderNameMock,
			ProviderSettingsList: []*birch.Document{birch.NewDocument(
				birch.EC.String("region", "test-region"),
			)},
		},
		NoExpiration:   true,
		ExpirationTime: time.Now(),
	}

	assert.NoError(t, h.Insert())
	mock.Set(h.Id, cloud.MockInstance{
		Status: cloud.StatusRunning,
	})

	ts := utility.RoundPartOfHour(0).Format(TSFormat)
	j := NewSpawnhostExpirationCheckJob(ts, &h)
	j.Run(context.Background())
	assert.NoError(t, j.Error())
	assert.True(t, j.Status().Completed)

	found, err := host.FindOneId(ctx, h.Id)
	assert.NoError(t, err)
	require.NotNil(t, found)
	assert.True(t, found.ExpirationTime.Sub(h.ExpirationTime) > 0)
}
