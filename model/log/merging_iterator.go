package log

import (
	"container/heap"

	"github.com/mongodb/grip"
)

type mergingIterator struct {
	iterators    []LogIterator
	lineLimit    int
	lineCount    int
	iteratorHeap *logIteratorHeap
	currentItem  LogLine
	catcher      grip.Catcher
	started      bool
	exhausted    bool
}

// newMergeIterator returns a LogIterator that merges N logs, passed in as
// iterators, respecting the order of each line's timestamp.
func newMergingIterator(lineLimit int, iterators ...LogIterator) *mergingIterator {
	return &mergingIterator{
		iterators:    iterators,
		lineLimit:    lineLimit,
		iteratorHeap: &logIteratorHeap{},
		catcher:      grip.NewBasicCatcher(),
	}
}

func (i *mergingIterator) Next() bool {
	if !i.started {
		i.init()
	}

	if i.exhausted {
		return false
	}

	if i.lineLimit > 0 && i.lineLimit == i.lineCount {
		i.exhausted = true
		return false
	}

	it := i.iteratorHeap.SafePop()
	if it == nil {
		return false
	}
	i.currentItem = it.Item()
	i.lineCount++

	if it.Next() {
		i.iteratorHeap.SafePush(it)
	} else {
		i.catcher.Add(it.Err())
		i.catcher.Add(it.Close())
		if i.catcher.HasErrors() {
			return false
		}
	}

	return true
}

func (i *mergingIterator) Exhausted() bool {
	if i.exhausted {
		return true
	}

	for _, it := range i.iterators {
		if !it.Exhausted() {
			return false
		}
	}
	return true
}

func (i *mergingIterator) init() {
	heap.Init(i.iteratorHeap)

	for j := range i.iterators {
		if i.iterators[j].Next() {
			i.iteratorHeap.SafePush(i.iterators[j])
		}

		// Fail early.
		if i.iterators[j].Err() != nil {
			i.catcher.Add(i.iterators[j].Err())
			i.iteratorHeap = &logIteratorHeap{}
			break
		}
	}

	i.started = true
}

func (i *mergingIterator) Err() error { return i.catcher.Resolve() }

func (i *mergingIterator) Item() LogLine { return i.currentItem }

func (i *mergingIterator) Close() error {
	catcher := grip.NewBasicCatcher()

	for {
		it := i.iteratorHeap.SafePop()
		if it == nil {
			break
		}
		catcher.Add(it.Close())
	}

	return catcher.Resolve()
}

////////////////////
// Log Iterator Heap
////////////////////

// logIteratorHeap is a heap of log iterators.
type logIteratorHeap struct {
	its []LogIterator
}

// Len returns the size of the heap.
func (h logIteratorHeap) Len() int { return len(h.its) }

// Less returns true if the object at index i is less than the object at index
// j in the heap, false otherwise.
func (h logIteratorHeap) Less(i, j int) bool {
	return h.its[i].Item().Timestamp < h.its[j].Item().Timestamp
}

// Swap swaps the objects at indexes i and j.
func (h logIteratorHeap) Swap(i, j int) { h.its[i], h.its[j] = h.its[j], h.its[i] }

// Push appends a new object of type LogIterator to the heap. Note that if x is
// not a LogIterator nothing happens.
func (h *logIteratorHeap) Push(x interface{}) {
	it, ok := x.(LogIterator)
	if !ok {
		return
	}

	h.its = append(h.its, it)
}

// Pop returns the next object (as an empty interface) from the heap. Note that
// if the heap is empty this will panic.
func (h *logIteratorHeap) Pop() interface{} {
	old := h.its
	n := len(old)
	x := old[n-1]
	h.its = old[0 : n-1]
	return x
}

// SafePush is a wrapper function around heap.Push that ensures, during compile
// time, that the correct type of object is put in the heap.
func (h *logIteratorHeap) SafePush(it LogIterator) {
	heap.Push(h, it)
}

// SafePop is a wrapper function around heap.Pop that converts the returned
// interface into a LogIterator object before returning it.
func (h *logIteratorHeap) SafePop() LogIterator {
	if h.Len() == 0 {
		return nil
	}

	i := heap.Pop(h)
	it := i.(LogIterator)
	return it
}
