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
	"github.com/mongodb/amboy"
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
			j, ok := NewSpawnhostStartJob(SpawnHostModifyJobOptions{
				Host:      &h,
				Source:    evergreen.ModifySpawnHostManual,
				User:      "user",
				Timestamp: ts,
			}).(*spawnhostStartJob)
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
			j := NewSpawnhostStartJob(SpawnHostModifyJobOptions{
				Host:      &h,
				Source:    evergreen.ModifySpawnHostManual,
				User:      "user",
				Timestamp: ts,
			})

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
			j := NewSpawnhostStartJob(SpawnHostModifyJobOptions{
				Host:      &h,
				Source:    evergreen.ModifySpawnHostManual,
				User:      "user",
				Timestamp: ts,
			})

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
			j := NewSpawnhostStartJob(SpawnHostModifyJobOptions{
				Host:      &h,
				Source:    evergreen.ModifySpawnHostManual,
				User:      "user",
				Timestamp: ts,
			})

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
			j := NewSpawnhostStartJob(SpawnHostModifyJobOptions{
				Host:      &h,
				Source:    evergreen.ModifySpawnHostManual,
				User:      "user",
				Timestamp: ts,
			})
			// Simulate the job running out of retry attempts, which should
			// cause an event to be logged.
			j.UpdateRetryInfo(amboy.JobRetryOptions{CurrentAttempt: utility.ToIntPtr(j.RetryInfo().GetMaxAttempts())})

			j.Run(ctx)
			assert.Error(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostUninitialized, dbHost.Status)

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostStarted, false)
		},
		"RunStartsHostAndSchedulesNextStartTime": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			userTZ, err := time.LoadLocation("Antarctica/South_Pole")
			require.NoError(t, err)
			now := utility.BSONTime(time.Now())
			h := host.Host{
				Id:           "host-stopped",
				Status:       evergreen.HostStopped,
				Provider:     evergreen.ProviderNameMock,
				Distro:       distro.Distro{Provider: evergreen.ProviderNameMock},
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					DailyStartTime: "10:00",
					DailyStopTime:  "18:00",
					TimeZone:       userTZ.String(),
					NextStartTime:  now.Add(time.Minute),
				},
			}
			assert.NoError(t, h.Insert(ctx))
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewSpawnhostStartJob(SpawnHostModifyJobOptions{
				Host:      &h,
				Source:    evergreen.ModifySpawnHostSleepSchedule,
				User:      sleepScheduleUser,
				Timestamp: ts,
			})

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostStarted, true)

			assert.True(t, dbHost.SleepSchedule.NextStartTime.After(now), "next start time should be set in the future")

			hrs, mins, secs := dbHost.SleepSchedule.NextStartTime.In(userTZ).Clock()
			assert.Equal(t, 10, hrs, "next start time should be at 10:00 in user's local time")
			assert.Equal(t, 0, mins, "next start time should be at 10:00 in user's local time")
			assert.Equal(t, 0, secs, "next start time should be at 10:00 in user's local time")
		},
		"RunNoopsIfTemporarilyExemptFromSleepSchedule": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			userTZ, err := time.LoadLocation("Antarctica/South_Pole")
			require.NoError(t, err)
			now := utility.BSONTime(time.Now())
			nextStart := utility.BSONTime(now.Add(time.Hour))
			h := host.Host{
				Id:           "host-running",
				Status:       evergreen.HostStopped,
				Provider:     evergreen.ProviderNameMock,
				Distro:       distro.Distro{Provider: evergreen.ProviderNameMock},
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					DailyStartTime:         "10:00",
					DailyStopTime:          "18:00",
					TimeZone:               userTZ.String(),
					NextStartTime:          nextStart,
					TemporarilyExemptUntil: utility.BSONTime(now.Add(time.Hour)),
				},
			}
			assert.NoError(t, h.Insert(ctx))
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewSpawnhostStartJob(SpawnHostModifyJobOptions{
				Host:      &h,
				Source:    evergreen.ModifySpawnHostSleepSchedule,
				User:      sleepScheduleUser,
				Timestamp: ts,
			})

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostStopped, dbHost.Status, "host should not be started because it is temporarily exempt")

			assert.True(t, dbHost.SleepSchedule.NextStartTime.Equal(nextStart), "next start time should be the same as original")
		},
		"RunNoopsIfNotScheduledToStartYet": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			userTZ, err := time.LoadLocation("Antarctica/South_Pole")
			require.NoError(t, err)
			now := utility.BSONTime(time.Now())
			nextStart := utility.BSONTime(now.Add(time.Hour))
			h := host.Host{
				Id:           "host-running",
				Status:       evergreen.HostStopped,
				Provider:     evergreen.ProviderNameMock,
				Distro:       distro.Distro{Provider: evergreen.ProviderNameMock},
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					DailyStartTime: "10:00",
					DailyStopTime:  "18:00",
					TimeZone:       userTZ.String(),
					NextStartTime:  nextStart,
				},
			}
			assert.NoError(t, h.Insert(ctx))
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewSpawnhostStartJob(SpawnHostModifyJobOptions{
				Host:      &h,
				Source:    evergreen.ModifySpawnHostSleepSchedule,
				User:      sleepScheduleUser,
				Timestamp: ts,
			})

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostStopped, dbHost.Status, "host should not be started because it has not reached its scheduled start time")

			assert.True(t, dbHost.SleepSchedule.NextStartTime.Equal(nextStart), "next start time should be the same as original")
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
