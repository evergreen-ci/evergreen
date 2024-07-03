package units

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSleepSchedulerJob(t *testing.T) {
	t.Run("NewSleepSchedulerJobSetsExpectedFields", func(t *testing.T) {
		env := &mock.Environment{}
		j, ok := NewSleepSchedulerJob(env, "ts").(*sleepSchedulerJob)
		require.True(t, ok)
		assert.Contains(t, j.ID(), sleepSchedulerJobName)
		assert.Contains(t, j.ID(), "ts")
		assert.NotZero(t, j.env)
	})

	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection))
	}()

	const easternTZ = "America/New_York"
	easternTZLoc, err := time.LoadLocation(easternTZ)
	require.NoError(t, err)

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob){
		"EnqueuesJobsForHostsNeedingToStopForSleepSchedule": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			now := utility.BSONTime(time.Now())
			hosts := []host.Host{
				{
					Id:           "h0",
					Status:       evergreen.HostRunning,
					StartedBy:    "me",
					NoExpiration: true,
					SleepSchedule: host.SleepScheduleInfo{
						WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
						NextStartTime:    utility.BSONTime(now.Add(time.Hour)),
						NextStopTime:     utility.BSONTime(now.Add(-time.Minute)),
					},
				},
				{
					Id:           "h1",
					Status:       evergreen.HostRunning,
					StartedBy:    "me",
					NoExpiration: true,
					SleepSchedule: host.SleepScheduleInfo{
						TemporarilyExemptUntil: utility.BSONTime(now.Add(time.Hour)),
					},
				},
				{
					Id:           "h2",
					Status:       evergreen.HostStopping,
					StartedBy:    "me",
					NoExpiration: true,
					SleepSchedule: host.SleepScheduleInfo{
						WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
						NextStartTime:    utility.BSONTime(now.Add(time.Hour)),
						NextStopTime:     utility.BSONTime(now.Add(-time.Minute)),
					},
				},
			}
			for _, h := range hosts {
				require.NoError(t, h.Insert(ctx))
			}

			j.Run(ctx)
			assert.NoError(t, j.Error())

			q, err := env.RemoteQueueGroup().Get(ctx, spawnHostModificationQueueGroup)
			require.NoError(t, err)

			numJobs := 0
			expectedHostsToStop := map[string]bool{"h0": false, "h2": false}
			for ji := range q.JobInfo(ctx) {
				numJobs++
				if ji.Type.Name == spawnhostStopName {
					for hostID := range expectedHostsToStop {
						if strings.Contains(ji.ID, hostID) {
							expectedHostsToStop[hostID] = true
							break
						}
					}
				}
			}
			assert.Equal(t, 2, numJobs)
			for hostID, found := range expectedHostsToStop {
				assert.True(t, found, "expected host '%s' to have a stop job", hostID)
			}
		},
		"EnqueuesJobsForHostsNeedingToStartForSleepSchedule": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			now := utility.BSONTime(time.Now())
			hosts := []host.Host{
				{
					Id:           "h0",
					Status:       evergreen.HostStopped,
					StartedBy:    "me",
					NoExpiration: true,
					SleepSchedule: host.SleepScheduleInfo{
						WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
						NextStartTime:    utility.BSONTime(now.Add(-time.Minute)),
						NextStopTime:     utility.BSONTime(now.Add(time.Hour)),
					},
				},
				{
					Id:           "h1",
					Status:       evergreen.HostRunning,
					StartedBy:    "me",
					NoExpiration: true,
					SleepSchedule: host.SleepScheduleInfo{
						TemporarilyExemptUntil: utility.BSONTime(now.Add(time.Hour)),
					},
				},
				{
					Id:           "h2",
					Status:       evergreen.HostStopped,
					StartedBy:    "me",
					NoExpiration: true,
					SleepSchedule: host.SleepScheduleInfo{
						WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
						NextStartTime:    utility.BSONTime(now.Add(time.Minute)),
						NextStopTime:     utility.BSONTime(now.Add(time.Hour)),
					},
				},
			}
			for _, h := range hosts {
				require.NoError(t, h.Insert(ctx))
			}

			j.Run(ctx)
			assert.NoError(t, j.Error())

			q, err := env.RemoteQueueGroup().Get(ctx, spawnHostModificationQueueGroup)
			require.NoError(t, err)

			numJobs := 0
			expectedHostsToStart := map[string]bool{"h0": false, "h2": false}
			for ji := range q.JobInfo(ctx) {
				numJobs++
				if ji.Type.Name == spawnhostStartName {
					for hostID := range expectedHostsToStart {
						if strings.Contains(ji.ID, hostID) {
							expectedHostsToStart[hostID] = true
							break
						}
					}
				}
			}
			assert.Equal(t, 2, numJobs)
			for hostID, found := range expectedHostsToStart {
				assert.True(t, found, "expected host '%s' to have a start job", hostID)
			}
		},
		"NoopsWithoutHostsNeedingModificationForSleepSchedule": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			j.Run(ctx)
			assert.NoError(t, j.Error())

			q, err := env.RemoteQueueGroup().Get(ctx, spawnHostModificationQueueGroup)
			require.NoError(t, err)
			assert.Zero(t, q.Stats(ctx).Total)
		},
		"AddsNextSleepScheduleTimesBasedOnCurrentTimeForHostMissingThem": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			h := host.Host{
				Id:           "host_missing_sleep_schedule_times",
				Status:       evergreen.HostRunning,
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					DailyStartTime: "10:00",
					DailyStopTime:  "18:00",
					TimeZone:       easternTZ,
				},
			}
			require.NoError(t, h.Insert(ctx))

			// Simulate the current time, which is:
			// Wednesday February 21, 2024 at 15:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-21 15:00:00", easternTZLoc)
			require.NoError(t, err)
			now = utility.BSONTime(now.UTC())

			j.startedAt = now
			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)

			expectedNextStartTime, err := time.ParseInLocation(time.DateTime, "2024-02-22 10:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStartTime, dbHost.SleepSchedule.NextStartTime, 0, "next start time should be at 10:00 local time on the next day")

			expectedNextStopTime, err := time.ParseInLocation(time.DateTime, "2024-02-21 18:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStopTime, dbHost.SleepSchedule.NextStopTime, 0, "next stop time should be at 18:00 local time on the same day")
		},
		"AddsNextStartTimeForHostMissingIt": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			now := utility.BSONTime(time.Now())
			h := host.Host{
				Id:           "host_missing_sleep_schedule_times",
				Status:       evergreen.HostRunning,
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					TimeZone:         "America/New_York",
					WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
					NextStopTime:     now,
				},
			}
			require.NoError(t, h.Insert(ctx))

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.NotZero(t, dbHost.SleepSchedule.NextStartTime)
			assert.True(t, dbHost.SleepSchedule.NextStartTime.After(now), "next start time should be in the future")
			assert.True(t, dbHost.SleepSchedule.NextStopTime.Equal(now), "next stop time should be unchanged")
		},
		"AddsNextStopTimeForHostMissingIt": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			now := utility.BSONTime(time.Now())
			h := host.Host{
				Id:           "host_missing_sleep_schedule_times",
				Status:       evergreen.HostRunning,
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					TimeZone:         "America/New_York",
					WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
					NextStartTime:    now,
				},
			}
			require.NoError(t, h.Insert(ctx))

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.True(t, dbHost.SleepSchedule.NextStartTime.Equal(now), "next start time should be unchanged")
			assert.NotZero(t, dbHost.SleepSchedule.NextStopTime)
			assert.True(t, dbHost.SleepSchedule.NextStopTime.After(now), "next stop time should be in the future")
		},
		"ClearsExpiredTemporaryExemptionAndSetsNextStopAndStartTimesForHost": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			now := utility.BSONTime(time.Now())
			h := host.Host{
				Id:           "host_missing_sleep_schedule_times",
				Status:       evergreen.HostRunning,
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					TimeZone:               "America/New_York",
					WholeWeekdaysOff:       []time.Weekday{time.Saturday, time.Sunday},
					TemporarilyExemptUntil: now.Add(-time.Hour),
				},
			}
			require.NoError(t, h.Insert(ctx))

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Zero(t, dbHost.SleepSchedule.TemporarilyExemptUntil, "temporary exemption should be cleared")
			assert.True(t, dbHost.SleepSchedule.NextStartTime.After(now), "next start time should be in the future")
			assert.True(t, dbHost.SleepSchedule.NextStopTime.After(now), "next stop time should be in the future")
		},
		"DoesNotAddNextSleepScheduleTimesForPermanentlyExemptHost": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			originalSettings, err := evergreen.GetConfig(ctx)
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, originalSettings.SleepSchedule.Set(ctx))
			}()
			env.EvergreenSettings.SleepSchedule.PermanentlyExemptHosts = []string{"host_missing_sleep_schedule_times_but_permanently_exempt"}
			require.NoError(t, env.EvergreenSettings.SleepSchedule.Set(ctx))

			h := host.Host{
				Id:           "host_missing_sleep_schedule_times_but_permanently_exempt",
				Status:       evergreen.HostRunning,
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					TimeZone:          "America/New_York",
					WholeWeekdaysOff:  []time.Weekday{time.Saturday, time.Sunday},
					PermanentlyExempt: true,
				},
			}
			require.NoError(t, h.Insert(ctx))

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Zero(t, dbHost.SleepSchedule.NextStartTime)
			assert.Zero(t, dbHost.SleepSchedule.NextStopTime)
		},
		"DoesNotAddNextSleepScheduleTimesForTerminatedHost": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			h := host.Host{
				Id:           "host_missing_sleep_schedule_times_but_terminated",
				Status:       evergreen.HostTerminated,
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					TimeZone:         "America/New_York",
					WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
				},
			}
			require.NoError(t, h.Insert(ctx))

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Zero(t, dbHost.SleepSchedule.NextStartTime)
			assert.Zero(t, dbHost.SleepSchedule.NextStopTime)
		},
		"ReschedulesSleepScheduleTimesBasedOnCurrentTimeForHostExceedingAttemptTimeouts": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			// Simulate the current time, which is:
			// Wednesday February 21, 2024 at 15:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-21 15:00:00", easternTZLoc)
			require.NoError(t, err)
			now = utility.BSONTime(now.UTC())

			h := host.Host{
				Id:           "host_with_long_outdated_sleep_schedule_times",
				Status:       evergreen.HostRunning,
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					DailyStartTime: "10:00",
					DailyStopTime:  "18:00",
					TimeZone:       easternTZ,
					NextStartTime:  utility.BSONTime(now.Add(-utility.Day)),
					NextStopTime:   utility.BSONTime(now.Add(-utility.Day)),
				},
			}
			require.NoError(t, h.Insert(ctx))

			j.startedAt = now
			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)

			expectedNextStartTime, err := time.ParseInLocation(time.DateTime, "2024-02-22 10:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStartTime, dbHost.SleepSchedule.NextStartTime, 0, "next start time should be at 10:00 local time on the next day")

			expectedNextStopTime, err := time.ParseInLocation(time.DateTime, "2024-02-21 18:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStopTime, dbHost.SleepSchedule.NextStopTime, 0, "next stop time should be at 18:00 local time on the same day")
		},
		"ReschedulesNextStopForHostExceedingAttemptTimeout": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			now := utility.BSONTime(time.Now())
			h := host.Host{
				Id:           "host_taking_too_long_to_stop",
				Status:       evergreen.HostRunning,
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					TimeZone:         "America/New_York",
					WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
					NextStartTime:    now,
					NextStopTime:     utility.BSONTime(now.Add(-utility.Day)),
				},
			}
			require.NoError(t, h.Insert(ctx))

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.True(t, dbHost.SleepSchedule.NextStartTime.Equal(now), "next start time should be unchanged")
			assert.True(t, dbHost.SleepSchedule.NextStopTime.After(now), "next stop time should be re-scheduled to be in the future")
		},
		"ReschedulesNextStartForHostExceedingAttemptTimeout": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			now := utility.BSONTime(time.Now())
			h := host.Host{
				Id:           "host_taking_too_long_to_start",
				Status:       evergreen.HostStopped,
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					TimeZone:         "America/New_York",
					WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
					NextStartTime:    utility.BSONTime(now.Add(-utility.Day)),
					NextStopTime:     now,
				},
			}
			require.NoError(t, h.Insert(ctx))

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.True(t, h.SleepSchedule.NextStopTime.Equal(now), "next stop time should be unchanged")
			assert.True(t, dbHost.SleepSchedule.NextStartTime.After(now), "next start time should be re-scheduled to be in the future")
		},
		"MarksNewlyAddedHostAsPermanentlyExempt": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			originalSettings, err := evergreen.GetConfig(ctx)
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, originalSettings.SleepSchedule.Set(ctx))
			}()
			env.EvergreenSettings.SleepSchedule.PermanentlyExemptHosts = []string{"host_added_to_permanent_exemption"}
			require.NoError(t, env.EvergreenSettings.SleepSchedule.Set(ctx))

			now := utility.BSONTime(time.Now())
			h := host.Host{
				Id:           "host_added_to_permanent_exemption",
				Status:       evergreen.HostRunning,
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					TimeZone:         "America/New_York",
					WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
					NextStartTime:    now,
					NextStopTime:     now,
				},
			}
			require.NoError(t, h.Insert(ctx))

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.True(t, dbHost.SleepSchedule.PermanentlyExempt, "host should be marked as permanently exempt")
			assert.Zero(t, dbHost.SleepSchedule.NextStartTime, "host should clear its next start time for permanent exemption")
			assert.Zero(t, dbHost.SleepSchedule.NextStopTime, "host should clear its next stop time for permanent exemption")
		},
		"MarksNewlyRemovedHostAsNoLongerPermanentlyExempt": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			originalSettings, err := evergreen.GetConfig(ctx)
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, originalSettings.SleepSchedule.Set(ctx))
			}()
			env.EvergreenSettings.SleepSchedule.PermanentlyExemptHosts = []string{"some_other_host"}
			require.NoError(t, env.EvergreenSettings.SleepSchedule.Set(ctx))

			now := utility.BSONTime(time.Now())
			h := host.Host{
				Id:           "host_removed_from_permanent_exemption",
				Status:       evergreen.HostRunning,
				NoExpiration: true,
				SleepSchedule: host.SleepScheduleInfo{
					TimeZone:          "America/New_York",
					WholeWeekdaysOff:  []time.Weekday{time.Saturday, time.Sunday},
					PermanentlyExempt: true,
				},
			}
			require.NoError(t, h.Insert(ctx))

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.False(t, dbHost.SleepSchedule.PermanentlyExempt, "host should no longer be marked as permanently exempt")
			assert.NotZero(t, dbHost.SleepSchedule.NextStartTime, "host should set its next start time")
			assert.True(t, dbHost.SleepSchedule.NextStartTime.After(now), "next start time should be in the future")
			assert.NotZero(t, dbHost.SleepSchedule.NextStopTime, "host should set its next stop time")
			assert.True(t, dbHost.SleepSchedule.NextStartTime.After(now), "next stop time should be in the future")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = testutil.TestSpan(ctx, t)

			require.NoError(t, db.ClearCollections(host.Collection))
			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			oldServiceFlags, err := evergreen.GetServiceFlags(ctx)
			require.NoError(t, err)
			newServiceFlags := *oldServiceFlags
			newServiceFlags.SleepScheduleBetaTestDisabled = true
			require.NoError(t, evergreen.SetServiceFlags(ctx, newServiceFlags))
			defer func() {
				assert.NoError(t, evergreen.SetServiceFlags(ctx, *oldServiceFlags))
			}()

			j, ok := NewSleepSchedulerJob(env, "ts").(*sleepSchedulerJob)
			require.True(t, ok)

			tCase(ctx, t, env, j)
		})
	}
}
