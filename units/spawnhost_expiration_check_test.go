package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
)

func TestSpawnhostExpirationCheckJob(t *testing.T) {
	config := testutil.TestConfig()
	assert.NoError(t, evergreen.UpdateConfig(config))
	assert.NoError(t, db.ClearCollections(host.Collection))
	mock := cloud.GetMockProvider()

	distroRegion1 := distro.Distro{
		Provider:         evergreen.ProviderNameMock,
		ProviderSettings: &map[string]interface{}{"region": "region-1"},
	}

	distroRegion2 := distro.Distro{
		Provider:         evergreen.ProviderNameMock,
		ProviderSettings: &map[string]interface{}{"region": "region-2"},
	}

	h1 := host.Host{
		Id:             "h1",
		Status:         evergreen.HostRunning,
		UserHost:       true,
		Provider:       evergreen.ProviderNameMock,
		Distro:         distroRegion1,
		NoExpiration:   true,
		ExpirationTime: time.Now(),
	}
	h2 := host.Host{
		Id:             "h2",
		Status:         evergreen.HostTerminated,
		UserHost:       true,
		Provider:       evergreen.ProviderNameMock,
		Distro:         distroRegion1,
		NoExpiration:   true,
		ExpirationTime: time.Now(),
	}
	h3 := host.Host{
		Id:             "h3",
		Status:         evergreen.HostRunning,
		UserHost:       true,
		Provider:       evergreen.ProviderNameMock,
		Distro:         distroRegion1,
		NoExpiration:   false,
		ExpirationTime: time.Now(),
	}
	h4 := host.Host{
		Id:             "h4",
		Status:         evergreen.HostRunning,
		UserHost:       true,
		Provider:       evergreen.ProviderNameMock,
		Distro:         distroRegion1,
		NoExpiration:   true,
		ExpirationTime: time.Now().AddDate(1, 0, 0),
	}
	h5 := host.Host{
		Id:             "h5",
		Status:         evergreen.HostRunning,
		UserHost:       true,
		Provider:       evergreen.ProviderNameMock,
		Distro:         distroRegion2,
		NoExpiration:   true,
		ExpirationTime: time.Now(),
	}
	assert.NoError(t, h1.Insert())
	assert.NoError(t, h2.Insert())
	assert.NoError(t, h3.Insert())
	assert.NoError(t, h4.Insert())
	assert.NoError(t, h5.Insert())

	mock.Set(h1.Id, cloud.MockInstance{
		Status: cloud.StatusRunning,
	})
	mock.Set(h5.Id, cloud.MockInstance{
		Status: cloud.StatusRunning,
	})

	ts := util.RoundPartOfHour(0).Format(tsFormat)
	j := NewSpawnhostExpirationCheckJob(ts)

	j.Run(context.Background())
	assert.NoError(t, j.Error())
	assert.True(t, j.Status().Completed)

	found, err := host.FindOneId(h1.Id)
	assert.NoError(t, err)
	assert.True(t, found.ExpirationTime.Sub(h1.ExpirationTime) > 0)

	found, err = host.FindOneId(h2.Id)
	assert.NoError(t, err)
	assert.Equal(t, h2.ExpirationTime.Truncate(time.Second), found.ExpirationTime.Truncate(time.Second))

	found, err = host.FindOneId(h3.Id)
	assert.NoError(t, err)
	assert.Equal(t, h3.ExpirationTime.Truncate(time.Second), found.ExpirationTime.Truncate(time.Second))

	found, err = host.FindOneId(h4.Id)
	assert.NoError(t, err)
	assert.Equal(t, h4.ExpirationTime.Truncate(time.Second), found.ExpirationTime.Truncate(time.Second))

	found, err = host.FindOneId(h5.Id)
	assert.NoError(t, err)
	assert.True(t, found.ExpirationTime.Sub(h5.ExpirationTime) > 0)

}
