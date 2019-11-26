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
	"go.mongodb.org/mongo-driver/mongo"
	mgo "gopkg.in/mgo.v2"
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

	RegisterLegacyManualMigrationOperation(string, db.MigrationOperation) error
	GetLegacyManualMigrationOperation(string) (db.MigrationOperation, bool)
	RegisterLegacyDocumentProcessor(string, db.Processor) error
	GetLegacyDocumentProcessor(string) (db.Processor, bool)

	RegisterManualMigrationOperation(string, client.MigrationOperation) error
	GetManualMigrationOperation(string) (client.MigrationOperation, bool)
	RegisterDocumentProcessor(string, client.Processor) error
	GetDocumentProcessor(string) (client.Processor, bool)

	NewDependencyManager(string) dependency.Manager
	RegisterCloser(func() error)
	Close() error

	SetPreferedDB(interface{})
	PreferClient() bool
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
	legacy  db.MigrationOperation
	current client.MigrationOperation
}

type processor struct {
	legacy  db.Processor
	current client.Processor
}

type envState struct {
	queue        amboy.Queue
	metadataNS   model.Namespace
	session      db.Session
	client       client.Client
	deps         model.DependencyNetworker
	migrations   map[string]migrationOp
	processor    map[string]processor
	closers      []func() error
	isSetup      bool
	preferClient bool
	mu           sync.RWMutex
}

func (e *envState) Setup(q amboy.Queue, cl client.Client, session db.Session) error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(session == nil, "cannot use a nil session")
	catcher.NewWhen(cl == nil, "cannot use a nil client")

	e.mu.Lock()
	defer e.mu.Unlock()
	catcher.NewWhen(e.isSetup, "reconfiguring a queue is not supported")

	catcher.NewWhen(!q.Started(), "configuring anser environment with a non-running queue")

	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	dbName := session.DB("").Name()
	if dbName == "test" || dbName == "" {
		dbName = defaultAnserDB
	}

	e.closers = append(e.closers, func() error { session.Close(); return nil })
	e.queue = q
	e.session = session
	e.client = cl
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
	if op.current == nil && op.legacy != nil {
		ok = false
	}
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

	if docp.current == nil && docp.legacy != nil {
		ok = false
	}

	return docp.current, ok
}

func (e *envState) RegisterLegacyManualMigrationOperation(name string, op db.MigrationOperation) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.migrations[name]; ok {
		return errors.Errorf("migration operation %s already exists", name)
	}

	e.migrations[name] = migrationOp{legacy: op}
	return nil
}

func (e *envState) GetLegacyManualMigrationOperation(name string) (db.MigrationOperation, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	op, ok := e.migrations[name]
	if op.legacy == nil && op.current != nil {
		ok = false
	}

	return op.legacy, ok
}

func (e *envState) RegisterLegacyDocumentProcessor(name string, docp db.Processor) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.processor[name]; ok {
		return errors.Errorf("document processor named %s already registered", name)
	}

	e.processor[name] = processor{legacy: docp}
	return nil
}

func (e *envState) GetLegacyDocumentProcessor(name string) (db.Processor, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	docp, ok := e.processor[name]
	if docp.legacy == nil && docp.current != nil {
		ok = false
	}
	return docp.legacy, ok
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
	if e.unsafePreferClient() {
		d.MigrationHelper = NewMigrationHelper(e)
	} else {
		d.MigrationHelper = NewLegacyMigrationHelper(e)
	}

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

func (e *envState) SetPreferedDB(in interface{}) {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch in.(type) {
	case db.Session:
		e.preferClient = false
	case client.Client:
		e.preferClient = true
	case *mongo.Client:
		e.preferClient = true
	case *mgo.Session:
		e.preferClient = false
	}
}

func (e *envState) PreferClient() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.unsafePreferClient()
}

func (e *envState) unsafePreferClient() bool {
	if e.client == nil && e.session != nil {
		return false
	}
	if e.session == nil && e.client != nil {
		return true
	}

	return e.preferClient
}
