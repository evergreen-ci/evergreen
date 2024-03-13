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

func TestSpawnhostStartJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)
	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection, event.EventCollection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, mock cloud.MockProvider){
		"NewSpawnhostStartJobSetsExpectedFields": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			h := host.Host{
				Id:       "host_id",
				Status:   evergreen.HostRunning,
				Provider: evergreen.ProviderNameMock,
				Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
			}
			j, ok := NewSpawnhostStartJob(&h, evergreen.ModifySpawnHostManual, "user", ts).(*spawnhostStartJob)
			require.True(t, ok)

			assert.NotZero(t, j.RetryInfo().GetMaxAttempts(), "job should retry")
			assert.Equal(t, h.Id, j.HostID)
			assert.Equal(t, "user", j.UserID)
			assert.Equal(t, evergreen.ModifySpawnHostManual, j.Source)
		},
		"RunStartsStoppedHost": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			h := host.Host{
				Id:       "host-stopped",
				Status:   evergreen.HostStopped,
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
			assert.NoError(t, j.Error())
			assert.True(t, j.Status().Completed)

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostStarted, true)
		},
		"RunStartsStoppedHostAndClearsKeepOff": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			h := host.Host{
				Id:       "host-stopped",
				Status:   evergreen.HostStopped,
				Provider: evergreen.ProviderNameMock,
				Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
				SleepSchedule: host.SleepScheduleInfo{
					ShouldKeepOff: true,
				},
			}
			assert.NoError(t, h.Insert(ctx))
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewSpawnhostStartJob(&h, "user", ts)

			j.Run(ctx)
			assert.NoError(t, j.Error())
			assert.True(t, j.Status().Completed)

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
			assert.False(t, dbHost.SleepSchedule.ShouldKeepOff)

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostStarted, true)
		},
		"RunNoopsIfHostIsAlreadyRunning": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			h := host.Host{
				Id:       "host-running",
				Status:   evergreen.HostRunning,
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
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostStarted, true)
		},
		"RunErrorsIfHostCannotBeStarted": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
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
		// "RunSchedulesNextStartTime": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
		// },
		// "RunNoopsIfNotScheduledToStart": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
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
