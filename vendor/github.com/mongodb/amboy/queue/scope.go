package queue

import (
	"sync"

	"github.com/pkg/errors"
)

type ScopeManager interface {
	Acquire(string, []string) error
	Release(string, []string) error
}

type scopeManagerImpl struct {
	mutex  sync.Mutex
	scopes map[string]string
}

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

	for _, sc := range scopes {
		holder, ok := s.scopes[sc]
		if !ok {
			s.scopes[sc] = id
			continue
		}

		if holder == id {
			continue
		}
		return errors.Errorf("could not acquire lock scope '%s' held by '%s' not '%s'", sc, holder, id)
	}

	return nil
}

func (s *scopeManagerImpl) Release(id string, scopes []string) error {
	if len(scopes) == 0 {
		return nil
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, sc := range scopes {
		holder, ok := s.scopes[sc]
		if !ok {
			continue
		}
		if holder == id {
			delete(s.scopes, sc)
			continue
		}
		return errors.Errorf("could not release lock scope '%s', held by '%s' not '%s'", sc, holder, id)
	}

	return nil
}
