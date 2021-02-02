package queue

import (
	"sync"

	"github.com/pkg/errors"
)

// ScopeManager provides a service to queue implementations to support
// additional locking semantics for queues that cannot push that into
// their backing storage.
type ScopeManager interface {
	Acquire(string, []string) error
	Release(string, []string) error
}

type scopeManagerImpl struct {
	mutex  sync.Mutex
	scopes map[string]string
}

// NewLocalScopeManager constructs a ScopeManager implementation
// suitable for use in most local (in memory) queue implementations.
func NewLocalScopeManager() ScopeManager {
	return &scopeManagerImpl{
		scopes: map[string]string{},
	}
}

func (s *scopeManagerImpl) Acquire(id string, scopes []string) error {
	if len(scopes) == 0 {
		return nil
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	var scopesToAcquire []string
	for _, sc := range scopes {
		holder, ok := s.scopes[sc]
		if !ok {
			scopesToAcquire = append(scopesToAcquire, sc)
			continue
		}

		if holder == id {
			continue
		}
		return errors.Errorf("could not acquire lock scope '%s' held by '%s' not '%s'", sc, holder, id)
	}

	for _, sc := range scopesToAcquire {
		s.scopes[sc] = id
	}

	return nil
}

func (s *scopeManagerImpl) Release(id string, scopes []string) error {
	if len(scopes) == 0 {
		return nil
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	var scopesToRelease []string
	for _, sc := range scopes {
		holder, ok := s.scopes[sc]
		if !ok {
			continue
		}
		if holder == id {
			scopesToRelease = append(scopesToRelease, sc)
			continue
		}
		return errors.Errorf("could not release lock scope '%s', held by '%s' not '%s'", sc, holder, id)
	}

	for _, sc := range scopesToRelease {
		delete(s.scopes, sc)
	}

	return nil
}
