package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type TaskQueueService interface {
	FindNextTask(string, TaskSpec) (*TaskQueueItem, error)
	Refresh(string) error
	RefreshFindNextTask(string, TaskSpec) (*TaskQueueItem, error)
}

type TaskDistroQueueService interface {
	Refresh() error
	FindNextTask(TaskSpec) *TaskQueueItem
}

type TaskDistroQueueServiceConstructor func(string, []TaskQueueItem, time.Duration) TaskDistroQueueService

type taskDispatchService struct {
	taskDistroQueueServices map[string]TaskDistroQueueService
	mu                      sync.RWMutex
	ttl                     time.Duration
}

func NewTaskDispatchService(ttl time.Duration) TaskQueueService {
	return &taskDispatchService{
		ttl:                     ttl,
		taskDistroQueueServices: map[string]TaskDistroQueueService{},
	}
}

func (s *taskDispatchService) FindNextTask(distro string, spec TaskSpec) (*TaskQueueItem, error) {
	distroDispatchService, err := s.ensureQueue(distro)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return distroDispatchService.FindNextTask(spec), nil
}

func (s *taskDispatchService) RefreshFindNextTask(distro string, spec TaskSpec) (*TaskQueueItem, error) {
	distroDispatchService, err := s.ensureQueue(distro)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := distroDispatchService.Refresh(); err != nil {
		return nil, errors.WithStack(err)
	}

	return distroDispatchService.FindNextTask(spec), nil
}

func (s *taskDispatchService) Refresh(distro string) error {
	distroDispatchService, err := s.ensureQueue(distro)
	if err != nil {
		return errors.WithStack(err)
	}

	if err := distroDispatchService.Refresh(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (s *taskDispatchService) ensureQueue(distro string) (TaskDistroQueueService, error) {
	// If there is a "distro": *taskDistroDispatchService in the taskDistroQueueServices map, return that.
	// Otherwise, get the "distro"'s taskQueue from the database; seed its taskDistroDispatchService; put that in the map and return it.
	s.mu.RLock()

	distroDispatchService, ok := s.taskDistroQueueServices[distro]
	s.mu.RUnlock()
	if ok {
		return distroDispatchService, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	distroDispatchService, ok = s.taskDistroQueueServices[distro]
	if ok {
		return distroDispatchService, nil
	}

	taskQueue, err := FindDistroTaskQueue(distro)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	taskQueueItems := taskQueue.Queue
	distroDispatchService = newDistroTaskDispatchService(distro, taskQueueItems, s.ttl)
	s.taskDistroQueueServices[distro] = distroDispatchService

	return distroDispatchService, nil
}

// taskDistroDispatchService is an in-memory representation of schedulable tasks for a distro.
//
// TODO Pass all task group tasks, not just dispatchable ones, to the constructor.
type taskDistroDispatchService struct {
	mu          sync.RWMutex
	distroID    string
	order       []string
	units       map[string]schedulableUnit
	ttl         time.Duration
	lastUpdated time.Time
}

// schedulableUnit represents a unit of tasks that must be kept together in the queue because they
// are pinned to some number of hosts. That is, it represents tasks and task groups, but it does
// _not_ represent builds or DAGs of tasks and their dependencies.
type schedulableUnit struct {
	id           string
	runningHosts int // number of hosts unit is currently running on
	maxHosts     int // number of hosts unit can run on
	tasks        []TaskQueueItem
}

// newTaskDispatchService creates a taskDistroDispatchService from a slice of TaskQueueItems.
func newDistroTaskDispatchService(distroID string, items []TaskQueueItem, ttl time.Duration) TaskDistroQueueService {
	t := &taskDistroDispatchService{
		distroID: distroID,
		ttl:      ttl,
	}

	if len(items) != 0 {
		t.rebuild(items)
	}

	return t
}

func (t *taskDistroDispatchService) Refresh() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !shouldRefreshCached(t.ttl, t.lastUpdated) {
		return nil
	}

	taskQueue, err := FindDistroTaskQueue(t.distroID)
	if err != nil {
		return errors.WithStack(err)
	}

	taskQueueItems := taskQueue.Queue
	t.rebuild(taskQueueItems)

	return nil
}

func shouldRefreshCached(ttl time.Duration, lastUpdated time.Time) bool {
	return lastUpdated.IsZero() || time.Since(lastUpdated) > ttl
}

func (t *taskDistroDispatchService) rebuild(items []TaskQueueItem) {
	// This slice likely has too much capacity, but it helps append performance.
	order := make([]string, 0, len(items))
	units := map[string]schedulableUnit{}
	var ok bool
	var unit schedulableUnit
	var id string

	for _, item := range items {
		if item.Group == "" {
			order = append(order, item.Id)
			units[item.Id] = schedulableUnit{
				id:       item.Id,
				maxHosts: 0, // maxHosts == 0 indicates not a task group
				tasks:    []TaskQueueItem{item},
			}
		} else {
			// If it's the first time encountering the task group, save it to the order
			// and create an entry for it in the map. Otherwise, append to the
			// TaskQueueItem array in the map.
			id = compositeGroupId(item.Group, item.BuildVariant, item.Version)
			if _, ok = units[id]; !ok {
				order = append(order, id)
				units[id] = schedulableUnit{
					id:       id,
					maxHosts: item.GroupMaxHosts,
					tasks:    []TaskQueueItem{item},
				}
			} else {
				unit = units[id]
				unit.tasks = append(unit.tasks, item)
				units[id] = unit
			}
		}
	}

	t.order = order
	t.units = units
	t.lastUpdated = time.Now()

	return
}

// FindNextTask returns the next dispatchable task in the queue.
func (t *taskDistroDispatchService) FindNextTask(spec TaskSpec) *TaskQueueItem {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If units (map[string]schedulableUnit) is empty, the queue has been emptied. Reset order as an optimization so that the
	// service no longer needs to iterate over it and check each item against the map.
	if len(t.units) == 0 {
		t.order = []string{}
		return nil
	}

	var unit schedulableUnit
	var ok bool
	var next *TaskQueueItem

	if spec.Group != "" {
		unit, ok = t.units[compositeGroupId(spec.Group, spec.BuildVariant, spec.Version)]
		if ok {
			if next = t.nextTaskGroupTask(unit); next != nil {
				return next
			}
		}
		// If the task group is not present in the task group map, it has been dispatched.
		// Fall through to getting a task not in that group.
	}

	var numHosts int
	var err error

	for _, schedulableUnitID := range t.order {
		unit, ok = t.units[schedulableUnitID]
		if !ok {
			continue
		}
		// If maxHosts is not set, this is not a task group.
		if unit.maxHosts == 0 {
			delete(t.units, schedulableUnitID)
			return &unit.tasks[0]
		}
		if unit.runningHosts < unit.maxHosts {
			// TODO: For a multi-host task group, it's not possible to correctly
			// dispatch tasks based on number of hosts running tasks in the task group
			// without a transaction. When we use a driver that supports transactions,
			// we likely want to rewrite this code to use transactions. Currently it's
			// possible to dispatch a multi-host task group to more tasks than max
			// hosts. Before transactions are available, it might be possible to do this
			// better by performing this query again, just before returning from this
			// function.
			numHosts, err = host.NumHostsByTaskSpec(spec.Group, spec.BuildVariant, spec.ProjectID, spec.Version)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "problem running NumHostsByTaskSpec query",
					"group":   spec.Group,
					"variant": spec.BuildVariant,
					"project": spec.ProjectID,
					"version": spec.Version,
				}))
				return nil
			}
			unit.runningHosts = numHosts
			t.units[schedulableUnitID] = unit
			if unit.runningHosts < unit.maxHosts {
				if next = t.nextTaskGroupTask(unit); next != nil {
					return next
				}
			}
		}
	}

	return nil
}

func compositeGroupId(group, variant, version string) string {
	return fmt.Sprintf("%s-%s-%s", group, variant, version)
}

func (t *taskDistroDispatchService) nextTaskGroupTask(unit schedulableUnit) *TaskQueueItem {
	for i, nextTask := range unit.tasks {
		if nextTask.IsDispatched == true {
			continue
		}

		nextTaskFromDB, err := task.FindOneId(nextTask.Id)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem finding task in db",
				"task":    nextTask.Id,
			}))
			return nil
		}
		if nextTaskFromDB == nil {
			grip.Error(message.Fields{
				"message": "task from db not found",
				"task":    nextTask.Id,
			})
			return nil
		}

		if t.isBlockedSingleHostTaskGroup(unit, nextTaskFromDB) {
			delete(t.units, unit.id)
			return nil
		}

		// Cache dispatched status.
		t.units[unit.id].tasks[i].IsDispatched = true
		unit.tasks[i].IsDispatched = true

		if nextTaskFromDB.StartTime != util.ZeroTime {
			continue
		}
		// Don't cache dispatched status when returning the next TaskQueueItem - in case the task fails to start.
		return &nextTask
	}
	// If all the tasks have been dispatched, remove the unit.
	delete(t.units, unit.id)
	return nil
}

// isBlockedSingleHostTaskGroup checks if the task is running in a 1-host task group, has finished,
// and did not succeed. But rely on EndTask to block later tasks.
func (t *taskDistroDispatchService) isBlockedSingleHostTaskGroup(unit schedulableUnit, dbTask *task.Task) bool {
	return unit.maxHosts == 1 && !util.IsZeroTime(dbTask.FinishTime) && dbTask.Status != evergreen.TaskSucceeded
}
