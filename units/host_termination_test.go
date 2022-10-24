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
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host){
		"TerminatesRunningHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			require.NoError(t, h.Insert())
			mcp.Set(h.Id, cloud.MockInstance{
				IsUp:   true,
				Status: cloud.StatusRunning,
			})

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "some termination message",
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)

			events, err := event.Find(event.MostRecentHostEvents(h.Id, "", 50))
			require.NoError(t, err)
			require.NotEmpty(t, events)
			assert.Equal(t, event.EventHostStatusChanged, events[0].EventType)
			data, ok := events[0].Data.(*event.HostEventData)
			require.True(t, ok)
			assert.Equal(t, "some termination message", data.Logs)

			cloudHost := mcp.Get(h.Id)
			require.NotZero(t, cloudHost)
			assert.Equal(t, cloud.StatusTerminated, cloudHost.Status)
		},
		"SkipsCloudHostTermination": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			require.NoError(t, h.Insert())
			mcp.Set(h.Id, cloud.MockInstance{
				IsUp:   true,
				Status: cloud.StatusRunning,
			})

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:          true,
				SkipCloudHostTermination: true,
				TerminationReason:        "some termination message",
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)

			events, err := event.Find(event.MostRecentHostEvents(h.Id, "", 50))
			require.NoError(t, err)
			require.NotEmpty(t, events)
			assert.Equal(t, event.EventHostStatusChanged, events[0].EventType)
			data, ok := events[0].Data.(*event.HostEventData)
			require.True(t, ok)
			assert.Equal(t, "some termination message", data.Logs)

			cloudHost := mcp.Get(h.Id)
			require.NotZero(t, cloudHost)
			assert.Equal(t, cloud.StatusRunning, cloudHost.Status, "cloud host should be unchanged because cloud host termination should be skipped")
		},
		"NoopsForStaticHosts": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Provider = evergreen.ProviderNameStatic
			h.Distro.Provider = evergreen.ProviderNameStatic
			require.NoError(t, h.Insert())

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "foo",
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(host.ById(h.Id))
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
			require.NoError(t, h.Insert())
			mcp.Set(h.Id, cloud.MockInstance{
				IsUp:   true,
				Status: cloud.StatusRunning,
			})

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "foo",
			})
			j.Run(ctx)
			require.NoError(t, j.Error())
		},
		"TerminatesDBHostWithoutCloudHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			require.NoError(t, h.Insert())

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "foo",
			})
			j.Run(ctx)
			require.Error(t, j.Error())

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.NotEqual(t, evergreen.HostRunning, dbHost.Status)
		},
		"MarksUninitializedIntentHostAsTerminated": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostUninitialized
			require.NoError(t, h.Insert())

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "foo",
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"MarksBuildingFailedIntentHostAsTerminated": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostBuildingFailed
			require.NoError(t, h.Insert())

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "foo",
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"NoopsWithAlreadyTerminatedIntentHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, mcp cloud.MockProvider, h *host.Host) {
			// The ID must be a valid intent host ID.
			h.Id = h.Distro.GenerateName()
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert())

			j := NewHostTerminationJob(env, h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "foo",
			})
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, event.EventCollection))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

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
