package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// type TaskQueueItemDispatcher interface {
// 	FindNextTask(string, TaskSpec) (*TaskQueueItem, error)
// 	Refresh(string) error
// 	RefreshFindNextTask(string, TaskSpec) (*TaskQueueItem, error)
// }

type TaskQueueService interface {
	FindNextTask(string, TaskSpec) (*TaskQueueItem, error)
	Refresh(string) error
	RefreshFindNextTask(string, TaskSpec) (*TaskQueueItem, error)
}

type CachedDispatcher interface {
	Refresh() error
	FindNextTask(TaskSpec) *TaskQueueItem
}

type taskDispatchService struct {
	cachedDispatchers map[string]CachedDispatcher
	mu                sync.RWMutex
	ttl               time.Duration
}

func NewTaskDispatchService(ttl time.Duration) TaskQueueService {
	return &taskDispatchService{
		ttl:               ttl,
		cachedDispatchers: map[string]CachedDispatcher{},
	}
}

func (s *taskDispatchService) FindNextTask(distroID string, spec TaskSpec) (*TaskQueueItem, error) {
	distroDispatchService, err := s.ensureQueue(distroID)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return distroDispatchService.FindNextTask(spec), nil
}

func (s *taskDispatchService) RefreshFindNextTask(distroID string, spec TaskSpec) (*TaskQueueItem, error) {

	distroDispatchService, err := s.ensureQueue(distroID)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := distroDispatchService.Refresh(); err != nil {
		return nil, errors.WithStack(err)
	}

	return distroDispatchService.FindNextTask(spec), nil
}

func (s *taskDispatchService) Refresh(distroID string) error {
	distroDispatchService, err := s.ensureQueue(distroID)
	if err != nil {
		return errors.WithStack(err)
	}

	if err := distroDispatchService.Refresh(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (s *taskDispatchService) ensureQueue(distroID string) (CachedDispatcher, error) {
	// If there is a "distro": *basicCachedDispatcherImpl in the cachedDispatchers map, return that.
	// Otherwise, get the "distro"'s taskQueue from the database; seed its cachedDispatcher; put that in the map and return it.
	s.mu.RLock()

	distroDispatchService, ok := s.cachedDispatchers[distroID]
	s.mu.RUnlock()
	if ok {
		return distroDispatchService, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	distroDispatchService, ok = s.cachedDispatchers[distroID]
	if ok {
		return distroDispatchService, nil
	}

	taskQueue, err := FindDistroTaskQueue(distroID)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	d, err := distro.FindOne(distro.ById(distroID))
	if err != nil {
		return nil, errors.Wrapf(err, "Database error for find() by distro id '%s'", distroID)
	}

	config, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving Evergreen config")
	}
	plannerSettings, err := d.GetResolvedPlannerSettings(config)
	if err != nil {
		return nil, errors.Wrapf(err, "error resolving the PlannerSettings for distro '%s'", d.Id)
	}

	switch plannerSettings.Version {
	case evergreen.PlannerVersionTunable:
		distroDispatchService, err := newDistroTaskDAGDispatchService(taskQueue, s.ttl)
		if err != nil {
			return nil, err
		}
		s.cachedDispatchers[distroID] = distroDispatchService
		return distroDispatchService, nil
	default:
		distroDispatchService = newDistroTaskDispatchService(taskQueue, s.ttl)
		s.cachedDispatchers[distroID] = distroDispatchService
		return distroDispatchService, nil
	}
}

// cachedDispatcher is an in-memory representation of schedulable tasks for a distro.
//
// TODO Pass all task group tasks, not just dispatchable ones, to the constructor.
type basicCachedDispatcherImpl struct {
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
	group        string
	project      string
	version      string
	variant      string
	runningHosts int // number of hosts unit is currently running on
	maxHosts     int // number of hosts unit can run on
	tasks        []TaskQueueItem
}

// newDistroTaskDispatchService creates a basicCachedDispatcherImpl from a slice of TaskQueueItems.
func newDistroTaskDispatchService(taskQueue TaskQueue, ttl time.Duration) *basicCachedDispatcherImpl {
	t := &basicCachedDispatcherImpl{
		distroID: taskQueue.Distro,
		ttl:      ttl,
	}

	if taskQueue.Length() != 0 {
		t.rebuild(taskQueue.Queue)
	}

	grip.Debug(message.Fields{
		"dispatcher":   "schedulableunit-task-dispatcher",
		"function":     "newDistroTaskDispatchService",
		"message":      "initializing new basicCachedDispatcherImpl for a distro",
		"distro_id":    t.distroID,
		"order_length": len(t.order),
		"units_length": len(t.units),
		"ttl":          t.ttl,
		"last_updated": t.lastUpdated,
	})
	return t
}

func (t *basicCachedDispatcherImpl) Refresh() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !shouldRefreshCached(t.ttl, t.lastUpdated, t.distroID) {
		return nil
	}

	taskQueue, err := FindDistroTaskQueue(t.distroID)
	if err != nil {
		return errors.WithStack(err)
	}

	taskQueueItems := taskQueue.Queue
	t.rebuild(taskQueueItems)

	grip.Debug(message.Fields{
		"dispatcher":   "schedulableunit-task-dispatcher",
		"function":     "Refresh",
		"message":      "refresh was successful",
		"distro_id":    t.distroID,
		"order_length": len(t.order),
		"units_length": len(t.units),
	})

	return nil
}

func shouldRefreshCached(ttl time.Duration, lastUpdated time.Time, distroID string) bool {
	grip.DebugWhen(time.Since(lastUpdated) > ttl, message.Fields{
		"dispatcher":            "schedulableunit-task-dispatcher",
		"function":              "shouldRefreshCached",
		"message":               "it's time to rebuild the order and units representations from the distro's TaskQueueItems",
		"distro_id":             distroID,
		"ttl":                   ttl.Seconds(),
		"current_time":          time.Now(),
		"last_updated":          lastUpdated,
		"sec_since_last_update": time.Now().Sub(lastUpdated).Seconds(),
	})

	return lastUpdated.IsZero() || time.Since(lastUpdated) > ttl
}

func (t *basicCachedDispatcherImpl) rebuild(items []TaskQueueItem) {
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
				group:    item.Group,
				project:  item.Project,
				version:  item.Version,
				variant:  item.BuildVariant,
				maxHosts: 0, // maxHosts == 0 indicates not a task group
				tasks:    []TaskQueueItem{item},
			}
		} else {
			// If it's the first time encountering the task group, save it to the order
			// and create an entry for it in the map. Otherwise, append to the
			// TaskQueueItem array in the map.
			id = compositeGroupId(item.Group, item.BuildVariant, item.Project, item.Version)
			if _, ok = units[id]; !ok {
				order = append(order, id)
				units[id] = schedulableUnit{
					id:       id,
					group:    item.Group,
					project:  item.Project,
					version:  item.Version,
					variant:  item.BuildVariant,
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
}

// FindNextTask returns the next dispatchable task in the queue.
func (t *basicCachedDispatcherImpl) FindNextTask(spec TaskSpec) *TaskQueueItem {
	grip.Debug(message.Fields{
		"dispatcher":               "schedulableunit-task-dispatcher",
		"function":                 "FindNextTask",
		"message":                  "entered function",
		"order_length":             len(t.order),
		"units_length":             len(t.units),
		"taskspec_group":           spec.Group,
		"taskspec_build_variant":   spec.BuildVariant,
		"taskspec_version":         spec.Version,
		"taskspec_project":         spec.Project,
		"taskspec_group_max_hosts": spec.GroupMaxHosts,
		"distro_id":                t.distroID,
	})
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.units) == 0 && len(t.order) > 0 {
		grip.Debug(message.Fields{
			"dispatcher":   "schedulableunit-task-dispatcher",
			"function":     "FindNextTask",
			"message":      "t.units (map[string]schedulableUnit) is empty, but t.order ([]string) is not; resetting t.order = []string{} - returning nil",
			"distro_id":    t.distroID,
			"order_length": len(t.order),
			"units_length": len(t.units),
		})

		t.order = []string{}
		return nil
	}

	var unit schedulableUnit
	var ok bool
	var next *TaskQueueItem
	// If the host just ran a task group, give it one back.
	if spec.Group != "" {
		unit, ok = t.units[compositeGroupId(spec.Group, spec.BuildVariant, spec.Project, spec.Version)]
		if ok {
			if next = t.nextTaskGroupTask(unit); next != nil {
				return next
			}
		}
		// If the task group is not present in the task group map, it has been dispatched.
		// Fall through to get a task that's not in that task group.
		grip.Debug(message.Fields{
			"dispatcher":               "schedulableunit-task-dispatcher",
			"function":                 "FindNextTask",
			"message":                  "basicCachedDispatcherImpl.units[key] was not found - assuming it has been dispatched; falling through to try and get a task not in the current task group",
			"key":                      compositeGroupId(spec.Group, spec.BuildVariant, spec.Project, spec.Version),
			"taskspec_group":           spec.Group,
			"taskspec_build_variant":   spec.BuildVariant,
			"taskspec_version":         spec.Version,
			"taskspec_project":         spec.Project,
			"taskspec_group_max_hosts": spec.GroupMaxHosts,
			"distro_id":                t.distroID,
		})
	}

	var numHosts int
	var err error

	for _, schedulableUnitID := range t.order {
		unit, ok = t.units[schedulableUnitID]
		if !ok {
			grip.Debug(message.Fields{
				"dispatcher":         "schedulableunit-task-dispatcher",
				"function":           "FindNextTask",
				"message":            "basicCachedDispatcherImpl.units[schedulableUnitID] was not found",
				"distro_id":          t.distroID,
				"order_length":       len(t.order),
				"units_length":       len(t.units),
				"schedulableunit_id": schedulableUnitID,
				"taskspec_group":     spec.Group,
				"taskspec_variant":   spec.BuildVariant,
				"taskspec_project":   spec.Project,
				"taskspec_version":   spec.Version,
			})
			continue
		}

		// If maxHosts is not set, this is not a task group.
		if unit.maxHosts == 0 {
			delete(t.units, schedulableUnitID)
			if len(unit.tasks) == 0 {
				grip.Critical(message.Fields{
					"dispatcher":                    "schedulableunit-task-dispatcher",
					"function":                      "FindNextTask",
					"message":                       "schedulableUnit.maxHosts == 0 - this is not a task group; but schedulableUnit.tasks is empty - returning nil",
					"distro_id":                     t.distroID,
					"schedulableunit_id":            unit.id,
					"schedulableunit_group":         unit.group,
					"schedulableunit_project":       unit.project,
					"schedulableunit_version":       unit.version,
					"schedulableunit_variant":       unit.variant,
					"schedulableunit_running_hosts": unit.runningHosts,
					"schedulableunit_max_hosts":     unit.maxHosts,
					"schedulableunit_num_tasks":     len(unit.tasks),
					"task_id_returned":              "",
				})

				return nil
			}

			grip.Debug(message.Fields{
				"dispatcher":                    "schedulableunit-task-dispatcher",
				"function":                      "FindNextTask",
				"message":                       "schedulableUnit.maxHosts == 0 - this is not a task group",
				"distro_id":                     t.distroID,
				"schedulableunit_id":            unit.id,
				"schedulableunit_group":         unit.group,
				"schedulableunit_project":       unit.project,
				"schedulableunit_version":       unit.version,
				"schedulableunit_variant":       unit.variant,
				"schedulableunit_running_hosts": unit.runningHosts,
				"schedulableunit_max_hosts":     unit.maxHosts,
				"schedulableunit_num_tasks":     len(unit.tasks),
				"task_id_returned":              unit.tasks[0].Id,
			})

			// A non-task group schedulableUnit's tasks ([]TaskQueueItem) only contain a single element.
			return &unit.tasks[0]
		}

		if unit.runningHosts < unit.maxHosts {
			// TODO: For a multi-host task group, it's not possible to correctly
			// dispatch tasks based on number of hosts running tasks in the task group
			// without a transaction. When we use a driver that supports transactions,
			// we likely want to rewrite this code to use transactions. Currently it's
			// possible to dispatch a multi-host task group to more hosts than max
			// hosts. Before transactions are available, it might be possible to do this
			// better by performing this query again, just before returning from this
			// function.
			numHosts, err = host.NumHostsByTaskSpec(unit.group, unit.variant, unit.project, unit.version)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"dispatcher":                    "schedulableunit-task-dispatcher",
					"function":                      "FindNextTask",
					"message":                       "problem running NumHostsByTaskSpec query - returning nil",
					"distro_id":                     t.distroID,
					"schedulableunit_id":            unit.id,
					"schedulableunit_group":         unit.group,
					"schedulableunit_project":       unit.project,
					"schedulableunit_version":       unit.version,
					"schedulableunit_variant":       unit.variant,
					"schedulableunit_running_hosts": unit.runningHosts,
					"schedulableunit_max_hosts":     unit.maxHosts,
					"schedulableunit_num_tasks":     len(unit.tasks),
					"taskspec_group":                spec.Group,
					"taskspec_variant":              spec.BuildVariant,
					"taskspec_project":              spec.Project,
					"taskspec_version":              spec.Version,
				}))
				return nil
			}
			unit.runningHosts = numHosts
			t.units[schedulableUnitID] = unit

			if unit.runningHosts < unit.maxHosts {
				if next = t.nextTaskGroupTask(unit); next != nil {
					return next
				}
			} else {
				grip.Debug(message.Fields{
					"dispatcher":                    "schedulableunit-task-dispatcher",
					"function":                      "FindNextTask",
					"message":                       "schedulableUnit.runningHosts < schedulableUnit.maxHosts is false",
					"distro_id":                     t.distroID,
					"order_length":                  len(t.order),
					"units_length":                  len(t.units),
					"schedulableunit_id":            unit.id,
					"schedulableunit_group":         unit.group,
					"schedulableunit_project":       unit.project,
					"schedulableunit_version":       unit.version,
					"schedulableunit_variant":       unit.variant,
					"schedulableunit_running_hosts": unit.runningHosts,
					"schedulableunit_max_hosts":     unit.maxHosts,
					"schedulableunit_num_tasks":     len(unit.tasks),
					"taskspec_group":                spec.Group,
					"taskspec_build_variant":        spec.BuildVariant,
					"taskspec_project":              spec.Project,
					"taskspec_version":              spec.Version,
					"taskspec_group_max_hosts":      spec.GroupMaxHosts,
				})
			}
		}
	}

	grip.Debug(message.Fields{
		"dispatcher":               "schedulableunit-task-dispatcher",
		"function":                 "FindNextTask",
		"message":                  "no task - returning nil",
		"distro_id":                t.distroID,
		"order_length":             len(t.order),
		"units_length":             len(t.units),
		"taskspec_group":           spec.Group,
		"taskspec_build_variant":   spec.BuildVariant,
		"taskspec_version":         spec.Version,
		"taskspec_project":         spec.Project,
		"taskspec_group_max_hosts": spec.GroupMaxHosts,
	})

	return nil
}

func compositeGroupId(group, variant, project, version string) string {
	return fmt.Sprintf("%s_%s_%s_%s", group, variant, project, version)
}

func (t *basicCachedDispatcherImpl) nextTaskGroupTask(unit schedulableUnit) *TaskQueueItem {
	for i, nextTaskQueueItem := range unit.tasks {
		if nextTaskQueueItem.IsDispatched == true {
			continue
		}

		nextTaskFromDB, err := task.FindOneId(nextTaskQueueItem.Id)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem finding task in db",
				"task_id": nextTaskQueueItem.Id,
			}))
			return nil
		}
		if nextTaskFromDB == nil {
			grip.Error(message.Fields{
				"message": "task from db not found",
				"task_id": nextTaskQueueItem.Id,
			})
			return nil
		}

		if isBlockedSingleHostTaskGroup(unit, nextTaskFromDB) {
			grip.Debug(message.Fields{
				"dispatcher":                    "schedulableunit-task-dispatcher",
				"function":                      "nextTaskGroupTask",
				"message":                       "a task running in a 1-host task group, has finished, but did not succeed; deleting from t.units[unit_id]",
				"distro_id":                     t.distroID,
				"schedulableunit_id":            unit.id,
				"schedulableunit_group":         unit.group,
				"schedulableunit_project":       unit.project,
				"schedulableunit_version":       unit.version,
				"schedulableunit_variant":       unit.variant,
				"schedulableunit_running_hosts": unit.runningHosts,
				"schedulableunit_max_hosts":     unit.maxHosts,
				"schedulableunit_num_tasks":     len(unit.tasks),
			})
			delete(t.units, unit.id)
			return nil
		}

		// Cache dispatched status.
		t.units[unit.id].tasks[i].IsDispatched = true
		unit.tasks[i].IsDispatched = true

		// It's running (or already ran) on another host.
		if nextTaskFromDB.StartTime != util.ZeroTime {
			grip.Debug(message.Fields{
				"dispatcher":                    "schedulableunit-task-dispatcher",
				"function":                      "nextTaskGroupTask",
				"message":                       "nextTaskFromDB.StartTime != util.ZeroTime - this task is running or already ran on another host; let's check for a next TaskQueueItem in schedulableUnit.tasks[]",
				"task_id":                       nextTaskQueueItem.Id,
				"distro_id":                     t.distroID,
				"schedulableunit_id":            unit.id,
				"schedulableunit_group":         unit.group,
				"schedulableunit_project":       unit.project,
				"schedulableunit_version":       unit.version,
				"schedulableunit_variant":       unit.variant,
				"schedulableunit_running_hosts": unit.runningHosts,
				"schedulableunit_max_hosts":     unit.maxHosts,
				"schedulableunit_num_tasks":     len(unit.tasks),
				"schedulableunit_index_of_task": i,
			})

			continue
		}

		return &nextTaskQueueItem
	}
	// If all the tasks have been dispatched, remove the unit.
	delete(t.units, unit.id)

	return nil
}

// isBlockedSingleHostTaskGroup checks if the task is running in a 1-host task group, has finished,
// and did not succeed. But rely on EndTask to block later tasks.
func isBlockedSingleHostTaskGroup(unit schedulableUnit, dbTask *task.Task) bool {
	return unit.maxHosts == 1 && !util.IsZeroTime(dbTask.FinishTime) && dbTask.Status != evergreen.TaskSucceeded
}
