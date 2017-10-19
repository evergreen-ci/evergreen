package evergreen

import (
	"sync"

	legacyDB "github.com/evergreen-ci/db"
	"github.com/evergreen-ci/sink/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/amboy/queue/driver"
	"github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	mgo "gopkg.in/mgo.v2"
)

var globalEnvState *envState

func init() {
	globalEnvState = &envState{}
}

// GetEnvironment returns the global application level
// environment. This implementation is thread safe, but must be
// configured before use.
//
// In general you should call this operation once per process
// execution and pass the Environment interface through your
// application like a context, although there are cases in legacy code
// (e.g. models) and in the implementation of amboy jobs where it is
// neccessary to access the global environment. There is a mock
// implementation for use in testing.
func GetEnvironment() Environment { return globalEnvState }

// Environment provides application-level services (e.g. databases,
// configuration, queues.
type Environment interface {
	// Configure initializes the object. Some implementations may
	// not allow the same instance to be configured more than
	// once.
	//
	// If Configure returns without an error, you should assume
	// that the queues have been started, there was no issue
	// establishing a connection to the database, and that the
	// local and remote queues have started.
	Configure(context.Context, string) error

	// Returns the settings object.
	Settings() *Settings
	LocalQueue() amboy.Queue
	RemoteQueue() amboy.Queue
	Session() db.Session
}

type envState struct {
	remoteQueue amboyQueue
	localQueue  amboy.Queue
	settings    *Settings
	session     *mgo.Session
	mu          sync.RWMutex
}

func (e *envState) Configure(ctx context.Context, confPath string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// make sure we don't reconfigure (and leak resources)
	if e.settings != nil || legacyDB.HasGlobalSessionProvider() {
		return errors.New("cannot reconfigre a configured environment")
	}

	var err error

	// read configuration from the file and validate.
	// at some point this should just be read from the database at
	// a later stage, and populated with default values if it
	// isn't in the db.
	e.settings, err = evergreen.NewSettings(c.ConfigPath)
	if err != nil {
		return errors.Wrap(err, "problem getting settings")
	}
	if err = settings.Validate(); err != nil {
		return errors.Wrap(err, "problem validating settings")
	}

	// set up the database connection configuration using the
	// legacy session factory mechanism. in the future the
	// environment can and should be the only provider of database
	// sessions.
	sf := legacyDB.SessionFactoryFromConfig(e.settings)
	legacyDB.SetGlobalSessionProvider(sf)

	e.session, _, err = sf.GetSession()
	if err != nil {
		return errors.Wrap(err, "problem getting database session")
	}

	// configure the remote mongodb-backed amboy
	// queue.
	var qmdb *driver.MongoDB

	opts := driver.DefaultMongoDBOptions()
	opts.URI = e.settings.Database.Url

	qmdb, err = driver.OpenNewMongoDB(ctx, "service", opts, e.session)
	if err != nil {
		return errors.Wrap(err, "problem setting queue backend")
	}

	if err = rq.SetDriver(qmdb); err != nil {
		return errors.WithStack(err)
	}
	rq := queue.NewRemoteUnordered(2)
	s.remoteQueue = rq
	if err = s.remoteQueue.Start(ctx); err != nil {
		return errors.Wrap(err, "problem starting remote queue")
	}

	// configure the local-only (memory-backed) queue.
	s.localQueue = queue.NewLocalLimitedSize(2, 1024) // workers, capacity
	if err = s.localQueue.Start(ctx); err != nil {
		return errors.Wrap(err, "problem starting local queue")
	}

	return nil
}

func (e *envState) Settings() *Settings {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.settings
}

func (e *envState) LocalQueue() amboy.Queue {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.localQueue
}

func (e *envState) RemoteQueue() amboy.Queue {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.localQueue
}

func (e *envState) Session() db.Session {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return db.WrapSession(e.session.Clone())
}
