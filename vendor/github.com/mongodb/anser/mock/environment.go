package mock

import (
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/anser/client"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type Environment struct {
	ShouldPreferClient bool
	IsSetup            bool
	ReturnNilClient    bool
	SetupError         error
	SessionError       error
	ClientError        error
	QueueError         error
	NetworkError       error

	PreferedSetup           interface{}
	Session                 *Session
	Client                  *Client
	Network                 *DependencyNetwork
	Queue                   amboy.Queue
	SetupClient             client.Client
	SetupSession            db.Session
	Closers                 []func() error
	DependencyManagers      map[string]*DependencyManager
	LegacyMigrationRegistry map[string]db.MigrationOperation
	LegacyProcessorRegistry map[string]db.Processor
	MigrationRegistry       map[string]client.MigrationOperation
	ProcessorRegistry       map[string]client.Processor
	MetaNS                  model.Namespace
}

func NewEnvironment() *Environment {
	return &Environment{
		Session:                 NewSession(),
		Network:                 NewDependencyNetwork(),
		DependencyManagers:      make(map[string]*DependencyManager),
		LegacyMigrationRegistry: make(map[string]db.MigrationOperation),
		LegacyProcessorRegistry: make(map[string]db.Processor),
		MigrationRegistry:       make(map[string]client.MigrationOperation),
		ProcessorRegistry:       make(map[string]client.Processor),
	}
}

func (e *Environment) Setup(q amboy.Queue, cl client.Client, session db.Session) error {
	e.Queue = q
	e.IsSetup = true
	e.SetupSession = session
	e.SetupClient = cl
	return e.SetupError
}

func (e *Environment) GetSession() (db.Session, error) {
	if e.SessionError != nil {
		return nil, e.SessionError
	}
	return e.Session, nil
}

func (e *Environment) GetClient() (client.Client, error) {
	if e.ClientError != nil {
		return nil, e.ClientError
	}

	if e.ReturnNilClient {
		return nil, nil
	}

	if e.Client == nil {
		e.Client = NewClient()
	}

	return e.Client, nil

}

func (e *Environment) GetQueue() (amboy.Queue, error) {
	if e.QueueError != nil {
		return nil, e.QueueError
	}

	return e.Queue, nil
}

func (e *Environment) GetDependencyNetwork() (model.DependencyNetworker, error) {
	if e.NetworkError != nil {
		return nil, e.NetworkError
	}

	return e.Network, nil
}

func (e *Environment) RegisterLegacyManualMigrationOperation(name string, op db.MigrationOperation) error {
	if _, ok := e.LegacyMigrationRegistry[name]; ok {
		return errors.Errorf("migration operation %s already exists", name)
	}

	e.LegacyMigrationRegistry[name] = op
	return nil
}

func (e *Environment) GetLegacyManualMigrationOperation(name string) (db.MigrationOperation, bool) {
	op, ok := e.LegacyMigrationRegistry[name]
	return op, ok
}

func (e *Environment) RegisterLegacyDocumentProcessor(name string, docp db.Processor) error {
	if _, ok := e.LegacyProcessorRegistry[name]; ok {
		return errors.Errorf("document processor named %s already registered", name)
	}

	e.LegacyProcessorRegistry[name] = docp
	return nil
}

func (e *Environment) GetLegacyDocumentProcessor(name string) (db.Processor, bool) {
	docp, ok := e.LegacyProcessorRegistry[name]
	return docp, ok
}

func (e *Environment) RegisterManualMigrationOperation(name string, op client.MigrationOperation) error {
	if _, ok := e.MigrationRegistry[name]; ok {
		return errors.Errorf("migration operation %s already exists", name)
	}

	e.MigrationRegistry[name] = op
	return nil
}

func (e *Environment) GetManualMigrationOperation(name string) (client.MigrationOperation, bool) {
	op, ok := e.MigrationRegistry[name]
	return op, ok
}

func (e *Environment) RegisterDocumentProcessor(name string, docp client.Processor) error {
	if _, ok := e.ProcessorRegistry[name]; ok {
		return errors.Errorf("document processor named %s already registered", name)
	}

	e.ProcessorRegistry[name] = docp
	return nil
}

func (e *Environment) GetDocumentProcessor(name string) (client.Processor, bool) {
	docp, ok := e.ProcessorRegistry[name]
	return docp, ok
}

func (e *Environment) MetadataNamespace() model.Namespace { return e.MetaNS }

func (e *Environment) NewDependencyManager(n string) dependency.Manager {
	edges := dependency.NewJobEdges()
	e.DependencyManagers[n] = &DependencyManager{
		Name:     n,
		JobEdges: &edges,
	}

	return e.DependencyManagers[n]
}

func (e *Environment) RegisterCloser(closer func() error) { e.Closers = append(e.Closers, closer) }
func (e *Environment) Close() error {
	catcher := grip.NewSimpleCatcher()
	for _, closer := range e.Closers {
		catcher.Add(closer())
	}
	return catcher.Resolve()
}

func (e *Environment) PreferClient() bool           { return e.ShouldPreferClient }
func (e *Environment) SetPreferedDB(in interface{}) { e.PreferedSetup = in }
