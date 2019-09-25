package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
)

func TestSpawnhostStartJob(t *testing.T) {
	config := testutil.TestConfig()
	assert.NoError(t, evergreen.UpdateConfig(config))
	assert.NoError(t, db.ClearCollections(host.Collection))
	mock := cloud.GetMockProvider()
	t.Run("NewSpawnhostStartJobHostNotStopped", func(t *testing.T) {
		h := host.Host{
			Id:       "host-running",
			Status:   evergreen.HostRunning,
			Provider: evergreen.ProviderNameMock,
			Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
		}
		assert.NoError(t, h.Insert())
		mock.Set(h.Id, cloud.MockInstance{
			IsUp:   true,
			Status: cloud.StatusRunning,
		})

		ts := util.RoundPartOfMinute(1).Format(tsFormat)
		j := NewSpawnhostStartJob(&h, "user", ts)

		j.Run(context.Background())
		assert.Error(t, j.Error())
	})
	t.Run("NewSpawnhostStartJobOK", func(t *testing.T) {
		h := host.Host{
			Id:       "host-stopped",
			Status:   evergreen.HostStopped,
			Provider: evergreen.ProviderNameMock,
			Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
		}
		assert.NoError(t, h.Insert())
		mock.Set(h.Id, cloud.MockInstance{
			IsUp:   true,
			Status: cloud.StatusStopped,
		})

		ts := util.RoundPartOfMinute(1).Format(tsFormat)
		j := NewSpawnhostStartJob(&h, "user", ts)

		j.Run(context.Background())
		assert.NoError(t, j.Error())
		assert.True(t, j.Status().Completed)

		startedHost, err := host.FindOneId(h.Id)
		assert.NoError(t, err)
		assert.Equal(t, evergreen.HostRunning, startedHost.Status)
	})
}
