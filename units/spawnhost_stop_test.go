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

func TestSpawnhostStopJob(t *testing.T) {
	config := testutil.TestConfig()
	assert.NoError(t, evergreen.UpdateConfig(config))
	assert.NoError(t, db.ClearCollections(host.Collection))
	mock := cloud.GetMockProvider()
	t.Run("NewSpawnhostStopJobHostNotRunning", func(t *testing.T) {
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
		j := NewSpawnhostStopJob(&h, "user", ts)

		j.Run(context.Background())
		assert.Error(t, j.Error())
	})
	t.Run("NewSpawnhostStopJobOK", func(t *testing.T) {
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
		j := NewSpawnhostStopJob(&h, "user", ts)

		j.Run(context.Background())
		assert.NoError(t, j.Error())
		assert.True(t, j.Status().Completed)

		stoppedHost, err := host.FindOneId(h.Id)
		assert.NoError(t, err)
		assert.Equal(t, evergreen.HostStopped, stoppedHost.Status)
	})
}
