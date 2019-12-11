package units

import (
	"context"
	"fmt"
	"math"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/mongodb/grip"
	jcli "github.com/mongodb/jasper/cli"
	jmock "github.com/mongodb/jasper/mock"
	"github.com/mongodb/jasper/remote"
	jutil "github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupJasperService creates a Jasper service with credentials for testing.
func setupJasperService(ctx context.Context, env *mock.Environment, mngr *jmock.Manager, h *host.Host) (jutil.CloseFunc, error) {
	if _, err := h.Upsert(); err != nil {
		return nil, errors.WithStack(err)
	}
	port := testutil.NextPort()
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	env.Settings().HostJasper.Port = port

	creds, err := h.GenerateJasperCredentials(ctx, env)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	closeService, err := remote.StartRPCService(ctx, mngr, addr, creds)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return closeService, errors.WithStack(h.SaveJasperCredentials(ctx, env, creds))
}

// setupHostCredentials is used to bootstrap the credentials collection
// for testing.
func setupHostCredentials(ctx context.Context, env *mock.Environment) error {
	settings := env.Settings()
	settings.DomainName = "test-service"

	if err := db.ClearCollections(evergreen.CredentialsCollection, host.Collection); err != nil {
		return errors.WithStack(err)
	}

	maxExpiration := time.Duration(math.MaxInt64)

	bootstrapConfig := certdepot.BootstrapDepotConfig{
		CAName: evergreen.CAName,
		MongoDepot: &certdepot.MongoDBOptions{
			DatabaseName:   settings.Database.DB,
			CollectionName: evergreen.CredentialsCollection,
			DepotOptions: certdepot.DepotOptions{
				CA:                evergreen.CAName,
				DefaultExpiration: 365 * 24 * time.Hour,
			},
		},
		CAOpts: &certdepot.CertificateOptions{
			CA:         evergreen.CAName,
			CommonName: evergreen.CAName,
			Expires:    maxExpiration,
		},
		ServiceName: settings.DomainName,
		ServiceOpts: &certdepot.CertificateOptions{
			CA:         evergreen.CAName,
			CommonName: settings.DomainName,
			Host:       settings.DomainName,
			Expires:    maxExpiration,
		},
	}

	var err error
	env.Depot, err = certdepot.BootstrapDepotWithMongoClient(ctx, env.Client(), bootstrapConfig)

	return errors.WithStack(err)
}

func teardownJasperService(closeService jutil.CloseFunc) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(teardownHostCredentials())
	if closeService != nil {
		catcher.Add(closeService())
	}
	return catcher.Resolve()
}

func teardownHostCredentials() error {
	return db.ClearCollections(evergreen.CredentialsCollection, host.Collection)
}

func getJasperRestartJobName(hostID string, restartThroughJasper bool, ts string, attempt int) string {
	id := fmt.Sprintf("%s.%s.%s.attempt-%d", jasperRestartJobName, hostID, ts, attempt)
	if restartThroughJasper {
		id += ".restart-through-jasper"
	}
	return id
}

// withJasperServiceSetupAndTeardown performs necessary setup to start a
// Jasper RPC service, executes the given test function, and cleans up.
func withJasperServiceSetupAndTeardown(ctx context.Context, env *mock.Environment, manager *jmock.Manager, h *host.Host, fn func(evergreen.Environment)) error {
	if err := setupHostCredentials(ctx, env); err != nil {
		grip.Error(errors.Wrap(teardownHostCredentials(), "problem tearing down test"))
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

func TestJasperRestartJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host){
		"NewJasperRestartJobPopulatesFields": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host) {
			expiration := time.Now()
			ts := time.Now().Format(TSFormat)

			j := NewJasperRestartJob(env, *h, expiration, true, ts, 5)
			restartJob, ok := j.(*jasperRestartJob)
			require.True(t, ok)

			assert.Equal(t, h.Id, restartJob.HostID)
			assert.True(t, restartJob.RestartThroughJasper)
			assert.Equal(t, expiration, restartJob.CredentialsExpiration)
			assert.Equal(t, 5, restartJob.CurrentAttempt)
			assert.Equal(t, ts, restartJob.Timestamp)
			assert.Equal(t, getJasperRestartJobName(h.Id, true, ts, 5), restartJob.ID())
		},
		"RequeueJobRestartsThroughJasperIfAttemptsRemainingAndCredentialsNotExpiring": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host) {
			expiration := time.Now().Add(24 * time.Hour)
			ts := time.Now().Format(TSFormat)

			j := NewJasperRestartJob(env, *h, expiration, true, ts, 0)
			restartJob, ok := j.(*jasperRestartJob)
			require.True(t, ok)

			require.NoError(t, restartJob.tryRequeue(ctx))

			newJob, ok := env.RemoteQueue().Get(ctx, getJasperRestartJobName(h.Id, true, ts, 1))
			require.True(t, ok)
			require.NotNil(t, newJob)
			newRestartJob, ok := newJob.(*jasperRestartJob)
			require.True(t, ok)
			require.NotNil(t, newRestartJob)

			assert.True(t, newRestartJob.RestartThroughJasper)
		},
		"RequeueJobDoesNotRestartThroughJasperIfCredentialsExpiring": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host) {
			expiration := time.Now()
			ts := time.Now().Format(TSFormat)

			j := NewJasperRestartJob(env, *h, expiration, true, ts, 0)
			restartJob, ok := j.(*jasperRestartJob)
			require.True(t, ok)

			require.NoError(t, restartJob.tryRequeue(ctx))

			newJob, ok := env.RemoteQueue().Get(ctx, getJasperRestartJobName(h.Id, false, ts, 0))
			require.True(t, ok)
			require.NotNil(t, newJob)
			newRestartJob, ok := newJob.(*jasperRestartJob)
			require.True(t, ok)
			require.NotNil(t, newRestartJob)

			assert.False(t, newRestartJob.RestartThroughJasper)
		},
		"ChecksAttemptsBeforeRequeueingWithJasperRestart": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host) {
			expiration := time.Now().Add(24 * time.Hour)
			ts := time.Now().Format(TSFormat)

			j := NewJasperRestartJob(env, *h, expiration, true, ts, jasperRestartRetryLimit+1)
			restartJob, ok := j.(*jasperRestartJob)
			require.True(t, ok)

			require.NoError(t, restartJob.tryRequeue(ctx))

			_, ok = env.RemoteQueue().Get(ctx, getJasperRestartJobName(h.Id, true, ts, jasperRestartRetryLimit+1))
			assert.False(t, ok)

			newJob, ok := env.RemoteQueue().Get(ctx, getJasperRestartJobName(h.Id, false, ts, 0))
			require.True(t, ok)
			newRestartJob, ok := newJob.(*jasperRestartJob)
			require.True(t, ok)
			require.NotNil(t, newRestartJob)

			assert.False(t, newRestartJob.RestartThroughJasper)
		},
		"DoesNotRequeueIfNoAttemptsRemainingAndNotRestartingThroughJasper": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host) {
			expiration := time.Now()
			ts := time.Now().Format(TSFormat)

			j := NewJasperRestartJob(env, *h, expiration, false, ts, jasperRestartRetryLimit+1)
			restartJob, ok := j.(*jasperRestartJob)
			require.True(t, ok)

			assert.Error(t, restartJob.tryRequeue(ctx))
		},
		"RunPerformsExpectedOperationsWhenRestartingThroughJasper": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host) {
			clientCreds, err := env.CertificateDepot().Find(env.Settings().DomainName)
			require.NoError(t, err)

			creds, err := h.JasperClientCredentials(ctx, env)
			require.NoError(t, err)

			assert.Equal(t, clientCreds.Cert, creds.Cert)
			assert.Equal(t, clientCreds.Key, creds.Key)
			assert.Equal(t, clientCreds.CACert, creds.CACert)
			assert.Equal(t, h.Id, h.JasperCredentialsID)
			assert.Equal(t, h.JasperCredentialsID, creds.ServerName)

			expiration := time.Now().Add(24 * time.Hour)
			ts := time.Now().Format(TSFormat)

			j := NewJasperRestartJob(env, *h, expiration, true, ts, 0)
			j.Run(ctx)

			restartJob, ok := j.(*jasperRestartJob)
			require.True(t, ok)

			// Job will error because mock service will not be restarted.
			assert.True(t, restartJob.HasErrors())

			// Job updates LCT.
			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			assert.WithinDuration(t, time.Now(), dbHost.LastCommunicationTime, time.Minute)

			// Jasper service received expected commands to update credentials
			// and restart service.

			require.Len(t, mngr.Procs, 2)

			writeCredentialsProc := mngr.Procs[0]

			var writeCredsCmdFound bool
			for _, arg := range writeCredentialsProc.Info(ctx).Options.Args {
				if strings.Contains(arg, "echo") && strings.Contains(arg, fmt.Sprintf("> '%s'", h.Distro.BootstrapSettings.JasperCredentialsPath)) {
					writeCredsCmdFound = true
					break
				}
			}
			assert.True(t, writeCredsCmdFound)

			killJasperCmd := fmt.Sprintf("pgrep -f '%s' | xargs kill", strings.Join(jcli.BuildServiceCommand(env.Settings().HostJasper.BinaryName), " "))
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

			mngr := &jmock.Manager{}
			mngr.ManagerID = "mock-manager-id"

			h := &host.Host{
				Id:   "host_id",
				Host: "localhost",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:                distro.BootstrapMethodUserData,
						Communication:         distro.CommunicationMethodRPC,
						JasperCredentialsPath: "/jasper_credentials_path",
					},
					SSHKey: "ssh_key",
				},
				Status:               evergreen.HostProvisioning,
				NeedsReprovision:     host.ReprovisionJasperRestart,
				NeedsNewAgentMonitor: true,
			}

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))

			require.NoError(t, withJasperServiceSetupAndTeardown(tctx, env, mngr, h, func(env evergreen.Environment) {
				testCase(tctx, t, env, mngr, h)
			}))
		})
	}
}
