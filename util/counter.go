package util

import (
	"fmt"
	"sync"
)

type SafeCounter struct {
	value int
	mutex sync.Mutex
}

func (s *SafeCounter) Inc() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.value++
}

func (s *SafeCounter) Value() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.value
}

func (s *SafeCounter) String() string {
	return fmt.Sprint(s.Value())
}
