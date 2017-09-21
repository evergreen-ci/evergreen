/*
Capped Results Storage

The CappedResultsStorage type provides a fixed size for results
storage, for local queues (that don't have external storage) in the
context of long running applications.
*/
package driver

import (
	"container/heap"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
)

// CappedResultStorage provides a fixed size storage structure for
// queue results, and is used as the backend for a queue
// implementation.
type CappedResultStorage struct {
	Cap     int
	results cappedResults
	table   map[string]amboy.Job
	mutex   sync.RWMutex
}

// NewCappedResultStorage creates a new structure for storing queue
// results.
func NewCappedResultStorage(cap int) *CappedResultStorage {
	return &CappedResultStorage{
		Cap:   cap,
		table: make(map[string]amboy.Job),
	}
}

// Add inserts a job into the structure.
func (s *CappedResultStorage) Add(j amboy.Job) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	name := j.ID()
	_, ok := s.table[name]
	s.table[name] = j
	if ok {
		return
	}

	item := &resultItem{
		name: name,
		time: time.Now(),
	}

	heap.Push(&s.results, item)

	for size := len(s.table); size > s.Cap; size-- {
		item = heap.Pop(&s.results).(*resultItem)

		grip.Debugf("removing item '%s' from results storage", item.name)
		delete(s.table, item.name)
	}
}

// Get retrieves an object from the results storage by name. If the
// object doesn't exist, the second value is false.
func (s *CappedResultStorage) Get(name string) (amboy.Job, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	j, ok := s.table[name]
	if !ok {
		return nil, false
	}
	return j, true
}

// Contents is a generator that produces all jobs in the results
// storage. The order is random.
func (s *CappedResultStorage) Contents() <-chan amboy.Job {
	output := make(chan amboy.Job)

	go func() {
		s.mutex.RLock()
		defer s.mutex.RUnlock()

		for _, job := range s.table {
			output <- job
		}
		close(output)
	}()

	return output
}

// Size returns the current number of results in the storage
// structure.
func (s *CappedResultStorage) Size() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return len(s.results)
}

////////////////////////////////////////////////////////////////////////
//
// Internal Implementation of the heap interface.
//
////////////////////////////////////////////////////////////////////////

type cappedResults []*resultItem

type resultItem struct {
	name     string
	time     time.Time
	position int
}

func (cr cappedResults) Len() int {
	return len(cr)
}

func (cr cappedResults) Less(i, j int) bool {
	return cr[i].time.Before(cr[j].time)
}

func (cr cappedResults) Swap(i, j int) {
	cr[i], cr[j] = cr[j], cr[i]
	cr[i].position = i
	cr[j].position = j
}

func (cr *cappedResults) Push(x interface{}) {
	n := len(*cr)
	item := x.(*resultItem)
	item.position = n
	*cr = append(*cr, item)
}

func (cr *cappedResults) Pop() interface{} {
	old := *cr
	n := len(old)
	item := old[n-1]
	item.position = -1
	*cr = old[0 : n-1]

	return item
}
