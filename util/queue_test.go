package util

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
)

type QueueSuite struct {
	q Queue
	suite.Suite
}

func TestQueueSuite(t *testing.T) {
	suite.Run(t, &QueueSuite{})
}

func (s *QueueSuite) SetupTest() {
	s.q = NewQueue()
}

func (s *QueueSuite) TestQueueOperations() {
	s.True(s.q.IsEmpty())

	s.NoError(s.q.Enqueue("a"))
	s.False(s.q.IsEmpty())
	s.Equal("a", s.q.Peek())
	s.Equal(1, s.q.Length())

	s.NoError(s.q.Enqueue("b"))
	s.False(s.q.IsEmpty())
	s.Equal("a", s.q.Peek())
	s.Equal(2, s.q.Length())

	s.NoError(s.q.Enqueue("c"))
	s.False(s.q.IsEmpty())
	s.Equal("a", s.q.Peek())
	s.Equal(3, s.q.Length())

	s.Equal("a", s.q.Dequeue())
	s.False(s.q.IsEmpty())
	s.Equal("b", s.q.Peek())
	s.Equal(2, s.q.Length())

	s.Equal("b", s.q.Dequeue())
	s.False(s.q.IsEmpty())
	s.Equal("c", s.q.Peek())
	s.Equal(1, s.q.Length())

	s.Equal("c", s.q.Dequeue())
	s.True(s.q.IsEmpty())
	s.Equal(nil, s.q.Peek())
	s.Equal(0, s.q.Length())
}

func (s *QueueSuite) TestQueueThreadsafe() {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		for i := 0; i < 100; i++ {
			s.NoError(s.q.Enqueue(i))
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		for i := 0; i < 200; i++ {
			s.NoError(s.q.Enqueue(i))
		}
		wg.Done()
	}()
	wg.Wait()
	s.Equal(300, s.q.Length())

	wg.Add(1)
	go func() {
		for i := 0; i < 200; i++ {
			s.q.Dequeue()
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		for i := 0; i < 100; i++ {
			s.q.Dequeue()
		}
		wg.Done()
	}()
	wg.Wait()
	s.Equal(0, s.q.Length())
}
