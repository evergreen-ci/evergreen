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

type taskDispatchService struct {
	queues map[string]*taskDistroDispatchService
	mu     sync.RWMutex
	ttl    time.Duration
}

func NewTaskQueueService(ttl time.Duration) TaskQueueService {
	grip.Info(message.Fields{
		"logged_by": "brian",
		"message":   "begin NewTaskDispatchService",
		"ttl":       ttl,
	})
	defer func() {
		grip.Info(message.Fields{
			"logged_by": "brian",
			"message":   "end NewTaskQueueService",
		})
	}()
	return &taskDispatchService{
		ttl:    ttl,
		queues: map[string]*taskDistroDispatchService{},
	}
}

func (s *taskDispatchService) FindNextTask(distro string, spec TaskSpec) (*TaskQueueItem, error) {
	grip.Info(message.Fields{
		"logged_by": "brian",
		"message":   "begin taskDispatchService.FindNextTask(distro, spec)",
	})
	defer func() {
		grip.Info(message.Fields{
			"logged_by": "brian",
			"message":   "end taskDispatchService.FindNextTask(distro, spec)",
		})
	}()
	queue, err := s.ensureQueue(distro)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return queue.FindNextTask(spec), nil
}

func (s *taskDispatchService) Refresh(distro string) error {
	queue, err := s.ensureQueue(distro)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(queue.Refresh())
}

func (s *taskDispatchService) RefreshFindNextTask(distro string, spec TaskSpec) (*TaskQueueItem, error) {
	grip.Info(message.Fields{
		"logged_by": "brian",
		"message":   "begin taskDispatchService.RefreshFindNextTask(distro, spec)",
		"distro":    distro,
		"spec":      spec,
	})
	defer func() {
		grip.Info(message.Fields{
			"logged_by": "brian",
			"message":   "end taskDispatchService.RefreshFindNextTask(distro, spec)",
			"distro":    distro,
			"spec":      spec,
		})
	}()
	queue, err := s.ensureQueue(distro)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := queue.Refresh(); err != nil {
		return nil, errors.WithStack(queue.Refresh())
	}
	return queue.FindNextTask(spec), nil
}

func (s *taskDispatchService) ensureQueue(distro string) (*taskDistroDispatchService, error) {
	grip.Info(message.Fields{
		"logged_by":    "brian",
		"message":      "begin taskDispatchService.ensureQueue(distro)",
		"distro":       distro,
		"current_time": time.Now(),
	})
	defer func() {
		grip.Info(message.Fields{
			"logged_by":    "brian",
			"message":      "end taskDispatchService.ensureQueue(distro)",
			"distro":       distro,
			"current_time": time.Now(),
		})
	}()
	s.mu.RLock()
	queue, ok := s.queues[distro]
	s.mu.RUnlock()
	if ok {
		grip.Info(message.Fields{
			"logged_by": "brian",
			"message":   "We got a cached queue inside taskDispatchService.ensureQueue(distro)",
			"len_order": len(queue.order),
			"len_units": len(queue.units),
			"distro":    distro,
		})
		return queue, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	queue, ok = s.queues[distro]
	if ok {
		return queue, nil
	}

	grip.Info(message.Fields{
		"logged_by": "brian",
		"message":   "We did NOT got a cached queue inside taskDispatchService.ensureQueue(distro)",
		"distro":    distro,
	})

	grip.Info(message.Fields{
		"logged_by":    "brian",
		"message":      "Inside taskDispatchService.ensureQueue(distro) and constructing a newDistroTaskDispatchService(distro, nil, ttl)",
		"current_time": time.Now(),
	})

	// STU: we will not call rebuild() inside newDistroTaskDispatchService as we are not nil instead of a slice of TaskQueueItems, right?
	queue = newDistroTaskDispatchService(distro, nil, s.ttl)
	s.queues[distro] = queue
	grip.Info(message.Fields{
		"logged_by": "brian",
		"message":   "Inside taskDispatchService.ensureQueue(distro) and calling taskDistroDispatchService.Refresh()",
	})

	if err := queue.Refresh(); err != nil {
		return nil, errors.WithStack(err)
	}

	grip.Info(message.Fields{
		"logged_by": "brian",
		"message":   "Inside taskDispatchService.ensureQueue(distro) and about to return the distro's queue which is a *taskDistroDispatchService",
	})

	return queue, nil
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
func newDistroTaskDispatchService(distroID string, items []TaskQueueItem, ttl time.Duration) *taskDistroDispatchService {
	grip.Info(message.Fields{
		"logged_by":    "brian",
		"message":      "begin newDistroTaskDispatchService",
		"distro":       distroID,
		"items_length": len(items),
		"ttl":          ttl,
	})
	defer func() {
		grip.Info(message.Fields{
			"logged_by":    "brian",
			"message":      "end newDistroTaskDispatchService",
			"distro":       distroID,
			"items_length": len(items),
			"ttl":          ttl,
		})
	}()
	t := &taskDistroDispatchService{
		distroID: distroID,
		ttl:      ttl,
		// lastUpdated: time.Now(),
	}

	if items != nil {
		grip.Info(message.Fields{
			"logged_by":    "brian",
			"message":      "I'm inside newDistroTaskDispatchService and I'm calling rebuild(items []TaskQueueItem])",
			"distro":       distroID,
			"items_length": len(items),
			"ttl":          ttl,
		})
		t.rebuild(items)
	} else {
		grip.Info(message.Fields{
			"logged_by":    "brian",
			"message":      "I'm inside newDistroTaskDispatchService and I'm NOT calling rebuild(items []TaskQueueItem]), as items is nil",
			"distro":       distroID,
			"items_length": len(items),
			"ttl":          ttl,
		})
	}

	return t
}

func (t *taskDistroDispatchService) Refresh() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// if !t.shouldRefresh() {
	// 	return nil
	// }

	if !shouldRefreshCached(t.ttl, t.lastUpdated) {
		grip.Info(message.Fields{
			"logged_by": "brian",
			"message":   "I'm inside taskDistroDispatchService.Refresh() and shouldRefreshCached() has returned false",
			"distro":    t.distroID,
			"ttl":       t.ttl,
		})
		return nil
	}
	grip.Info(message.Fields{
		"logged_by": "brian",
		"message":   "I'm inside taskDistroDispatchService.Refresh() and shouldRefreshCached() has returned true",
		"distro":    t.distroID,
		"ttl":       t.ttl,
	})

	queue, err := FindDistroTaskQueue(t.distroID)
	if err != nil {
		return errors.WithStack(err)
	}

	grip.Info(message.Fields{
		"logged_by":    "brian",
		"message":      "I'm inside taskDistroDispatchService.Refresh() and this is the queue returned by FindDistroTaskQueue",
		"distro":       queue.Distro,
		"generated_at": queue.GeneratedAt,
		"queue_length": len(queue.Queue),
	})

	grip.Info(message.Fields{
		"logged_by":    "brian",
		"message":      "I'm inside taskDistroDispatchService.Refresh() and I'm going to call taskDistroDispatchService,rebuild(TaskQueue)",
		"distro":       queue.Distro,
		"generated_at": queue.GeneratedAt,
		"queue_length": len(queue.Queue),
	})
	t.rebuild(queue.Queue)

	return nil
}

func shouldRefreshCached(ttl time.Duration, lastUpdated time.Time) bool {
	// last_updated:0001-01-01T00:00:00Z.IsZero() || time.Since(0001-01-01T00:00:00Z) > 60 seconds
	grip.Info(message.Fields{
		"logged_by":                     "brian",
		"message":                       "Inside func shouldRefreshCached(ttl, lastupdated)",
		"lastUpdated.IsZero()":          lastUpdated.IsZero(),
		"ttl":                           ttl.Seconds(),
		"last_updated":                  lastUpdated,
		"time_since_lastupdated (secs)": time.Since(lastUpdated).Seconds(),
		"current_time":                  time.Now(),
		"shouldRefreshCached returned":  (lastUpdated.IsZero() || time.Since(lastUpdated) > ttl),
	})

	return lastUpdated.IsZero() || time.Since(lastUpdated) > ttl
}

// func (t *taskDistroDispatchService) shouldRefresh() bool {
// 	if shouldRefreshCached(t.ttl, t.lastUpdated) {
// 		t.lastUpdated = time.Now()
// 		return true
// 	}
//
// 	return false
// }

func (t *taskDistroDispatchService) rebuild(items []TaskQueueItem) {
	grip.Info(message.Fields{
		"logged_by":                     "brian",
		"message":                       "beginning taskDistroDispatchService.rebuild(items []TaskQueueItem)",
		"ttl":                           t.ttl.Seconds(),
		"len_items":                     len(items),
		"last_updated":                  t.lastUpdated,
		"time_since_lastupdated (secs)": time.Since(t.lastUpdated).Seconds(),
		"current_time":                  time.Now(),
	})

	// {    [-]
	//       last_updated:   0001-01-01T00:00:00Z
	//       len_items:      7
	//       logged_by:      brian
	//       message:        begin rebuild
	//       metadata:      {       [+]
	//      }
	//       time_since_lastupdated (secs):  9223372036.854776
	//       ttl:    60
	//  }

	if !shouldRefreshCached(t.ttl, t.lastUpdated) {
		// STU: why does it appear that we are now no longer ever getting in here?
		grip.Info(message.Fields{
			"logged_by":    "brian",
			"message":      "Inside taskDistroDispatchService.rebuild(items []TaskQueueItem) and shouldRefreshCached() has returned false",
			"current_time": time.Now(),
		})

		return
	}
	grip.Info(message.Fields{
		"logged_by":    "brian",
		"message":      "Inside taskDistroDispatchService.rebuild(items []TaskQueueItem) and shouldRefreshCached() has returned true",
		"current_time": time.Now(),
	})

	grip.Info(message.Fields{
		"logged_by":    "brian",
		"current_time": time.Now(),
		"message":      fmt.Sprintf("Inside taskDistroDispatchService.rebuild(items []TaskQueueItem) and the current value of t.lastUpdated is %s", t.lastUpdated),
		"next":         fmt.Sprintf("Inside taskDistroDispatchService.rebuild(items []TaskQueueItem) and now I'm going to set its value of %s", time.Now()),
	})

	t.lastUpdated = time.Now() // STU: Is this the correct place to do this?

	// This slice likely has too much capacity, but it helps append performance.
	order := make([]string, 0, len(items))
	units := map[string]schedulableUnit{}
	var ok bool
	var unit schedulableUnit
	var id string

	for _, item := range items {
		if item.Group == "" {
			grip.Info(message.Fields{
				"logged_by": "brian",
				"message":   fmt.Sprintf("item.Group == \"\", so adding item.Id '%s' to order string slice", item.Id),
			})
			order = append(order, item.Id)
			units[item.Id] = schedulableUnit{
				id:       item.Id,
				maxHosts: 0, // maxHosts == 0 indicates not a task group
				tasks:    []TaskQueueItem{item},
			}
		} else {
			// STU: we only get in here if we actually have taskGroups in play, right?
			// If it's the first time encountering the task group, save it to the order
			// and create an entry for it in the map. Otherwise, append to the
			// TaskQueueItem array in the map.
			id = compositeGroupId(item.Group, item.BuildVariant, item.Version)
			if _, ok = units[id]; !ok {
				// grip.Info(message.Fields{
				//      "logged_by": "brian",
				//      "message":   fmt.Sprintf("Setting '%s' key in the units map for the first time", item.Id),
				//      "and":       fmt.Sprintf("adding compositeGroupId '%s' to order string slice", id),
				// })
				units[id] = schedulableUnit{
					id:       id,
					maxHosts: item.GroupMaxHosts,
					tasks:    []TaskQueueItem{item},
				}
				order = append(order, id)
			} else {
				// grip.Info(message.Fields{
				//      "logged_by": "brian",
				//      "message":   fmt.Sprintf("Updating item.Id '%s' key in the units map", item.Id),
				// })
				unit = units[id]
				unit.tasks = append(unit.tasks, item)
				units[id] = unit
			}
		}
	}

	grip.Info(message.Fields{
		"logged_by": "brian",
		"message":   "Inside taskDistroDispatchService.rebuild(items []TaskQueueItem) - finished rebuild",
		"ttl":       t.ttl,
		"len_order": len(order),
		"len_units": len(units),
	})

	// {
	//       len_order:      52
	//       len_units:      52
	//       logged_by:      brian
	//       message:        post rebuild
	//       metadata:      {       [-]
	//       hostname:       evergreenapp-1.staging.build.10gen.cc
	//       level:  40
	//       pid:    11709
	//       process:        /srv/evergreen/current/clients/linux_amd64/evergreen
	//       time:   2019-04-29T17:10:17.747666363Z
	//      }
	//       ttl:    60000000000
	// }

	t.order = order
	t.units = units
	t.lastUpdated = time.Now()
	return
}

// FindNextTask returns the next dispatchable task in the queue.
func (t *taskDistroDispatchService) FindNextTask(spec TaskSpec) *TaskQueueItem {
	grip.Info(message.Fields{
		"logged_by": "brian",
		"message":   "beginning taskDistroDispatchService.FindNextTask(spec TaskSpec)",
		"spec":      spec,
	})
	defer func() {
		grip.Info(message.Fields{
			"logged_by": "brian",
			"message":   "end taskDistroDispatchService.FindNextTask(spec TaskSpec)",
			"spec":      spec,
		})
	}()
	t.mu.Lock()
	defer t.mu.Unlock()

	// If units is empty, the queue has been emptied. Reset order as an optimization so that the
	// service no longer needs to iterate over it and check each item against the map.
	if len(t.units) == 0 {
		grip.Info(message.Fields{
			"logged_by":       "brian",
			"message":         "Inside taskDistroDispatchService.FindNextTask(spec TaskSpec) and len(t.units) == 0",
			"spec":            spec,
			"len_order_slice": len(t.order),
		})
		t.order = []string{}
		return nil
	}

	grip.Info(message.Fields{
		"logged_by":       "brian",
		"message":         fmt.Sprintf("taskDistroDispatchService.FindNextTask(spec TaskSpec) and len(t.units) is %d", len(t.units)),
		"len_order_slice": len(t.order),
		"spec":            spec,
	})

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

	grip.Info(message.Fields{
		"logged_by": "brian",
		"message":   "taskDistroDispatchService.FindNextTask(spec TaskSpec) and falling through to getting a task not in that group",
		"spec":      spec,
		"unit":      unit,
	})

	var numHosts int
	var err error
	for _, schedulableUnitID := range t.order {
		grip.Info(message.Fields{
			"logged_by": "brian",
			"message":   "got a unit",
			"spec":      spec,
			"unit":      unit,
		})
		unit, ok = t.units[schedulableUnitID]
		if !ok {
			continue
		}
		// If maxHosts is not set, this is not a task group.
		if unit.maxHosts == 0 {
			grip.Info(message.Fields{
				"logged_by": "brian",
				"message":   "maxHosts not set",
				"spec":      spec,
				"unit":      unit,
			})
			delete(t.units, schedulableUnitID)
			grip.Info(message.Fields{
				"logged_by":                 "brian",
				"message":                   "taskDistroDispatchService.FindNextTask(spec TaskSpec) and Ahoy there from just after: delete(t.units, schedulableUnitID)",
				"schedulable_unit_id":       schedulableUnitID,
				"order_string_slice_length": len(t.order),
				"units_strings_to_schedulableunits_length": len(t.units),
			})
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
			grip.Info(message.Fields{
				"logged_by": "brian",
				"message":   "got numHosts",
				"numHosts":  numHosts,
				"spec":      spec,
				"unit":      unit,
			})
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
func (t *taskDistroDispatchService) isBlockedSingleHostTaskGroup(unit schedulableUnit, dbTask *task.Task) bool {
	if unit.maxHosts == 1 && !util.IsZeroTime(dbTask.FinishTime) && dbTask.Status != evergreen.TaskSucceeded {
		delete(t.units, unit.id)
		return true
	}
	return false
}
