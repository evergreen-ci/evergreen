package jasper

import (
	"sync"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// SignalTriggerFactory is a function that creates a SignalTrigger.
type SignalTriggerFactory func() SignalTrigger

type signalTriggerRegistry struct {
	mu             sync.RWMutex
	signalTriggers map[SignalTriggerID]SignalTriggerFactory
}

var jasperSignalTriggerRegistry *signalTriggerRegistry

func init() {
	jasperSignalTriggerRegistry = newSignalTriggerRegistry()

	signalTriggers := map[SignalTriggerID]SignalTriggerFactory{
		CleanTerminationSignalTrigger: makeCleanTerminationSignalTrigger,
	}

	for id, factory := range signalTriggers {
		grip.EmergencyPanic(RegisterSignalTriggerFactory(id, factory))
	}
}

func newSignalTriggerRegistry() *signalTriggerRegistry {
	return &signalTriggerRegistry{signalTriggers: map[SignalTriggerID]SignalTriggerFactory{}}
}

// RegisterSignalTriggerFactory registers a factory to create the signal trigger
// represented by the id.
func RegisterSignalTriggerFactory(id SignalTriggerID, factory SignalTriggerFactory) error {
	return errors.Wrap(jasperSignalTriggerRegistry.registerSignalTriggerFactory(id, factory), "problem registering signal trigger factory")
}

// GetSignalTriggerFactory retrieves a factory to create the signal trigger
// represented by the id.
func GetSignalTriggerFactory(id SignalTriggerID) (SignalTriggerFactory, bool) {
	return jasperSignalTriggerRegistry.getSignalTriggerFactory(id)
}

func (r *signalTriggerRegistry) registerSignalTriggerFactory(id SignalTriggerID, factory SignalTriggerFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if string(id) == "" {
		return errors.New("cannot register an empty signal trigger id")
	}

	if _, ok := r.signalTriggers[id]; ok {
		return errors.Errorf("signal trigger '%s' is already registered", string(id))
	}

	if factory == nil {
		return errors.Errorf("cannot register a nil factory for signal trigger id '%s'", string(id))
	}

	r.signalTriggers[id] = factory
	return nil
}

func (r *signalTriggerRegistry) getSignalTriggerFactory(id SignalTriggerID) (SignalTriggerFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.signalTriggers[id]
	return factory, ok
}
