package units

import (
	"context"
	"testing"
	"time"

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
			j := NewSpawnhostStopJob(&h, true, evergreen.ModifySpawnHostManual, "user", ts)

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
		"RunStopsHostAndSchedulesNextStopTime": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			userTZ, err := time.LoadLocation("Antarctica/South_Pole")
			require.NoError(t, err)
			now := utility.BSONTime(time.Now())
			h := host.Host{
				Id:           "host-running",
				Status:       evergreen.HostRunning,
				Provider:     evergreen.ProviderNameMock,
				Distro:       distro.Distro{Provider: evergreen.ProviderNameMock},
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					DailyStartTime: "10:00",
					DailyStopTime:  "18:00",
					TimeZone:       userTZ.String(),
					NextStopTime:   now.Add(-time.Minute),
				},
			}
			assert.NoError(t, h.Insert(ctx))
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusStopped,
			})

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewSpawnhostStopJob(&h, false, evergreen.ModifySpawnHostSleepSchedule, sleepScheduleUser, ts)

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostStopped, dbHost.Status)

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostStopped, true)

			assert.True(t, dbHost.SleepSchedule.NextStopTime.After(now), "next stop time should be set in the future")

			hrs, mins, secs := dbHost.SleepSchedule.NextStopTime.In(userTZ).Clock()
			assert.Equal(t, 18, hrs, "next stop time should be at 18:00 in user's local time")
			assert.Equal(t, 0, mins, "next stop time should be at 18:00 in user's local time")
			assert.Equal(t, 0, secs, "next stop time should be at 18:00 in user's local time")
		},
		"RunNoopsIfNotScheduledToStopYet": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			userTZ, err := time.LoadLocation("Antarctica/South_Pole")
			require.NoError(t, err)
			now := utility.BSONTime(time.Now())
			nextStop := utility.BSONTime(now.Add(time.Hour))
			h := host.Host{
				Id:           "host-running",
				Status:       evergreen.HostRunning,
				Provider:     evergreen.ProviderNameMock,
				Distro:       distro.Distro{Provider: evergreen.ProviderNameMock},
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					DailyStartTime: "10:00",
					DailyStopTime:  "18:00",
					TimeZone:       userTZ.String(),
					NextStopTime:   nextStop,
				},
			}
			assert.NoError(t, h.Insert(ctx))
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusStopped,
			})

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewSpawnhostStopJob(&h, false, evergreen.ModifySpawnHostSleepSchedule, sleepScheduleUser, ts)

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status, "host should not be stopped because it has not reached its scheduled stop time")

			assert.True(t, dbHost.SleepSchedule.NextStopTime.Equal(nextStop), "next stop time should be the same as original")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx := testutil.TestSpan(ctx, t)
			require.NoError(t, db.ClearCollections(host.Collection, event.EventCollection))
			mock := cloud.GetMockProvider()

			tCase(tctx, t, mock)
		})
	}
}
