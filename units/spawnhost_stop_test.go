package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnhostStopJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	assert.NoError(t, db.ClearCollections(host.Collection, event.EventCollection))
	mock := cloud.GetMockProvider()
	t.Run("NewSpawnhostStopJobSetsExpectedFields", func(t *testing.T) {
		ts := utility.RoundPartOfMinute(1).Format(TSFormat)
		h := host.Host{
			Id:       "host_id",
			Status:   evergreen.HostStopped,
			Provider: evergreen.ProviderNameMock,
			Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
		}
		j, ok := NewSpawnhostStopJob(&h, "user", ts).(*spawnhostStopJob)
		require.True(t, ok)

		assert.NotZero(t, j.RetryInfo().GetMaxAttempts(), "job should retry")
		assert.Equal(t, h.Id, j.HostID)
		assert.Equal(t, "user", j.UserID)
		assert.Equal(t, evergreen.ModifySpawnHostManual, j.Source)
	})
	t.Run("NewSpawnhostStopJobHostNotRunning", func(t *testing.T) {
		tctx := testutil.TestSpan(ctx, t)
		h := host.Host{
			Id:       "host-stopped",
			Status:   evergreen.HostStopped,
			Provider: evergreen.ProviderNameMock,
			Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
		}
		assert.NoError(t, h.Insert(tctx))
		mock.Set(h.Id, cloud.MockInstance{
			Status: cloud.StatusStopped,
		})

		ts := utility.RoundPartOfMinute(1).Format(TSFormat)
		j := NewSpawnhostStopJob(&h, "user", ts)

		j.Run(context.Background())
		assert.Error(t, j.Error())

		checkSpawnHostModificationEvent(t, h.Id, event.EventHostStopped, false)
	})
	t.Run("NewSpawnhostStopJobOK", func(t *testing.T) {
		tctx := testutil.TestSpan(ctx, t)
		h := host.Host{
			Id:       "host-running",
			Status:   evergreen.HostRunning,
			Provider: evergreen.ProviderNameMock,
			Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
		}
		assert.NoError(t, h.Insert(tctx))
		mock.Set(h.Id, cloud.MockInstance{
			Status: cloud.StatusRunning,
		})

		ts := utility.RoundPartOfMinute(1).Format(TSFormat)
		j := NewSpawnhostStopJob(&h, "user", ts)

		j.Run(context.Background())
		assert.NoError(t, j.Error())
		assert.True(t, j.Status().Completed)

		stoppedHost, err := host.FindOneId(tctx, h.Id)
		assert.NoError(t, err)
		assert.Equal(t, evergreen.HostStopped, stoppedHost.Status)

		checkSpawnHostModificationEvent(t, h.Id, event.EventHostStopped, true)
	})
}
