package util

import (
	"fmt"
	"sync"
)

type SafeCounter struct {
	value int
	mutex sync.RWMutex
}

func (s *SafeCounter) Inc() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.value++
}

func (s *SafeCounter) Value() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.value
}

func (s *SafeCounter) String() string {
	return fmt.Sprint(s.Value())
}
