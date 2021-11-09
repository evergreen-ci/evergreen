package queue

import (
	"sync"

	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

// ScopeManager provides a service to queue implementations to support
// additional locking semantics for queues that cannot push that into
// their backing storage.
type ScopeManager interface {
	Acquire(owner string, scopes []string) error
	Release(owner string, scopes []string) error
	ReleaseAndAcquire(ownerToRelease string, scopesToRelease []string, ownerToAcquire string, scopesToAcquire []string) error
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
		return amboy.NewDuplicateJobScopeErrorf("could not acquire lock scope '%s' held by '%s', not '%s'", sc, holder, id)
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

	toRelease, err := s.getScopesToRelease(id, scopes)
	if err != nil {
		return errors.Wrap(err, "getting scopes to release")
	}

	for _, sc := range toRelease {
		delete(s.scopes, sc)
	}

	return nil
}

func (s *scopeManagerImpl) getScopesToRelease(id string, scopes []string) ([]string, error) {
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
		return nil, errors.Errorf("could not release lock scope '%s', held by '%s' not '%s'", sc, holder, id)
	}
	return scopesToRelease, nil
}

func (s *scopeManagerImpl) ReleaseAndAcquire(ownerToRelease string, scopesToRelease []string, ownerToAcquire string, scopesToAcquire []string) error {
	if len(scopesToRelease) == 0 && len(scopesToAcquire) == 0 {
		return nil
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	toRelease, err := s.getScopesToRelease(ownerToRelease, scopesToRelease)
	if err != nil {
		return errors.Wrap(err, "getting scopes to release")
	}

	var toAcquire []string
	for _, sc := range scopesToAcquire {
		holder, ok := s.scopes[sc]
		if !ok || holder == ownerToRelease {
			toAcquire = append(toAcquire, sc)
			continue
		}
		if holder == ownerToAcquire {
			continue
		}
		return amboy.NewDuplicateJobScopeErrorf("could not acquire lock scope '%s' held by '%s', which is neither '%s' nor '%s'", sc, holder, ownerToAcquire, ownerToRelease)
	}

	for _, sc := range toRelease {
		delete(s.scopes, sc)
	}

	for _, sc := range toAcquire {
		s.scopes[sc] = ownerToAcquire
	}

	return nil
}
