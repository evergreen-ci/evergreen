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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostTerminationJob(t *testing.T) {
	checkTerminationEvent := func(t *testing.T, hostID, reason string) {
		events, err := event.Find(event.MostRecentHostEvents(hostID, "", 50))
		require.NoError(t, err)
		require.NotEmpty(t, events)
		var foundTerminationEvent bool
		for _, e := range events {
			if e.EventType != event.EventHostStatusChanged {
				continue
			}
			data, ok := e.Data.(*event.HostEventData)
			require.True(t, ok)
			if data.NewStatus != evergreen.HostTerminated {
				continue
			}

			assert.Equal(t, reason, data.Logs, "event log termination reason should match expected reason")

			foundTerminationEvent = true
		}
		assert.True(t, foundTerminationEvent, "expected host termination event to be logged")
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host){
		"TerminatesRunningHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			require.NoError(t, h.Insert(ctx))
			mcp.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			const reason = "some termination message"
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(ctx, host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)

			checkTerminationEvent(t, h.Id, reason)

			cloudHost := mcp.Get(h.Id)
			require.NotZero(t, cloudHost)
			assert.Equal(t, cloud.StatusTerminated, cloudHost.Status)
		},
		"SkipsCloudHostTermination": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			require.NoError(t, h.Insert(ctx))
			mcp.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			const reason = "some termination message"
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:          true,
				SkipCloudHostTermination: true,
				TerminationReason:        reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(ctx, host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)

			checkTerminationEvent(t, h.Id, reason)

			cloudHost := mcp.Get(h.Id)
			require.NotZero(t, cloudHost)
			assert.Equal(t, cloud.StatusRunning, cloudHost.Status, "cloud host should be unchanged because cloud host termination should be skipped")
		},
		"NoopsForStaticHosts": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Provider = evergreen.ProviderNameStatic
			h.Distro.Provider = evergreen.ProviderNameStatic
			require.NoError(t, h.Insert(ctx))

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "foo",
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(ctx, host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
		},
		"FailsWithNonexistentDBHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "foo",
			})
			terminationJob, ok := j.(*hostTerminationJob)
			require.True(t, ok)
			terminationJob.host = nil

			j.Run(ctx)
			assert.Error(t, j.Error())
		},
		"ReterminatesCloudHostIfAlreadyMarkedTerminated": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))
			mcp.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			const reason = "foo"
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			// Don't check the host events for termination since the host is
			// already in a terminated state.

			mockInstance := mcp.Get(h.Id)
			assert.Equal(t, cloud.StatusTerminated, mockInstance.Status)
		},
		"TerminatesDBHostWithoutCloudHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			require.NoError(t, h.Insert(ctx))

			const reason = "foo"
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTerminationEvent(t, h.Id, reason)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.NotEqual(t, evergreen.HostRunning, dbHost.Status)
		},
		"MarksUninitializedIntentHostAsTerminated": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostUninitialized
			require.NoError(t, h.Insert(ctx))

			const reason = "foo"
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTerminationEvent(t, h.Id, reason)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"MarksBuildingIntentHostAsTerminated": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostBuilding
			require.NoError(t, h.Insert(ctx))

			const reason = "foo"
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTerminationEvent(t, h.Id, reason)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"MarksBuildingFailedIntentHostAsTerminated": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostBuildingFailed
			require.NoError(t, h.Insert(ctx))

			const reason = "foo"
			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: reason,
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			checkTerminationEvent(t, h.Id, reason)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"NoopsWithAlreadyTerminatedIntentHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			// The ID must be a valid intent host ID.
			h.Id = h.Distro.GenerateName()
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "foo",
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, event.EventCollection))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = testutil.TestSpan(ctx, t)

			env := testutil.NewEnvironment(ctx, t)

			h := &host.Host{
				Id:          "i-12345",
				Status:      evergreen.HostRunning,
				Distro:      distro.Distro{Provider: evergreen.ProviderNameMock},
				Provider:    evergreen.ProviderNameMock,
				Provisioned: true,
			}

			provider := cloud.GetMockProvider()
			provider.Reset()

			tCase(ctx, t, env, provider, h)
		})
	}
}
