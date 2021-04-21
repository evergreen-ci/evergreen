package options

import (
	"sync"

	"github.com/mongodb/grip/send"
)

var globalLoggerRegistry LoggerRegistry = &basicLoggerRegistry{
	factories: map[string]LoggerProducerFactory{
		LogDefault:       NewDefaultLoggerProducer,
		LogFile:          NewFileLoggerProducer,
		LogInherited:     NewInheritedLoggerProducer,
		LogInMemory:      NewInMemoryLoggerProducer,
		LogSplunk:        NewSplunkLoggerProducer,
		LogBuildloggerV2: NewBuildloggerV2LoggerProducer,
	},
}

// GetGlobalLoggerRegistry returns the global logger registry.
func GetGlobalLoggerRegistry() LoggerRegistry { return globalLoggerRegistry }

// LoggerRegistry is an interface that stores reusable logger factories.
type LoggerRegistry interface {
	Register(LoggerProducerFactory)
	Check(string) bool
	Names() []string
	Resolve(string) (LoggerProducerFactory, bool)
}

type basicLoggerRegistry struct {
	mu        sync.RWMutex
	factories map[string]LoggerProducerFactory
}

// NewBasicLoggerRegistry returns a new LoggerRegistry implementation.
func NewBasicLoggerRegistry() LoggerRegistry {
	return &basicLoggerRegistry{
		factories: map[string]LoggerProducerFactory{},
	}
}

func (r *basicLoggerRegistry) Register(factory LoggerProducerFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[factory().Type()] = factory
}

func (r *basicLoggerRegistry) Check(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.factories[name]
	return ok
}

func (r *basicLoggerRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := []string{}
	for name := range r.factories {
		names = append(names, name)
	}

	return names
}

func (r *basicLoggerRegistry) Resolve(name string) (LoggerProducerFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, ok := r.factories[name]
	return factory, ok
}

// LoggerProducer produces a Logger interface backed by a grip logger.
type LoggerProducer interface {
	Type() string
	Configure() (send.Sender, error)
}

// LoggerProducerFactory creates a new instance of a LoggerProducer implementation.
type LoggerProducerFactory func() LoggerProducer
