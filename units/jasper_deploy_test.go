package units

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/credentials"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	jaspercli "github.com/mongodb/jasper/cli"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupJasperService creates a Jasper service with credentials for testing.
func setupJasperService(ctx context.Context, env *mock.Environment, mngr *jasper.MockManager, h *host.Host) (jasper.CloseFunc, error) {
	if _, err := h.Upsert(); err != nil {
		return nil, errors.WithStack(err)
	}
	port := testutil.NextPort()
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	env.Settings().HostJasper.Port = port

	creds, err := h.GenerateJasperCredentials(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	closeService, err := rpc.StartService(ctx, mngr, addr, creds)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return closeService, errors.WithStack(h.SaveJasperCredentials(ctx, creds))
}

// setupCredentialsCollection is used to bootstrap the credentials collection
// for testing.
func setupCredentialsCollection(ctx context.Context, env *mock.Environment) error {
	env.Settings().DomainName = "test-service"

	if err := db.ClearCollections(credentials.Collection, host.Collection); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(credentials.Bootstrap(env))
}

func teardownJasperService(closeService jasper.CloseFunc) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(db.ClearCollections(credentials.Collection, host.Collection))
	if closeService != nil {
		catcher.Add(closeService())
	}
	return catcher.Resolve()
}

func getJasperDeployJobName(hostID string, deployThroughJasper bool, jobID string) string {
	id := fmt.Sprintf("%s.%s.%s", jasperDeployJobName, hostID, jobID)
	if deployThroughJasper {
		id += ".deploy-through-jasper"
	}
	return id
}

// withJasperServiceSetupAndTeardown performs necessary setup to start a
// Jasper RPC service, executes the given test function, and cleans up.
func withJasperServiceSetupAndTeardown(ctx context.Context, env *mock.Environment, manager *jasper.MockManager, h *host.Host, fn func(evergreen.Environment)) error {
	if err := setupCredentialsCollection(ctx, env); err != nil {
		grip.Error(errors.Wrap(teardownJasperService(nil), "problem tearing down test"))
		return errors.Wrap(err, "problem setting up credentials collection")
	}

	closeService, err := setupJasperService(ctx, env, manager, h)
	if err != nil {
		grip.Error(errors.Wrap(teardownJasperService(closeService), "problem tearing down test"))
		return errors.Wrap(err, "problem setting up Jasper service")
	}

	fn(env)

	return errors.Wrap(teardownJasperService(closeService), "problem tearing down test")
}

func TestJasperDeployJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jasper.MockManager, h *host.Host){
		"NewJasperDeployJobPopulatesFields": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jasper.MockManager, h *host.Host) {
			expiration := time.Now()

			j := NewJasperDeployJob(env, h, expiration, true, "attempt-0")
			deployJob, ok := j.(*jasperDeployJob)
			require.True(t, ok)

			assert.Equal(t, h.Id, deployJob.HostID)
			assert.True(t, deployJob.DeployThroughJasper)
			assert.Equal(t, expiration, deployJob.CredentialsExpiration)
			assert.Equal(t, getJasperDeployJobName(h.Id, true, "attempt-0"), deployJob.ID())
		},
		"RequeueJobRedeploysThroughJasperIfAttemptsRemainingAndCredentialsNotExpiring": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jasper.MockManager, h *host.Host) {
			expiration := time.Now().Add(24 * time.Hour)

			j := NewJasperDeployJob(env, h, expiration, true, "attempt-0")
			deployJob, ok := j.(*jasperDeployJob)
			require.True(t, ok)

			require.NoError(t, deployJob.tryRequeueDeploy(ctx))

			newJob, ok := env.RemoteQueue().Get(ctx, getJasperDeployJobName(h.Id, true, "attempt-1"))
			require.True(t, ok)
			require.NotNil(t, newJob)
			newDeployJob, ok := newJob.(*jasperDeployJob)
			require.True(t, ok)
			require.NotNil(t, newDeployJob)

			assert.True(t, newDeployJob.DeployThroughJasper)
		},
		"RequeueJobDoesNotDeployThroughJasperIfCredentialsExpiring": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jasper.MockManager, h *host.Host) {
			expiration := time.Now()

			j := NewJasperDeployJob(env, h, expiration, true, "attempt-0")
			deployJob, ok := j.(*jasperDeployJob)
			require.True(t, ok)

			require.NoError(t, deployJob.tryRequeueDeploy(ctx))

			newJob, ok := env.RemoteQueue().Get(ctx, getJasperDeployJobName(h.Id, false, "attempt-0"))
			require.True(t, ok)
			require.NotNil(t, newJob)
			newDeployJob, ok := newJob.(*jasperDeployJob)
			require.True(t, ok)
			require.NotNil(t, newDeployJob)

			assert.False(t, newDeployJob.DeployThroughJasper)
		},
		"ChecksAttemptsBeforeRequeueingWithJasperDeploy": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jasper.MockManager, h *host.Host) {
			expiration := time.Now().Add(24 * time.Hour)

			h.JasperDeployAttempts = jasperDeployRetryLimit
			_, err := h.Upsert()
			require.NoError(t, err)

			j := NewJasperDeployJob(env, h, expiration, true, fmt.Sprintf("attempt-%d", jasperDeployRetryLimit))
			deployJob, ok := j.(*jasperDeployJob)
			require.True(t, ok)

			require.NoError(t, deployJob.tryRequeueDeploy(ctx))

			_, ok = env.RemoteQueue().Get(ctx, getJasperDeployJobName(h.Id, true, fmt.Sprintf("attempt-%d", jasperDeployRetryLimit+1)))
			assert.False(t, ok)

			newJob, ok := env.RemoteQueue().Get(ctx, getJasperDeployJobName(h.Id, false, "attempt-0"))
			require.True(t, ok)
			newDeployJob, ok := newJob.(*jasperDeployJob)
			require.True(t, ok)
			require.NotNil(t, newDeployJob)

			assert.False(t, newDeployJob.DeployThroughJasper)
		},
		"DoesNotRequeueIfNoAttemptsRemainingAndNotDeployingThroughJasper": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jasper.MockManager, h *host.Host) {
			expiration := time.Now()

			h.JasperDeployAttempts = jasperDeployRetryLimit
			_, err := h.Upsert()
			require.NoError(t, err)

			j := NewJasperDeployJob(env, h, expiration, false, fmt.Sprintf("attempt-%d", jasperDeployRetryLimit))
			deployJob, ok := j.(*jasperDeployJob)
			require.True(t, ok)

			assert.Error(t, deployJob.tryRequeueDeploy(ctx))
		},
		"RunPerformsExpectedOperationsWhenDeployingThroughJasper": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jasper.MockManager, h *host.Host) {
			clientCreds, err := credentials.ForJasperClient(ctx)
			require.NoError(t, err)

			creds, err := h.JasperClientCredentials(ctx)
			require.NoError(t, err)

			assert.Equal(t, clientCreds.Cert, creds.Cert)
			assert.Equal(t, clientCreds.Key, creds.Key)
			assert.Equal(t, clientCreds.CACert, creds.CACert)
			assert.Equal(t, h.Id, h.JasperCredentialsID)
			assert.Equal(t, h.JasperCredentialsID, creds.ServerName)

			expiration := time.Now().Add(24 * time.Hour)

			j := NewJasperDeployJob(env, h, expiration, true, "attempt-0")
			j.Run(ctx)

			deployJob, ok := j.(*jasperDeployJob)
			require.True(t, ok)

			// Job will error because mock service will not be restarted.
			assert.True(t, deployJob.HasErrors())

			// Job updates LCT.
			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.WithinDuration(t, time.Now(), dbHost.LastCommunicationTime, time.Minute)

			// Jasper service received expected commands to update credentials
			// and restart service.

			require.Len(t, mngr.Procs, 2)

			writeCredsCmd := fmt.Sprintf("cat > '%s'", h.Distro.BootstrapSettings.JasperCredentialsPath)
			writeCredentialsProc := mngr.Procs[0]

			var writeCredsCmdFound bool
			for _, arg := range writeCredentialsProc.Info(ctx).Options.Args {
				if strings.Contains(arg, writeCredsCmd) {
					writeCredsCmdFound = true
					break
				}
			}
			assert.True(t, writeCredsCmdFound)

			killJasperCmd := fmt.Sprintf("pgrep -f '%s' | xargs kill", strings.Join(jaspercli.BuildServiceCommand(env.Settings().HostJasper.BinaryName), " "))
			killJasperProc := mngr.Procs[1]
			var killJasperCmdFound bool
			for _, arg := range killJasperProc.Info(ctx).Options.Args {
				if strings.Contains(arg, killJasperCmd) {
					killJasperCmdFound = true
					break
				}
			}
			assert.True(t, killJasperCmdFound)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			mngr := &jasper.MockManager{}
			mngr.ManagerID = "mock-manager-id"

			h := &host.Host{
				Id:   "host_id",
				Host: "localhost",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:                distro.BootstrapMethodUserData,
						Communication:         distro.CommunicationMethodRPC,
						JasperCredentialsPath: "/etc/creds.pem",
					},
					SSHKey: "/etc/mci.pem",
				},
			}
			_, err := h.Upsert()
			require.NoError(t, err)

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx, "", nil))

			require.NoError(t, withJasperServiceSetupAndTeardown(tctx, env, mngr, h, func(env evergreen.Environment) {
				testCase(tctx, t, env, mngr, h)
			}))
		})
	}
}
