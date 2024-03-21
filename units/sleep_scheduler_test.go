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

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob){
		"EnqueuesJobsForHostsNeedingToStopForSleepSchedule": func(ctx context.Context, t *testing.T, env *mock.Environment, j *sleepSchedulerJob) {
			hosts := []host.Host{
				{
					Id:           "h0",
					Status:       evergreen.HostRunning,
					StartedBy:    "me",
					NoExpiration: true,
					SleepSchedule: host.SleepScheduleInfo{
						NextStopTime: time.Now().Add(-time.Minute),
					},
				},
				{
					Id:           "h1",
					Status:       evergreen.HostRunning,
					StartedBy:    "me",
					NoExpiration: true,
					SleepSchedule: host.SleepScheduleInfo{
						PermanentlyExempt: true,
					},
				},
				{
					Id:           "h2",
					Status:       evergreen.HostStopping,
					StartedBy:    "me",
					NoExpiration: true,
					SleepSchedule: host.SleepScheduleInfo{
						NextStartTime: time.Now().Add(-5 * time.Minute),
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
			hosts := []host.Host{
				{
					Id:           "h0",
					Status:       evergreen.HostStopped,
					StartedBy:    "me",
					NoExpiration: true,
					SleepSchedule: host.SleepScheduleInfo{
						NextStartTime: time.Now().Add(-time.Minute),
					},
				},
				{
					Id:           "h1",
					Status:       evergreen.HostRunning,
					StartedBy:    "me",
					NoExpiration: true,
					SleepSchedule: host.SleepScheduleInfo{
						PermanentlyExempt: true,
					},
				},
				{
					Id:           "h2",
					Status:       evergreen.HostStopped,
					StartedBy:    "me",
					NoExpiration: true,
					SleepSchedule: host.SleepScheduleInfo{
						NextStartTime: time.Now().Add(time.Minute),
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
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = testutil.TestSpan(ctx, t)

			require.NoError(t, db.ClearCollections(host.Collection))
			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			j, ok := NewSleepSchedulerJob(env, "ts").(*sleepSchedulerJob)
			require.True(t, ok)

			tCase(ctx, t, env, j)
		})
	}
}
