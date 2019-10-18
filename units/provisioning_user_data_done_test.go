package units

import (
	"context"
	"fmt"
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

func TestUserDataDoneJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host){
		"NewUserDataSpawnHostReadyJobPopulatesFields": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host) {
			_, err := h.Upsert()
			require.NoError(t, err)

			j := NewUserDataDoneJob(env, *h, "id")
			readyJob, ok := j.(*userDataDoneJob)
			require.True(t, ok)

			assert.Equal(t, h.Id, readyJob.HostID)
		},
		"RunNoopsIfHostNotProvisioning": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host) {
			require.NoError(t, h.SetRunning(evergreen.User))

			j := NewUserDataDoneJob(env, *h, "id")
			j.Run(ctx)
			require.NoError(t, j.Error())

			assert.Empty(t, mngr.Procs)
		},
		"RunFailsWithoutPathToFile": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host) {
			h.Distro.BootstrapSettings.ClientDir = ""
			_, err := h.Upsert()
			require.NoError(t, err)

			j := NewUserDataDoneJob(env, *h, "id")
			j.Run(ctx)
			assert.Error(t, j.Error())
		},
		"RunChecksForPathToFile": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host) {
			j := NewUserDataDoneJob(env, *h, "id")
			j.Run(ctx)
			require.NoError(t, j.Error())

			require.Len(t, mngr.Procs, 1)
			info := mngr.Procs[0].Info(ctx)

			path, err := h.UserDataDoneFilePath()
			require.NoError(t, err)

			expectedCmd := []string{h.Distro.BootstrapSettings.ShellPath, "-l", "-c", fmt.Sprintf("ls %s", path)}
			require.Equal(t, len(expectedCmd), len(info.Options.Args))
			assert.Equal(t, expectedCmd, info.Options.Args)

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

			mngr := &jmock.Manager{}

			h := &host.Host{
				Id:   "host_id",
				Host: "localhost",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodUserData,
						Communication: distro.CommunicationMethodRPC,
						ClientDir:     "/client_dir",
						ShellPath:     "/shell_path",
					},
				},
				Status:      evergreen.HostProvisioning,
				Provisioned: true,
			}
			require.NoError(t, withJasperServiceSetupAndTeardown(tctx, env, mngr, h, func(env evergreen.Environment) {
				testCase(tctx, t, env, mngr, h)
			}))
		})
	}
}
