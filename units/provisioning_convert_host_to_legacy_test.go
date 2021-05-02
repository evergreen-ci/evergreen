package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	jmock "github.com/mongodb/jasper/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertHostToLegacyProvisioningJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, mgr *jmock.Manager, h *host.Host){
		"PopulatesFields": func(ctx context.Context, t *testing.T, env *mock.Environment, mgr *jmock.Manager, h *host.Host) {
			j := NewConvertHostToLegacyProvisioningJob(env, *h, "job-id", 0)

			info := j.TimeInfo()
			assert.Equal(t, maxHostReprovisioningJobTime, info.MaxTime)

			convertJob, ok := j.(*convertHostToLegacyProvisioningJob)
			require.True(t, ok)

			assert.Equal(t, env, convertJob.env)
			assert.Equal(t, h.Id, convertJob.HostID)
			assert.Equal(t, *h, *convertJob.host)
		},
		"NoopsIfAgentIsUp": func(ctx context.Context, t *testing.T, env *mock.Environment, mgr *jmock.Manager, h *host.Host) {
			h.NeedsNewAgent = false
			require.NoError(t, h.Insert())

			j := NewConvertHostToLegacyProvisioningJob(env, *h, "job-id", 0)
			convertJob, ok := j.(*convertHostToLegacyProvisioningJob)
			require.True(t, ok)
			convertJob.Run(ctx)

			assert.True(t, convertJob.RetryInfo().NeedsRetry)
			assert.Empty(t, mgr.Procs)
		},
		"NoopsIfHostIsNotProvisioningOrRunning": func(ctx context.Context, t *testing.T, env *mock.Environment, mgr *jmock.Manager, h *host.Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert())

			j := NewConvertHostToLegacyProvisioningJob(env, *h, "job-id", 0)
			convertJob, ok := j.(*convertHostToLegacyProvisioningJob)
			require.True(t, ok)
			convertJob.Run(ctx)

			assert.False(t, convertJob.HasErrors())
			assert.Empty(t, mgr.Procs)
		},
		"NoopsIfDoesNotNeedLegacyProvisioning": func(ctx context.Context, t *testing.T, env *mock.Environment, mgr *jmock.Manager, h *host.Host) {
			h.NeedsReprovision = host.ReprovisionNone
			require.NoError(t, h.Insert())

			j := NewConvertHostToLegacyProvisioningJob(env, *h, "job-id", 0)
			convertJob, ok := j.(*convertHostToLegacyProvisioningJob)
			require.True(t, ok)
			convertJob.Run(ctx)

			assert.False(t, convertJob.HasErrors())
			assert.Empty(t, mgr.Procs)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))
			mgr := &jmock.Manager{}
			env.JasperProcessManager = mgr
			sshKeyName, sshKeyValue := "foo", "bar"
			env.Settings().Keys = map[string]string{sshKeyName: sshKeyValue}
			env.Settings().HostJasper = evergreen.HostJasperConfig{
				BinaryName: "binary",
			}

			require.NoError(t, setupHostCredentials(ctx, env))
			defer func() {
				assert.NoError(t, teardownHostCredentials())
			}()

			h := &host.Host{
				Id:               "id",
				Status:           evergreen.HostProvisioning,
				NeedsReprovision: host.ReprovisionToLegacy,
				NeedsNewAgent:    true,
				Distro: distro.Distro{
					Id: "distro-id",
					BootstrapSettings: distro.BootstrapSettings{
						Method:          distro.BootstrapMethodLegacySSH,
						Communication:   distro.CommunicationMethodLegacySSH,
						JasperBinaryDir: "/jasper_dir",
					},
					SSHKey: sshKeyName,
				},
				Host: "localhost",
				User: evergreen.User,
			}

			testCase(tctx, t, env, mgr, h)
		})
	}
}
