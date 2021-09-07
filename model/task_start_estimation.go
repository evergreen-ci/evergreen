package model

import (
	"sort"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	// average observed time for host to go from this status to running
	hostInitializingDelay  = 4 * time.Minute
	hostStartingDelay      = 3 * time.Minute
	hostProvisiongingDelay = 1 * time.Minute
)

type estimatedTask struct {
	duration time.Duration
}

type estimatedHost struct {
	timeToCompletion time.Duration
}

type estimatedHostPool []estimatedHost

type estimatedTimeSimulator struct {
	tasks       estimatedTaskQueue
	hosts       estimatedHostPool
	timeElapsed time.Duration
	currentPos  int
}

func (h *estimatedHost) decrementTime(t time.Duration) {
	h.timeToCompletion -= t
}

func (p estimatedHostPool) Len() int { return len(p) }
func (p estimatedHostPool) Swap(i, j int) {
	temp := p[j]
	p[j] = p[i]
	p[i] = temp
}
func (p estimatedHostPool) Less(i, j int) bool { return p[i].timeToCompletion < p[j].timeToCompletion }

func (s *estimatedTimeSimulator) simulate(pos int) time.Duration {
	if len(s.hosts) == 0 {
		return -1
	}
	if s.tasks.IsEmpty() {
		return -1
	}
	sort.Sort(s.hosts)
	for s.currentPos <= pos {
		s.dispatchNextTask()
		s.currentPos++
	}

	return s.timeElapsed
}

func (s *estimatedTimeSimulator) dispatchNextTask() {
	count := len(s.hosts)
	// fast forward time until the soonest host completes its task
	fastForwardTime := s.hosts[0].timeToCompletion
	s.timeElapsed += fastForwardTime
	s.hosts = s.hosts[1:]
	for i := range s.hosts {
		s.hosts[i].decrementTime(fastForwardTime)
	}

	// dequeue the next task
	nextTask := s.tasks.Dequeue()
	newlyDispatched := estimatedHost{timeToCompletion: nextTask.duration}

	// assign it to the host that just completed and move it back into the pool in order
	for i := 0; i < count; i++ {
		if i < count-2 {
			if s.hosts[i].timeToCompletion <= nextTask.duration &&
				s.hosts[i+1].timeToCompletion >= nextTask.duration {
				s.hosts = append(s.hosts[:i], append([]estimatedHost{newlyDispatched}, s.hosts[i:]...)...)
				return
			}
		} else {
			s.hosts = append(s.hosts, newlyDispatched)
			return
		}
	}
}

// GetEstimatedStartTime returns the estimated start time for a task
func GetEstimatedStartTime(t task.Task) (time.Duration, error) {
	queue, err := LoadTaskQueue(t.DistroId)
	if err != nil {
		return -1, errors.Wrap(err, "error retrieving task queue")
	}
	if queue == nil {
		return -1, nil
	}
	queuePos := -1
	for i, q := range queue.Queue {
		if q.Id == t.Id {
			queuePos = i
			break
		}
	}
	if queuePos == -1 {
		return -1, nil
	}
	hosts, err := host.Find(db.Query(host.ByDistroIDs(t.DistroId)))
	if err != nil {
		return -1, errors.Wrap(err, "error retrieving hosts")
	}
	return createSimulatorModel(*queue, hosts).simulate(queuePos), nil
}

func createSimulatorModel(taskQueue TaskQueue, hosts []host.Host) *estimatedTimeSimulator {
	estimator := estimatedTimeSimulator{}
	for i := 0; i < len(taskQueue.Queue); i++ {
		_ = estimator.tasks.Enqueue(estimatedTask{duration: taskQueue.Queue[i].ExpectedDuration})
	}
	for _, h := range hosts {
		switch h.Status {
		case evergreen.HostUninitialized:
			estimator.hosts = append(estimator.hosts, estimatedHost{timeToCompletion: hostInitializingDelay})
		case evergreen.HostStarting:
			estimator.hosts = append(estimator.hosts, estimatedHost{timeToCompletion: hostStartingDelay})
		case evergreen.HostProvisioning:
			estimator.hosts = append(estimator.hosts, estimatedHost{timeToCompletion: hostProvisiongingDelay})
		case evergreen.HostRunning:
			if h.RunningTask == "" {
				estimator.hosts = append(estimator.hosts, estimatedHost{timeToCompletion: 0})
			} else {
				t, err := task.FindOne(task.ById(h.RunningTask))
				if err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"message": "retrieving tasks for task start estimation simulator",
					}))
					return &estimator
				}
				if t == nil {
					grip.Error(message.Fields{
						"message": "task does not exist, can't estimate its duration",
						"task_id": h.RunningTask,
					})
					continue
				}
				elapsed := time.Since(t.DispatchTime)
				estimator.hosts = append(estimator.hosts, estimatedHost{timeToCompletion: t.ExpectedDuration - elapsed})
			}
		}
	}

	return &estimator
}

type estimatedTaskQueue struct {
	Items []estimatedTask
	mu    sync.RWMutex
}

func NewQueue() estimatedTaskQueue {
	return estimatedTaskQueue{
		Items: []estimatedTask{},
	}
}

func (q *estimatedTaskQueue) Enqueue(item estimatedTask) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Items = append(q.Items, item)

	return nil
}

func (q *estimatedTaskQueue) Dequeue() *estimatedTask {
	if q.IsEmpty() {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	item := q.Items[0]
	q.Items = q.Items[1:]
	return &item
}

func (q *estimatedTaskQueue) IsEmpty() bool {
	return q.Length() == 0
}

func (q *estimatedTaskQueue) Peek() *estimatedTask {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.IsEmpty() {
		return nil
	}
	return &q.Items[0]
}

func (q *estimatedTaskQueue) Length() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.Items)
}
