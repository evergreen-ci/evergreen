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
)

func TestProvisioningCreateHostJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host){
		"PopulatesFields": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			j := NewHostCreateJob(env, *h, "job-id", 0, true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)

			assert.Equal(t, env, hostCreateJob.env)
			assert.Equal(t, h.Id, hostCreateJob.HostID)
			require.NotZero(t, hostCreateJob.host)
			assert.Equal(t, *h, *hostCreateJob.host)
		},
		"SucceedsForHostCreate": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			require.NoError(t, h.Insert(ctx))
			j := NewHostCreateJob(env, *h, "job-id", 0, true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)
			hostCreateJob.Run(ctx)
			assert.False(t, hostCreateJob.HasErrors())
			foundHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostStarting, foundHost.Status)
		},
		"NoopsForTerminatedHost": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))

			j := NewHostCreateJob(env, *h, "job-id", 0, true)
			hostCreateJob, ok := j.(*createHostJob)
			require.True(t, ok)
			hostCreateJob.Run(ctx)

			assert.False(t, hostCreateJob.HasErrors())
			foundHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostTerminated, foundHost.Status)
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

			j := NewHostCreateJob(env, *h, "", 0, true)
			j.Run(ctx)
			assert.NoError(t, j.Error())

			foundHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Zero(t, foundHost, "host creation should have been throttled due to hitting global dynamic max hosts")
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
