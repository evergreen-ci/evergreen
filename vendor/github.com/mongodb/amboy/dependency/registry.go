package dependency

import (
	"sync"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

var registry *registryCache

func init() {
	registry = &registryCache{
		m: map[string]ManagerFactory{},
		c: map[string]CheckFactory{},
	}

	RegisterManager(alwaysRunName, NewAlways)
	RegisterManager(checkTypeName, func() Manager { return makeCheckManager() })
	RegisterManager(createTypeName, func() Manager { return makeCreatesFile() })
	RegisterManager(localFileTypeName, func() Manager { return MakeLocalFile() })
}

// RegisterManager stores a dependency Manager factory in the global
// Manager registry.
func RegisterManager(name string, f ManagerFactory) { registry.addManager(name, f) }

// GetManagerFactory returns a globally registered manager factory by name.
func GetManagerFactory(name string) (ManagerFactory, error) { return registry.getManager(name) }

// RegisterCheck stores a CheckFactory in the global check registry.
func RegisterCheck(name string, f CheckFactory) { registry.addCheck(name, f) }

// GetCheckFactory returns a globally registered check factory by name.
func GetCheckFactory(name string) (CheckFactory, error) { return registry.getCheck(name) }

// ManagerFactory is a function that takes no arguments and returns
// a dependency.Manager interface. When implementing a new dependency
// type, also register a factory function with the DependencyFactory
// signature to facilitate serialization.
type ManagerFactory func() Manager

// CheckFactory is a function that takes no arguments and returns a
// dependency callback for use in callback-style dependencies.
type CheckFactory func() CheckFunc

type registryCache struct {
	m   map[string]ManagerFactory
	mmu sync.RWMutex
	c   map[string]CheckFactory
	cmu sync.RWMutex
}

func (r *registryCache) addManager(name string, factory ManagerFactory) {
	r.mmu.Lock()
	defer r.mmu.Unlock()

	if _, ok := r.m[name]; ok {
		grip.Warningf("overriding cached dependency Manager '%s'", name)
	}

	r.m[name] = factory
}

func (r *registryCache) getManager(name string) (ManagerFactory, error) {
	r.mmu.RLock()
	defer r.mmu.RUnlock()
	f, ok := r.m[name]
	if !ok {
		return nil, errors.Errorf("no factory named '%s' is registered", name)
	}
	return f, nil
}

func (r *registryCache) addCheck(name string, factory CheckFactory) {
	r.cmu.Lock()
	defer r.cmu.Unlock()

	if _, ok := r.c[name]; ok {
		grip.Warningf("overriding cached dependency callback '%s'", name)
	}

	r.c[name] = factory
}

func (r *registryCache) getCheck(name string) (CheckFactory, error) {
	r.cmu.RLock()
	defer r.cmu.RUnlock()

	f, ok := r.c[name]
	if !ok {
		return nil, errors.Errorf("no factory named '%s' is registered", name)
	}

	return f, nil
}
