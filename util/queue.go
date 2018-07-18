package util

import (
	"errors"
	"sync"
)

type Queue struct {
	Items []interface{}
	mu    sync.RWMutex
}

func NewQueue() Queue {
	return Queue{
		Items: []interface{}{},
	}
}

func (q *Queue) Enqueue(item interface{}) error {
	if item == nil {
		return errors.New("cannot enqueue nil item")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Items = append(q.Items, item)

	return nil
}

func (q *Queue) Dequeue() interface{} {
	if len(q.Items) == 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	item := q.Items[0]
	q.Items = q.Items[1:]
	return item
}

func (q *Queue) IsEmpty() bool {
	return q.Length() == 0
}

func (q *Queue) Peek() interface{} {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.IsEmpty() {
		return nil
	}
	return q.Items[0]
}

func (q *Queue) Length() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.Items)
}
