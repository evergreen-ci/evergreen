package queue

import (
	"container/heap"
	"sync"
	"time"
)

////////////////////////////////////////////////////////////////////////
//
// Internal implementation of a priority queue using container/heap
//
////////////////////////////////////////////////////////////////////////

type fixedStorageItem struct {
	id string
	ts time.Time
}

type fixedStorageHeap []*fixedStorageItem

func (pq fixedStorageHeap) Len() int           { return len(pq) }
func (pq fixedStorageHeap) Less(i, j int) bool { return pq[i].ts.Before(pq[j].ts) }
func (pq fixedStorageHeap) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i] }

func (pq *fixedStorageHeap) Push(x interface{}) {
	item := x.(*fixedStorageItem)
	*pq = append(*pq, item)
}

func (pq *fixedStorageHeap) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]

	return item
}

type fixedStorage struct {
	heap     heap.Interface
	capacity int
	sync.Mutex
}

func newFixedStorage(c int) *fixedStorage {
	fixed := make(fixedStorageHeap, 0, c)
	return &fixedStorage{
		capacity: c,
		heap:     &fixed,
	}
}

func (s *fixedStorage) Push(i string) {
	s.Lock()
	defer s.Unlock()
	heap.Push(s.heap, &fixedStorageItem{id: i, ts: time.Now()})
}

func (s *fixedStorage) Pop() string {
	s.Lock()
	defer s.Unlock()

	item, ok := heap.Pop(s.heap).(*fixedStorageItem)
	if !ok || item == nil {
		return ""
	}

	return item.id
}

func (s *fixedStorage) Oversize() int {
	s.Lock()
	defer s.Unlock()

	if s.capacity > s.heap.Len() {
		return 0
	}

	return s.heap.Len() - s.capacity
}
