package evergreen

import (
	"context"
	"sync"

	legacyDB "github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/amboy/queue/driver"
	"github.com/mongodb/anser/db"
	"github.com/pkg/errors"
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

	// Returns the settings object. The settings object is not
	// necessarily safe for concurrent access.
	Settings() *Settings
	Session() db.Session

	// The Environment provides access to two queue's, a
	// local-process level queue that is not persisted between
	// runs, and a remote shared queue that all processes can use
	// to distribute work amongst the application tier.
	//
	// The LocalQueue is not durable, and results aren't available
	// between process restarts.
	LocalQueue() amboy.Queue
	RemoteQueue() amboy.Queue
}

type envState struct {
	remoteQueue amboy.Queue
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
	e.settings, err = NewSettings(confPath)
	if err != nil {
		return errors.Wrap(err, "problem getting settings")
	}
	if err = e.settings.Validate(); err != nil {
		return errors.Wrap(err, "problem validating settings")
	}

	// set up the database connection configuration using the
	// legacy session factory mechanism. in the future the
	// environment can and should be the only provider of database
	// sessions.
	sf := e.settings.SessionFactory()
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
	opts.DB = e.settings.Amboy.DB
	opts.Priority = true

	qmdb, err = driver.OpenNewMongoDB(ctx, e.settings.Amboy.Name, opts, e.session)
	if err != nil {
		return errors.Wrap(err, "problem setting queue backend")
	}
	rq := queue.NewRemoteUnordered(e.settings.Amboy.PoolSizeRemote)
	if err = rq.SetDriver(qmdb); err != nil {
		return errors.WithStack(err)
	}
	e.remoteQueue = rq
	if err = e.remoteQueue.Start(ctx); err != nil {
		return errors.Wrap(err, "problem starting remote queue")
	}

	// configure the local-only (memory-backed) queue.
	e.localQueue = queue.NewLocalLimitedSize(e.settings.Amboy.PoolSizeLocal, e.settings.Amboy.LocalStorage)
	if err = e.localQueue.Start(ctx); err != nil {
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
