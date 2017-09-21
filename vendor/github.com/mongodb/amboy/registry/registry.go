package registry

import (
	"fmt"
	"sync"

	"github.com/mongodb/grip"
)

// Private implementation of the types registry. Public methods for
// accessing the registry instance are in other files.

type typeRegistry struct {
	job *jobs
	dep *dependencies
}

type jobs struct {
	m map[string]JobFactory
	l *sync.RWMutex
}

type dependencies struct {
	m map[string]DependencyFactory
	l *sync.RWMutex
}

func newTypeRegistry() *typeRegistry {
	return &typeRegistry{
		dep: &dependencies{
			m: make(map[string]DependencyFactory),
			l: &sync.RWMutex{},
		},
		job: &jobs{
			m: make(map[string]JobFactory),
			l: &sync.RWMutex{},
		},
	}
}

func (r *typeRegistry) registerDependencyType(name string, f DependencyFactory) {
	r.dep.l.Lock()
	defer r.dep.l.Unlock()

	if _, exists := r.dep.m[name]; exists {
		grip.Warningf("dependency named '%s' is already registered. Overwriting existing value.", name)
	}

	r.dep.m[name] = f
}

func (r *typeRegistry) registerJobType(name string, f JobFactory) {
	r.job.l.Lock()
	defer r.job.l.Unlock()

	if _, exists := r.job.m[name]; exists {
		grip.Warningf("job named '%s' is already registered. Overwriting existing value.", name)
	}

	r.job.m[name] = f
}

func (r *typeRegistry) getJobFactory(name string) (JobFactory, error) {
	r.job.l.RLock()
	defer r.job.l.RUnlock()

	factory, ok := r.job.m[name]
	if !ok {
		return nil, fmt.Errorf("there is no job type named '%s' registered", name)
	}

	return factory, nil
}

func (r *typeRegistry) getDependencyFactory(name string) (DependencyFactory, error) {
	r.dep.l.RLock()
	defer r.dep.l.RUnlock()

	factory, ok := r.dep.m[name]
	if !ok {
		return nil, fmt.Errorf("there is no job type named '%s' registered", name)
	}

	return factory, nil
}

func (r *typeRegistry) jobTypeNames() <-chan string {
	output := make(chan string)

	go func() {
		r.job.l.RLock()
		defer r.job.l.RUnlock()
		for j := range r.job.m {
			output <- j
		}
		close(output)
	}()

	return output
}
