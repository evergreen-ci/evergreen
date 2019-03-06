package model

import (
	"fmt"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// taskDispatchService is an in-memory representation of schedulable tasks for a distro.
//
// TODO Pass all task group tasks, not just dispatchable ones, to the constructor.
// TODO Maintain an in-memory global (but per app server) task queue per distro.
type taskDispatchService struct {
	mu       sync.Mutex
	distroID string
	order    []string
	units    map[string]schedulableUnit
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

// NewTaskDispatchService creates a taskDispatchService from a slice of TaskQueueItems.
func NewTaskDispatchService(distroID string, items []TaskQueueItem) *taskDispatchService {
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
				units[id] = schedulableUnit{
					id:       id,
					maxHosts: item.GroupMaxHosts,
					tasks:    []TaskQueueItem{item},
				}
				order = append(order, id)
			} else {
				unit = units[id]
				unit.tasks = append(unit.tasks, item)
				units[id] = unit
			}
		}
	}
	t := &taskDispatchService{
		distroID: distroID,
		order:    order,
		units:    units,
	}
	return t
}

// FindNextTask returns the next dispatchable task in the queue.
func (t *taskDispatchService) FindNextTask(spec TaskSpec) *TaskQueueItem {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If units is empty, the queue has been emptied. Reset order as an optimization so that the
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

func (t *taskDispatchService) nextTaskGroupTask(unit schedulableUnit) *TaskQueueItem {
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
			return nil
		}

		// Cache dispatched status.
		t.units[unit.id].tasks[i].IsDispatched = true
		unit.tasks[i].IsDispatched = true

		if nextTaskFromDB.StartTime != util.ZeroTime {
			continue
		}
		// Don't cache dispatched status when returning nextTask, in case the task fails to start.
		return &nextTask
	}
	// If all the tasks have been dispatched, remove the unit.
	delete(t.units, unit.id)
	return nil
}

// isBlockedSingleHostTaskGroup checks if the task is running in a 1-host task group, has finished,
// and did not succeed. If so it removes the unit from the local queue. But rely on EndTask to
// block later tasks.
func (t *taskDispatchService) isBlockedSingleHostTaskGroup(unit schedulableUnit, dbTask *task.Task) bool {
	if unit.maxHosts == 1 && !util.IsZeroTime(dbTask.FinishTime) && dbTask.Status != evergreen.TaskSucceeded {
		delete(t.units, unit.id)
		return true
	}
	return false
}
