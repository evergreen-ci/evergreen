package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserDataDoneJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jasper.MockManager, h *host.Host){
		"NewUserDataSpawnHostReadyJobPopulatesFields": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jasper.MockManager, h *host.Host) {
			_, err := h.Upsert()
			require.NoError(t, err)

			j := NewUserDataDoneJob(env, *h, "id")
			readyJob, ok := j.(*userDataDoneJob)
			require.True(t, ok)

			assert.Equal(t, h.Id, readyJob.HostID)
		},
		"RunNoopsIfHostNotProvisioning": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jasper.MockManager, h *host.Host) {
			require.NoError(t, h.SetRunning(evergreen.User))

			j := NewUserDataDoneJob(env, *h, "id")
			j.Run(ctx)
			require.NoError(t, j.Error())

			assert.Empty(t, mngr.Procs)
		},
		"RunFailsWithoutPathToFile": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jasper.MockManager, h *host.Host) {
			h.Distro.ClientDir = ""
			_, err := h.Upsert()
			require.NoError(t, err)

			j := NewUserDataDoneJob(env, *h, "id")
			j.Run(ctx)
			assert.Error(t, j.Error())
		},
		"RunChecksForPathToFile": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jasper.MockManager, h *host.Host) {
			j := NewUserDataDoneJob(env, *h, "id")
			j.Run(ctx)
			require.NoError(t, j.Error())

			require.Len(t, mngr.Procs, 1)
			info := mngr.Procs[0].Info(ctx)

			path, err := h.UserDataDoneFilePath()
			require.NoError(t, err)

			expectedCmd := []string{"ls", path}
			require.Equal(t, len(expectedCmd), len(info.Options.Args))

			for i := range expectedCmd {
				assert.Equal(t, expectedCmd[i], info.Options.Args[i])
			}

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx, "", nil))
			env.Settings().HostJasper = evergreen.HostJasperConfig{}

			mngr := &jasper.MockManager{}

			h := &host.Host{
				Id:   "host_id",
				Host: "localhost",
				Distro: distro.Distro{
					ClientDir:           "/client_dir",
					BootstrapMethod:     distro.BootstrapMethodUserData,
					CommunicationMethod: distro.CommunicationMethodRPC,
				},
				Status:      evergreen.HostProvisioning,
				Provisioned: true,
			}
			require.NoError(t, withJasperServiceSetupAndTeardown(tctx, env, mngr, h, func() {
				testCase(tctx, t, env, mngr, h)
			}))
		})
	}
}
