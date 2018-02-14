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
	value++
}

func (s *SafeCounter) Value() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return value
}

func (s *SafeCounter) String() string {
	return fmt.Sprint(s.Value())
}
