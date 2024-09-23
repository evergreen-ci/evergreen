package units

import (
	"context"
	"fmt"
	"math"
	"net"
	"testing"
	"time"

	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	serviceTestutil "github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	jmock "github.com/mongodb/jasper/mock"
	"github.com/mongodb/jasper/remote"
	jutil "github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupJasperService creates a Jasper service with credentials for testing.
func setupJasperService(ctx context.Context, env *mock.Environment, mngr *jmock.Manager, h *host.Host) (jutil.CloseFunc, error) {
	if _, err := h.Upsert(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	port := serviceTestutil.NextPort()
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
				DefaultExpiration: maxExpiration,
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

// withJasperServiceSetupAndTeardown performs necessary setup to start a
// Jasper RPC service, executes the given test function, and cleans up.
func withJasperServiceSetupAndTeardown(ctx context.Context, env *mock.Environment, manager *jmock.Manager, h *host.Host, fn func(evergreen.Environment)) error {
	if err := setupHostCredentials(ctx, env); err != nil {
		grip.Error(errors.Wrap(teardownHostCredentials(), "tearing down test"))
		return errors.Wrap(err, "setting up credentials collection")
	}

	closeService, err := setupJasperService(ctx, env, manager, h)
	if err != nil {
		grip.Error(errors.Wrap(teardownJasperService(closeService), "tearing down test"))
		return errors.Wrap(err, "setting up Jasper service")
	}

	fn(env)

	return errors.Wrap(teardownJasperService(closeService), "tearing down test")
}

func TestRestartJasperJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host){
		"NewRestartJasperJobPopulatesFields": func(ctx context.Context, t *testing.T, env evergreen.Environment, mngr *jmock.Manager, h *host.Host) {
			ts := time.Now().Format(TSFormat)

			j := NewRestartJasperJob(env, *h, ts)
			restartJob, ok := j.(*restartJasperJob)
			require.True(t, ok)

			assert.Equal(t, h.Id, restartJob.HostID)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			tctx = testutil.TestSpan(tctx, t)

			mngr := &jmock.Manager{}
			mngr.ManagerID = "mock-manager-id"

			h := &host.Host{
				Id:       "host_id",
				Host:     "localhost",
				Provider: evergreen.ProviderNameStatic,
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:                distro.BootstrapMethodUserData,
						Communication:         distro.CommunicationMethodRPC,
						JasperCredentialsPath: "/jasper_credentials_path",
						JasperBinaryDir:       "/jasper_binary_dir",
					},
					Arch: evergreen.ArchLinuxAmd64,
				},
				Status:               evergreen.HostProvisioning,
				NeedsReprovision:     host.ReprovisionRestartJasper,
				NeedsNewAgentMonitor: true,
			}

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))
			env.EvergreenSettings.HostJasper = evergreen.HostJasperConfig{
				BinaryName:       "binary_name",
				DownloadFileName: "download_file_name",
				URL:              "https://example.com",
				Version:          "version",
			}

			testCase(tctx, t, env, mngr, h)
		})
	}
}
