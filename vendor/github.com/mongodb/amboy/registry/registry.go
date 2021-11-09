package registry

import (
	"sync"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

var amboyRegistry *typeRegistry

func init() {
	amboyRegistry = newTypeRegistry()
}

// Private implementation of the types registry. Public methods for
// accessing the registry instance are in other files.

type typeRegistry struct {
	job *jobs
}

type jobs struct {
	m map[string]JobFactory
	l *sync.RWMutex
}

func newTypeRegistry() *typeRegistry {
	return &typeRegistry{
		job: &jobs{
			m: make(map[string]JobFactory),
			l: &sync.RWMutex{},
		},
	}
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
		return nil, errors.Errorf("there is no job type named '%s' registered", name)
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
