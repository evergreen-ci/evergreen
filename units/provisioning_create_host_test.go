package units

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestProvisioningCreateHostJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host){
		"PopulatesFields": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			j := NewHostCreateJob(env, *h, "job-id", true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)

			assert.Equal(t, env, hostCreateJob.env)
			assert.Equal(t, h.Id, hostCreateJob.HostID)
			require.NotZero(t, hostCreateJob.host)
			assert.Equal(t, *h, *hostCreateJob.host)
		},
		"SucceedsForHostCreate": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			require.NoError(t, h.Insert(ctx))
			j := NewHostCreateJob(env, *h, "job-id", true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)
			attrs := runCreateHostJobWithRecordedSpan(ctx, t, hostCreateJob)
			assert.False(t, hostCreateJob.HasErrors())
			foundHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostStarting, foundHost.Status)
			assert.Equal(t, "spawned", attrs[createHostOutcomeOtelAttribute])
			assert.Equal(t, "complete", attrs[createHostStageOtelAttribute])
			assert.Equal(t, h.Id, attrs[createHostIntentHostIDOtelAttribute])
			assert.Equal(t, h.Id, attrs[evergreen.HostIDOtelAttribute])
			assert.Equal(t, evergreen.HostStarting, attrs[createHostFinalStatusOtelAttribute])
			assert.Equal(t, evergreen.ProviderNameMock, attrs[evergreen.DistroProviderOtelAttribute])
			assert.Equal(t, true, attrs[createHostSpawnedOtelAttribute])
			assert.Equal(t, true, attrs[createHostReplacedOtelAttribute])
		},
		"NoopsForTerminatedHost": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))

			j := NewHostCreateJob(env, *h, "job-id", true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)
			attrs := runCreateHostJobWithRecordedSpan(ctx, t, hostCreateJob)

			assert.False(t, hostCreateJob.HasErrors())
			foundHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostTerminated, foundHost.Status)
			assert.Equal(t, "already_started", attrs[createHostOutcomeOtelAttribute])
			assert.Equal(t, "validate_host", attrs[createHostStageOtelAttribute])
			assert.Equal(t, evergreen.HostTerminated, attrs[createHostInitialStatusOtelAttribute])
			assert.Equal(t, evergreen.HostTerminated, attrs[createHostFinalStatusOtelAttribute])
			assert.Equal(t, false, attrs[createHostSpawnedOtelAttribute])
		},
		"RecordsMissingHostIntent": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			j := makeCreateHostJob()
			j.HostID = h.Id
			j.env = env
			j.SetID("missing-host-intent")

			attrs := runCreateHostJobWithRecordedSpan(ctx, t, j)

			assert.NoError(t, j.Error())
			assert.Equal(t, "host_missing", attrs[createHostOutcomeOtelAttribute])
			assert.Equal(t, "load_host", attrs[createHostStageOtelAttribute])
			assert.Equal(t, h.Id, attrs[createHostIntentHostIDOtelAttribute])
			assert.NotContains(t, attrs, evergreen.HostIDOtelAttribute)
		},
		"RecordsCloudManagerFailure": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			h.Distro.Provider = "invalid-provider"
			j := NewHostCreateJob(env, *h, "job-id", true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)

			attrs := runCreateHostJobWithRecordedSpan(ctx, t, hostCreateJob)

			assert.Error(t, hostCreateJob.Error())
			assert.Equal(t, "failed", attrs[createHostOutcomeOtelAttribute])
			assert.Equal(t, "get_cloud_manager", attrs[createHostStageOtelAttribute])
		},
		"RecordsWaitingForContainerImage": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			parent := host.Host{Id: "parent", Status: evergreen.HostStarting, HasContainers: true}
			require.NoError(t, parent.Insert(ctx))
			h.ParentID = parent.Id
			require.NoError(t, h.Insert(ctx))

			j := NewHostCreateJob(env, *h, "job-id", true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)
			attrs := runCreateHostJobWithRecordedSpan(ctx, t, hostCreateJob)

			assert.NoError(t, hostCreateJob.Error())
			assert.Equal(t, "waiting_for_image", attrs[createHostOutcomeOtelAttribute])
			assert.Equal(t, "wait_for_image", attrs[createHostStageOtelAttribute])
			assert.Equal(t, parent.Id, attrs[createHostParentIDOtelAttribute])
			assert.Equal(t, true, attrs[createHostRetryOtelAttribute])
		},
		"ThrottlesIntentHostAtGlobalMaxDynamicHosts": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			const maxHosts = 50

			// Create enough active task hosts to exceed the global max dynamic
			// hosts limit, meaning any more host creation should be throttled.
			statuses := []string{evergreen.HostDecommissioned, evergreen.HostStarting, evergreen.HostRunning}
			for i := 0; i < maxHosts+1; i++ {
				statusIdx := i % len(statuses)
				taskHost := host.Host{
					Id:        fmt.Sprintf("task_host_%d", i),
					Distro:    h.Distro,
					Provider:  evergreen.ProviderNameEc2Fleet,
					StartedBy: evergreen.User,
					Status:    statuses[statusIdx],
				}
				require.NoError(t, taskHost.Insert(ctx))
			}

			h.UserHost = false
			require.NoError(t, h.Insert(ctx))
			assert.True(t, h.IsSubjectToHostCreationThrottle(), "host should respect host creation throttle")

			env.Settings().HostInit.MaxTotalDynamicHosts = maxHosts

			j := NewHostCreateJob(env, *h, "", true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)
			attrs := runCreateHostJobWithRecordedSpan(ctx, t, hostCreateJob)
			assert.NoError(t, j.Error())

			foundHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Zero(t, foundHost, "host creation should have been throttled due to hitting global dynamic max hosts")
			assert.Equal(t, "throttled", attrs[createHostOutcomeOtelAttribute])
			assert.Equal(t, "check_throttle", attrs[createHostStageOtelAttribute])
			assert.Equal(t, true, attrs[createHostDistroLimitOtelAttribute])
			assert.EqualValues(t, maxHosts+1, attrs[createHostDistroActiveOtelAttribute])
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, distro.Collection))
			tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			tctx = testutil.TestSpan(tctx, t)

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))

			h := host.Host{
				Id:     "id",
				Status: evergreen.HostUninitialized,
				Distro: distro.Distro{
					Id: "distro-id",
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.BootstrapMethodSSH,
					},
					Arch: evergreen.ArchLinuxAmd64,
				},
				Host:     "localhost",
				User:     evergreen.User,
				UserHost: true,
			}
			d := distro.Distro{
				Id:       "distro-id",
				Provider: evergreen.ProviderNameMock,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(
					birch.EC.String("region", evergreen.DefaultEC2Region),
					birch.EC.String("aws_access_key_id", "key"),
					birch.EC.String("aws_secret_access_key", "secret"),
				)},
			}
			require.NoError(t, d.Insert(tctx))
			h.Distro = d
			testCase(tctx, t, env, &h)
		})
	}
}

func runCreateHostJobWithRecordedSpan(ctx context.Context, t *testing.T, j *createHostJob) map[string]any {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	spanCtx, span := tp.Tracer("test").Start(ctx, t.Name())
	j.Run(spanCtx)
	span.End()
	require.NoError(t, tp.Shutdown(t.Context()))

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	attrs := map[string]any{}
	for _, attr := range spans[0].Attributes() {
		attrs[string(attr.Key)] = attr.Value.AsInterface()
	}
	return attrs
}
