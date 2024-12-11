// Package mock provides a mock environment for testing.
package mock

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser/apm"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// this is just a hack to ensure that compile breaks clearly if the
// mock implementation diverges from the interface
var _ evergreen.Environment = &Environment{}

type Environment struct {
	Remote                  amboy.Queue
	Local                   amboy.Queue
	JasperProcessManager    jasper.Manager
	RemoteGroup             amboy.QueueGroup
	Depot                   certdepot.Depot
	ParamManager            *parameterstore.ParameterManager
	Closers                 map[string]func(context.Context) error
	DBSession               db.Session
	EvergreenSettings       *evergreen.Settings
	MongoClient             *mongo.Client
	mu                      sync.RWMutex
	DatabaseName            string
	EnvContext              context.Context
	InternalSender          *send.InternalSender
	roleManager             gimlet.RoleManager
	userManager             gimlet.UserManager
	userManagerInfo         evergreen.UserManagerInfo
	Clients                 evergreen.ClientConfig
	shutdownSequenceStarted bool
	versionID               string
}

// Configure sets default values on the Environment, except for the user manager
// and user manager info, which must be explicitly set.
func (e *Environment) Configure(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.EnvContext = ctx

	e.EvergreenSettings = testutil.TestConfig()

	e.Remote = queue.NewLocalLimitedSize(2, 1024)
	if err := e.Remote.Start(ctx); err != nil {
		return errors.WithStack(err)
	}
	e.Local = queue.NewLocalLimitedSize(2, 1024)
	if err := e.Local.Start(ctx); err != nil {
		return errors.WithStack(err)
	}
	qg, err := queue.NewLocalQueueGroup(ctx, queue.LocalQueueGroupOptions{
		DefaultQueue: queue.LocalQueueOptions{
			Constructor: func(ctx context.Context) (amboy.Queue, error) {
				return queue.NewLocalLimitedSize(2, 1024), nil
			},
		},
	})
	if err != nil {
		return err
	}
	e.RemoteGroup = qg

	e.InternalSender = send.MakeInternalLogger()

	jpm, err := jasper.NewSynchronizedManager(false)
	if err != nil {
		return errors.WithStack(err)
	}

	e.JasperProcessManager = jpm

	e.MongoClient, err = mongo.Connect(ctx, options.Client().
		ApplyURI(e.EvergreenSettings.Database.Url).
		SetWriteConcern(e.EvergreenSettings.Database.WriteConcernSettings.Resolve()).
		SetReadConcern(e.EvergreenSettings.Database.ReadConcernSettings.Resolve()).
		SetMonitor(apm.NewMonitor(apm.WithCommandAttributeDisabled(false))))

	if err != nil {
		return errors.WithStack(err)
	}

	e.DBSession = db.WrapClient(ctx, e.MongoClient).Clone()

	e.DatabaseName = e.EvergreenSettings.Database.DB
	e.roleManager = rolemanager.NewMongoBackedRoleManager(rolemanager.MongoBackedRoleManagerOpts{
		Client:          e.MongoClient,
		DBName:          e.DatabaseName,
		RoleCollection:  evergreen.RoleCollection,
		ScopeCollection: evergreen.ScopeCollection,
	})

	catcher := grip.NewBasicCatcher()
	catcher.Add(e.roleManager.RegisterPermissions(evergreen.ProjectPermissions))
	catcher.Add(e.roleManager.RegisterPermissions(evergreen.DistroPermissions))
	catcher.Add(e.roleManager.RegisterPermissions(evergreen.SuperuserPermissions))
	if catcher.HasErrors() {
		return errors.Wrap(catcher.Resolve(), "registering role manager permissions")
	}

	depot, err := BootstrapCredentialsCollection(ctx, e.MongoClient, e.EvergreenSettings.Database.Url, e.EvergreenSettings.Database.DB, e.EvergreenSettings.DomainName)
	if err != nil {
		return errors.Wrap(err, "bootstrapping host credentials collection")
	}
	e.Depot = depot

	// For testing purposes, set up parameter manager so it's backed by the DB.
	pm, err := parameterstore.NewParameterManager(ctx, parameterstore.ParameterManagerOptions{
		PathPrefix:     e.EvergreenSettings.ParameterStore.Prefix,
		CachingEnabled: true,
		SSMClient:      fakeparameter.NewFakeSSMClient(),
		DB:             e.MongoClient.Database(e.DatabaseName),
	})
	if err != nil {
		return errors.Wrap(err, "creating fake parameter manager")
	}
	e.ParamManager = pm

	// Although it would make more sense to call
	// auth.LoadUserManager(e.EvergreenSettings), we have to avoid an import
	// cycle where this package would transitively depend on the database
	// models.
	um, err := gimlet.NewBasicUserManager(nil, nil)
	if err != nil {
		return errors.Wrap(err, "creating user manager")
	}
	e.userManager = um

	e.Clients = evergreen.ClientConfig{
		LatestRevision: evergreen.ClientVersion,
		S3URLPrefix:    "https://foo.com",
	}

	return nil
}

// BootstrapCredentialsCollection initializes the credentials collection with
// the required CA configuration and returns the credentials depot.
func BootstrapCredentialsCollection(ctx context.Context, client *mongo.Client, dbURL, dbName, domainName string) (certdepot.Depot, error) {
	maxExpiration := time.Duration(math.MaxInt64)
	bootstrapConfig := certdepot.BootstrapDepotConfig{
		CAName: evergreen.CAName,
		MongoDepot: &certdepot.MongoDBOptions{
			MongoDBURI:     dbURL,
			DatabaseName:   dbName,
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
		ServiceName: domainName,
		ServiceOpts: &certdepot.CertificateOptions{
			CA:         evergreen.CAName,
			CommonName: domainName,
			Host:       domainName,
			Expires:    maxExpiration,
		},
	}

	depot, err := certdepot.BootstrapDepotWithMongoClient(ctx, client, bootstrapConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "could not bootstrap %s collection", evergreen.CredentialsCollection)
	}
	return depot, nil
}

func (e *Environment) Context() (context.Context, context.CancelFunc) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return context.WithCancel(e.EnvContext)
}

func (e *Environment) SetShutdown() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.shutdownSequenceStarted = true
}

func (e *Environment) ShutdownSequenceStarted() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.shutdownSequenceStarted
}
func (e *Environment) RemoteQueue() amboy.Queue {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Remote
}

func (e *Environment) LocalQueue() amboy.Queue {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Local
}

func (e *Environment) RemoteQueueGroup() amboy.QueueGroup {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.RemoteGroup
}

func (e *Environment) Session() db.Session {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.DBSession
}

func (e *Environment) ContextSession(_ context.Context) db.Session {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.DBSession
}

func (e *Environment) Client() *mongo.Client {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.MongoClient
}

func (e *Environment) DB() *mongo.Database {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.MongoClient.Database(e.DatabaseName)
}

func (e *Environment) SharedDB() *mongo.Database {
	return nil
}

func (e *Environment) JasperManager() jasper.Manager {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.JasperProcessManager
}

func (e *Environment) CertificateDepot() certdepot.Depot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.Depot
}

func (e *Environment) SetParameterManager(pm *parameterstore.ParameterManager) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ParamManager = pm
}

func (e *Environment) ParameterManager() *parameterstore.ParameterManager {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.ParamManager
}

func (e *Environment) Settings() *evergreen.Settings {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.EvergreenSettings
}

func (e *Environment) SaveConfig(context.Context) error {
	return nil
}

func (e *Environment) ClientConfig() *evergreen.ClientConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return &e.Clients
}

func (e *Environment) GetGitHubSender(string, string, evergreen.CreateInstallationTokenFunc) (send.Sender, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.InternalSender, nil
}

func (e *Environment) GetSender(key evergreen.SenderKey) (send.Sender, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.InternalSender, nil
}

func (e *Environment) SetSender(key evergreen.SenderKey, s send.Sender) error {
	return nil
}

func (e *Environment) RegisterCloser(name string, background bool, closer func(context.Context) error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Closers[name] = closer
}

func (e *Environment) Close(ctx context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// TODO we could, in the future call all closers in but that
	// would require more complex waiting and timeout logic

	deadline, _ := ctx.Deadline()
	catcher := grip.NewBasicCatcher()
	for name, closer := range e.Closers {
		if closer == nil {
			continue
		}

		grip.Info(message.Fields{
			"message":      "calling closer",
			"closer":       name,
			"timeout_secs": time.Since(deadline),
			"deadline":     deadline,
		})
		catcher.Add(closer(ctx))
	}

	return catcher.Resolve()
}

func (e *Environment) RoleManager() gimlet.RoleManager {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.roleManager
}

func (e *Environment) UserManager() gimlet.UserManager {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.userManager
}

func (e *Environment) SetUserManager(um gimlet.UserManager) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.userManager = um
}

func (e *Environment) UserManagerInfo() evergreen.UserManagerInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.userManagerInfo
}

func (e *Environment) SetUserManagerInfo(umi evergreen.UserManagerInfo) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.userManagerInfo = umi
}

// BuildVersion returns the ID of the Evergreen version that built the binary.
// Returns an empty string if the version ID isn't set.
func (e *Environment) BuildVersion() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.versionID
}
