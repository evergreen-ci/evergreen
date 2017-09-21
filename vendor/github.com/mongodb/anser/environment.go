/*
Migration Execution Environment

Anser provides the Environment interface, with a global instance
accessible via the exported GetEnvironment() function to provide
access to runtime configuration state: database connections;
amboy.Queue objects, and registries for task implementations.

The Environment is an interface: you can build a mock, or use one
provided for testing purposes by anser (coming soon).

*/
package anser

import (
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
)

const (
	defaultMetadataCollection = "migrations.metadata"
	defaultAnserDB            = "anser"
)

var globalEnv *envState

var dialTimeout = 10 * time.Second

func init() { ResetEnvironment() }

// Environment exposes the execution environment for the migration
// utility, and is the method by which, potentially serialized job
// definitions are able to gain access to the database and through
// which generator jobs are able to gain access to the queue.
//
// Implementations should be thread-safe, and are not required to be
// reconfigurable after their initial configuration.
type Environment interface {
	Setup(amboy.Queue, string) error
	GetSession() (db.Session, error)
	GetQueue() (amboy.Queue, error)
	GetDependencyNetwork() (model.DependencyNetworker, error)
	MetadataNamespace() model.Namespace
	RegisterManualMigrationOperation(string, db.MigrationOperation) error
	GetManualMigrationOperation(string) (db.MigrationOperation, bool)
	RegisterDocumentProcessor(string, db.Processor) error
	GetDocumentProcessor(string) (db.Processor, bool)
	NewDependencyManager(string, map[string]interface{}, model.Namespace) dependency.Manager
	RegisterCloser(func() error)
	Close() error
}

// GetEnvironment returns the global environment object. Because this
// produces a pointer to the global object, make sure that you have a
// way to replace it with a mock as needed for testing.
func GetEnvironment() Environment { return globalEnv }

// ResetEnvironment resets the global environment object. Use this
// only in testing (and only when you must.) It is not safe for
// concurrent use.
func ResetEnvironment() {
	globalEnv = &envState{
		migrations: make(map[string]db.MigrationOperation),
		processor:  make(map[string]db.Processor),
	}
}

type envState struct {
	queue      amboy.Queue
	session    *mgo.Session
	metadataNS model.Namespace
	deps       model.DependencyNetworker
	migrations map[string]db.MigrationOperation
	processor  map[string]db.Processor
	closers    []func() error
	isSetup    bool
	mu         sync.RWMutex
}

func (e *envState) Setup(q amboy.Queue, mongodbURI string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.isSetup {
		return errors.New("reconfiguring a queue is not supported")
	}

	session, err := mgo.DialWithTimeout(mongodbURI, dialTimeout)
	if err != nil {
		return errors.Wrap(err, "problem establishing connection")
	}

	if !q.Started() {
		return errors.New("configuring anser environment with a non-running queue")
	}

	dbName := session.DB("").Name
	if dbName == "test" {
		dbName = defaultAnserDB
	}

	e.queue = q
	e.session = session
	e.metadataNS.Collection = defaultMetadataCollection
	e.metadataNS.DB = dbName
	e.isSetup = true
	e.deps = newDependencyNetwork()

	return nil
}

func (e *envState) GetSession() (db.Session, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.session == nil {
		return nil, errors.New("no session defined")
	}

	return db.WrapSession(e.session.Clone()), nil
}

func (e *envState) GetQueue() (amboy.Queue, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.queue == nil {
		return nil, errors.New("no queue defined")
	}

	return e.queue, nil
}

func (e *envState) GetDependencyNetwork() (model.DependencyNetworker, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.deps == nil {
		return nil, errors.New("no dependency networker specified")
	}

	return e.deps, nil
}

func (e *envState) RegisterManualMigrationOperation(name string, op db.MigrationOperation) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.migrations[name]; ok {
		return errors.Errorf("migration operation %s already exists", name)
	}

	e.migrations[name] = op
	return nil
}

func (e *envState) GetManualMigrationOperation(name string) (db.MigrationOperation, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	op, ok := e.migrations[name]
	return op, ok
}

func (e *envState) RegisterDocumentProcessor(name string, docp db.Processor) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.processor[name]; ok {
		return errors.Errorf("document processor named %s already registered", name)
	}

	e.processor[name] = docp
	return nil
}

func (e *envState) GetDocumentProcessor(name string) (db.Processor, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	docp, ok := e.processor[name]
	return docp, ok
}

func (e *envState) MetadataNamespace() model.Namespace {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.metadataNS
}

func (e *envState) NewDependencyManager(migrationID string, query map[string]interface{}, ns model.Namespace) dependency.Manager {
	d := makeMigrationDependencyManager()

	e.mu.RLock()
	defer e.mu.RUnlock()

	d.MigrationHelper = NewMigrationHelper(e)
	d.Query = query
	d.MigrationID = migrationID
	d.NS = ns

	return d
}

func (e *envState) RegisterCloser(closer func() error) {
	if closer == nil {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.closers = append(e.closers, closer)
}

func (e *envState) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	grip.Noticef("closing %d resources registered in the anser environment", len(e.closers))

	catcher := grip.NewSimpleCatcher()
	for _, closer := range e.closers {
		catcher.Add(closer())
	}

	if catcher.HasErrors() {
		grip.Warningf("encountered %d errors closingoanser resources, out of %d", catcher.Len(), len(e.closers))
	}

	return catcher.Resolve()
}
