package evergreen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	legacyDB "github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
)

var globalEnvState *envState

func init() {
	ResetEnvironment()
}

// GetEnvironment returns the global application level
// environment. This implementation is thread safe, but must be
// configured before use.
//
// In general you should call this operation once per process
// execution and pass the Environment interface through your
// application like a context, although there are cases in legacy code
// (e.g. models) and in the implementation of amboy jobs where it is
// necessary to access the global environment. There is a mock
// implementation for use in testing.
func GetEnvironment() Environment { return globalEnvState }

func ResetEnvironment() { globalEnvState = &envState{} }

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

	// ClientConfig provides access to a list of evergreen clients,
	// for different platforms
	ClientConfig() *ClientConfig
}

type envState struct {
	remoteQueue  amboy.Queue
	localQueue   amboy.Queue
	settings     *Settings
	session      *mgo.Session
	mu           sync.RWMutex
	clientConfig *ClientConfig
}

func (e *envState) Configure(ctx context.Context, confPath string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// make sure we don't reconfigure (and leak resources)
	if e.settings != nil {
		return errors.New("cannot reconfigre a configured environment")
	}

	if err := e.initSettings(confPath); err != nil {
		return errors.WithStack(err)
	}

	catcher := grip.NewBasicCatcher()

	catcher.Add(e.initDB())
	catcher.Add(e.createQueues(ctx))
	catcher.Extend(e.initQueues(ctx))

	if !catcher.HasErrors() {
		var err error
		e.clientConfig, err = getClientConfig(e.settings.Ui.Url)
		catcher.Add(err)
		if err == nil && len(e.clientConfig.ClientBinaries) == 0 {
			grip.Warning("No clients are available for this server")
		}
	}
	return catcher.Resolve()
}

func (e *envState) initSettings(path string) error {
	// read configuration from the file and validate.
	// at some point this should just be read from the database at
	// a later stage, and populated with default values if it
	// isn't in the db.

	var err error

	if e.settings == nil {
		// this helps us test the validate method
		e.settings, err = NewSettings(path)
		if err != nil {
			return errors.Wrap(err, "problem getting settings")
		}
	}

	if err = e.settings.Validate(); err != nil {
		return errors.Wrap(err, "problem validating settings")
	}

	return nil
}

func (e *envState) initDB() error {
	if legacyDB.HasGlobalSessionProvider() {
		grip.Warning("database session configured; reconfiguring")
	}

	// set up the database connection configuration using the
	// legacy session factory mechanism. in the future the
	// environment can and should be the only provider of database
	// sessions.
	sf := e.settings.SessionFactory()
	legacyDB.SetGlobalSessionProvider(sf)

	var err error

	e.session, _, err = sf.GetSession()

	return errors.Wrap(err, "problem getting database session")
}

func (e *envState) createQueues(ctx context.Context) error {
	// configure the local-only (memory-backed) queue.
	e.localQueue = queue.NewLocalLimitedSize(e.settings.Amboy.PoolSizeLocal, e.settings.Amboy.LocalStorage)

	// configure the remote mongodb-backed amboy
	// queue.
	opts := queue.DefaultMongoDBOptions()
	opts.URI = e.settings.Database.Url
	opts.DB = e.settings.Amboy.DB
	opts.Priority = true

	qmdb, err := queue.OpenNewMongoDBDriver(ctx, e.settings.Amboy.Name, opts, e.session)
	if err != nil {
		return errors.Wrap(err, "problem setting queue backend")
	}
	rq := queue.NewRemoteUnordered(e.settings.Amboy.PoolSizeRemote)
	if err = rq.SetDriver(qmdb); err != nil {
		return errors.WithStack(err)
	}
	e.remoteQueue = rq

	return nil
}

func (e *envState) initQueues(ctx context.Context) []error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(e.localQueue.Start(ctx))
	catcher.Add(e.remoteQueue.Start(ctx))

	return catcher.Errors()
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
	return e.remoteQueue
}

func (e *envState) Session() db.Session {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return db.WrapSession(e.session.Copy())
}

func (e *envState) ClientConfig() *ClientConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.clientConfig
}

// getClientConfig should be called once at startup and looks at the
// current environment and loads all currently available client
// binaries for use by the API server in presenting the settings page.
//
// If there are no built clients, this returns an empty config
// version, but does *not* error.
func getClientConfig(baseURL string) (*ClientConfig, error) {
	c := &ClientConfig{}
	c.LatestRevision = ClientVersion

	root := filepath.Join(FindEvergreenHome(), ClientDirectory)

	if _, err := os.Stat(root); os.IsNotExist(err) {
		grip.Warningf("client directory '%s' does not exist, creating empty "+
			"directory and continuing with caution", root)
		grip.Error(os.MkdirAll(root, 0755))
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || info.Name() == "version" {
			return nil
		}

		parts := strings.Split(path, string(filepath.Separator))
		buildInfo := strings.Split(parts[len(parts)-2], "_")

		c.ClientBinaries = append(c.ClientBinaries, ClientBinary{
			URL: fmt.Sprintf("%s/%s/%s", baseURL, ClientDirectory,
				strings.Join(parts[len(parts)-2:], "/")),
			OS:   buildInfo[0],
			Arch: buildInfo[1],
		})

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "problem finding client binaries")
	}

	return c, nil
}
