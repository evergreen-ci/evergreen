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

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/anser/client"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	defaultMetadataCollection = "migrations.metadata"
	defaultAnserDB            = "anser"
)

var globalEnv *envState

func init() { ResetEnvironment() }

// Environment exposes the execution environment for the migration
// utility, and is the method by which, potentially serialized job
// definitions are able to gain access to the database and through
// which generator jobs are able to gain access to the queue.
//
// Implementations should be thread-safe, and are not required to be
// reconfigurable after their initial configuration.
type Environment interface {
	Setup(amboy.Queue, client.Client, db.Session) error
	GetSession() (db.Session, error)
	GetClient() (client.Client, error)
	GetQueue() (amboy.Queue, error)
	GetDependencyNetwork() (model.DependencyNetworker, error)
	MetadataNamespace() model.Namespace

	RegisterManualMigrationOperation(string, client.MigrationOperation) error
	GetManualMigrationOperation(string) (client.MigrationOperation, bool)
	RegisterDocumentProcessor(string, client.Processor) error
	GetDocumentProcessor(string) (client.Processor, bool)

	NewDependencyManager(string) dependency.Manager
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
		migrations: make(map[string]migrationOp),
		processor:  make(map[string]processor),
	}
}

type migrationOp struct {
	current client.MigrationOperation
}

type processor struct {
	current client.Processor
}

type envState struct {
	queue      amboy.Queue
	metadataNS model.Namespace
	session    db.Session
	client     client.Client
	deps       model.DependencyNetworker
	migrations map[string]migrationOp
	processor  map[string]processor
	closers    []func() error
	isSetup    bool
	mu         sync.RWMutex
}

func (e *envState) Setup(q amboy.Queue, cl client.Client, session db.Session) error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(session == nil, "cannot use a nil session")
	catcher.NewWhen(cl == nil, "cannot use a nil client")

	e.mu.Lock()
	defer e.mu.Unlock()
	catcher.NewWhen(e.isSetup, "reconfiguring a queue is not supported")

	catcher.NewWhen(!q.Info().Started, "configuring anser environment with a non-running queue")

	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	e.closers = append(e.closers, func() error { session.Close(); return nil })
	e.queue = q
	e.session = session
	e.client = cl
	e.metadataNS.Collection = defaultMetadataCollection
	e.metadataNS.DB = defaultAnserDB
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

	return e.session.Copy(), nil
}

func (e *envState) GetClient() (client.Client, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.client == nil {
		return nil, errors.New("no session defined")
	}

	return e.client, nil
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

func (e *envState) RegisterManualMigrationOperation(name string, op client.MigrationOperation) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.migrations[name]; ok {
		return errors.Errorf("migration operation %s already exists", name)
	}

	e.migrations[name] = migrationOp{current: op}
	return nil
}

func (e *envState) GetManualMigrationOperation(name string) (client.MigrationOperation, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	op, ok := e.migrations[name]
	return op.current, ok
}

func (e *envState) RegisterDocumentProcessor(name string, docp client.Processor) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.processor[name]; ok {
		return errors.Errorf("document processor named %s already registered", name)
	}

	e.processor[name] = processor{current: docp}
	return nil
}

func (e *envState) GetDocumentProcessor(name string) (client.Processor, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	docp, ok := e.processor[name]
	return docp.current, ok
}

func (e *envState) MetadataNamespace() model.Namespace {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.metadataNS
}

func (e *envState) NewDependencyManager(migrationID string) dependency.Manager {
	d := makeMigrationDependencyManager()

	e.mu.RLock()
	defer e.mu.RUnlock()

	d.MigrationID = migrationID
	d.MigrationHelper = NewMigrationHelper(e)

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
		grip.Warningf("encountered %d errors closing anser resources, out of %d", catcher.Len(), len(e.closers))
	}

	return catcher.Resolve()
}
