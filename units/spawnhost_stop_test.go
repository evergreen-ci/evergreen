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
	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection, event.EventCollection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, mock cloud.MockProvider){
		"NewSpawnhostStopJobSetsExpectedFields": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			h := host.Host{
				Id:       "host_id",
				Status:   evergreen.HostStopped,
				Provider: evergreen.ProviderNameMock,
				Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
			}
			j, ok := NewSpawnhostStopJob(&h, false, evergreen.ModifySpawnHostManual, "user", ts).(*spawnhostStopJob)
			require.True(t, ok)

			assert.NotZero(t, j.RetryInfo().GetMaxAttempts(), "job should retry")
			assert.Equal(t, h.Id, j.HostID)
			assert.Equal(t, "user", j.UserID)
			assert.Equal(t, evergreen.ModifySpawnHostManual, j.Source)
		},
		"RunStopsRunningHost": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			h := host.Host{
				Id:       "host-running",
				Status:   evergreen.HostRunning,
				Provider: evergreen.ProviderNameMock,
				Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
			}
			assert.NoError(t, h.Insert(ctx))
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusStopped,
			})

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewSpawnhostStopJob(&h, false, evergreen.ModifySpawnHostManual, "user", ts)

			j.Run(ctx)
			assert.NoError(t, j.Error())
			assert.True(t, j.Status().Completed)

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostStopped, dbHost.Status)
			assert.False(t, dbHost.SleepSchedule.ShouldKeepOff)

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostStopped, true)
		},
		"RunStopsRunningHostAndSetsKeepOff": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			h := host.Host{
				Id:       "host-running",
				Status:   evergreen.HostRunning,
				Provider: evergreen.ProviderNameMock,
				Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
			}
			assert.NoError(t, h.Insert(ctx))
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusStopped,
			})

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewSpawnhostStopJob(&h, true, "user", ts)

			j.Run(ctx)
			assert.NoError(t, j.Error())
			assert.True(t, j.Status().Completed)

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostStopped, dbHost.Status)
			assert.True(t, dbHost.SleepSchedule.ShouldKeepOff)

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostStopped, true)
		},
		"RunNoopsIfHostIsAlreadyStopped": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
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
			j := NewSpawnhostStopJob(&h, false, evergreen.ModifySpawnHostManual, "user", ts)

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostStopped, dbHost.Status)

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostStopped, true)
		},
		"RunErrorsIfHostCannotBeStopped": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			h := host.Host{
				Id:       "host-uninitialized",
				Status:   evergreen.HostUninitialized,
				Provider: evergreen.ProviderNameMock,
				Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
			}
			assert.NoError(t, h.Insert(ctx))
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewSpawnhostStartJob(&h, evergreen.ModifySpawnHostManual, "user", ts)

			j.Run(ctx)
			assert.Error(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostUninitialized, dbHost.Status)

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostStarted, false)
		},
		// "RunSchedulesNextStopTime": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
		// },
		// "RunNoopsIfNotScheduledToStopYet": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
		// },
	} {
		t.Run(tName, func(t *testing.T) {
			tctx := testutil.TestSpan(ctx, t)
			require.NoError(t, db.ClearCollections(host.Collection, event.EventCollection))
			mock := cloud.GetMockProvider()

			tCase(tctx, t, mock)
		})
	}
}
