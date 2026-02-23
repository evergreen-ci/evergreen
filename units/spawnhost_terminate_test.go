package units

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnHostTerminationJob(t *testing.T) {
	t.Run("ManualSourceSetsExpectedFields", func(t *testing.T) {
		ts := utility.RoundPartOfMinute(1).Format(TSFormat)
		h := host.Host{
			Id:       "host_id",
			Status:   evergreen.HostRunning,
			Provider: evergreen.ProviderNameMock,
			Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
		}
		j, ok := NewSpawnHostTerminationJob(&h, "user", ts, evergreen.ModifySpawnHostManual).(*spawnHostTerminationJob)
		require.True(t, ok)

		assert.Equal(t, h.Id, j.HostID)
		assert.Equal(t, "user", j.UserID)
		assert.Equal(t, evergreen.ModifySpawnHostManual, j.Source)
	})
	t.Run("ProjectSettingsSourceSetsExpectedFields", func(t *testing.T) {
		ts := utility.RoundPartOfMinute(1).Format(TSFormat)
		h := host.Host{
			Id:       "host_id",
			Status:   evergreen.HostRunning,
			Provider: evergreen.ProviderNameMock,
			Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
		}
		j, ok := NewSpawnHostTerminationJob(&h, "user", ts, evergreen.ModifySpawnHostProjectSettings).(*spawnHostTerminationJob)
		require.True(t, ok)

		assert.Equal(t, h.Id, j.HostID)
		assert.Equal(t, "user", j.UserID)
		assert.Equal(t, evergreen.ModifySpawnHostProjectSettings, j.Source)
	})
}
