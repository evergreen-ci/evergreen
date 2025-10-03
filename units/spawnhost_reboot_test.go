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
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnhostRebootJob(t *testing.T) {
	ctx := testutil.TestSpan(t.Context(), t)
	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection, event.EventCollection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, mock cloud.MockProvider){
		"NewSpawnhostRebootJobSetsExpectedFields": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			h := host.Host{
				Id:        "host_id",
				Status:    evergreen.HostRunning,
				Provider:  evergreen.ProviderNameMock,
				Distro:    distro.Distro{Provider: evergreen.ProviderNameMock},
				StartedBy: "me",
			}
			j, ok := NewSpawnhostRebootJob(SpawnHostModifyJobOptions{
				Host:      &h,
				Source:    evergreen.ModifySpawnHostManual,
				User:      "user",
				Timestamp: ts,
			}).(*spawnhostRebootJob)
			require.True(t, ok)

			assert.NotZero(t, j.RetryInfo().GetMaxAttempts(), "job should retry")
			assert.Equal(t, h.Id, j.HostID)
			assert.Equal(t, "user", j.UserID)
			assert.Equal(t, evergreen.ModifySpawnHostManual, j.Source)
		},
		"RunRebootRunningHost": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			h := host.Host{
				Id:        "host-running",
				Status:    evergreen.HostRunning,
				Provider:  evergreen.ProviderNameMock,
				Distro:    distro.Distro{Provider: evergreen.ProviderNameMock},
				StartedBy: "me",
			}
			assert.NoError(t, h.Insert(ctx))
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewSpawnhostRebootJob(SpawnHostModifyJobOptions{
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

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostRebooted, true)
		},
		"RunErrorsIfHostCannotBeRebooted": func(ctx context.Context, t *testing.T, mock cloud.MockProvider) {
			h := host.Host{
				Id:        "host-uninitialized",
				Status:    evergreen.HostUninitialized,
				Provider:  evergreen.ProviderNameMock,
				Distro:    distro.Distro{Provider: evergreen.ProviderNameMock},
				StartedBy: "me",
			}
			assert.NoError(t, h.Insert(ctx))
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusInitializing,
			})

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewSpawnhostRebootJob(SpawnHostModifyJobOptions{
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
			assert.True(t, j.Status().Completed)

			dbHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostUninitialized, dbHost.Status)

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostRebooted, false)
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
