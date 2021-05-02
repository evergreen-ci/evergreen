package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type TaskQueueItemDispatcher interface {
	FindNextTask(string, TaskSpec) (*TaskQueueItem, error)
	Refresh(string) error
	RefreshFindNextTask(string, TaskSpec) (*TaskQueueItem, error)
}

type CachedDispatcher interface {
	Refresh() error
	FindNextTask(TaskSpec) *TaskQueueItem
	Type() string
	CreatedAt() time.Time
}

type taskDispatchService struct {
	cachedDispatchers map[string]CachedDispatcher
	mu                sync.RWMutex
	ttl               time.Duration
	useAliases        bool
}

func NewTaskDispatchService(ttl time.Duration) TaskQueueItemDispatcher {
	return &taskDispatchService{
		ttl:               ttl,
		cachedDispatchers: map[string]CachedDispatcher{},
	}
}

func NewTaskDispatchAliasService(ttl time.Duration) TaskQueueItemDispatcher {
	return &taskDispatchService{
		ttl:               ttl,
		useAliases:        true,
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
	d, err := distro.FindOne(distro.ById(distroID))
	if err != nil {
		return nil, errors.Wrapf(err, "database error for find() by distro id '%s'", distroID)
	}
	// If there is a "distro": *basicCachedDispatcherImpl in the cachedDispatchers map, return that.
	// Otherwise, get the "distro"'s taskQueue from the database; seed its cachedDispatcher; put that in the map and return it.
	s.mu.Lock()
	defer s.mu.Unlock()

	distroDispatchService, ok := s.cachedDispatchers[distroID]
	if ok && distroDispatchService.Type() == d.DispatcherSettings.Version {
		return distroDispatchService, nil
	}

	var taskQueue TaskQueue
	if s.useAliases {
		taskQueue, err = FindDistroAliasTaskQueue(distroID)
	} else {
		taskQueue, err = FindDistroTaskQueue(distroID)
	}

	if err != nil {
		return nil, errors.WithStack(err)
	}

	switch d.DispatcherSettings.Version {
	case evergreen.DispatcherVersionRevisedWithDependencies:
		distroDispatchService, err = newDistroTaskDAGDispatchService(taskQueue, s.ttl)
		if err != nil {
			return nil, err
		}
	default:
		distroDispatchService = newDistroTaskDispatchService(taskQueue, d.DispatcherSettings.Version, s.ttl)
	}

	s.cachedDispatchers[distroID] = distroDispatchService
	return distroDispatchService, nil
}

////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////

const (
	SchedulableUnitDispatcher = "schedulableunit-task-dispatcher"
)

// basicCachedDispatcherImpl is an in-memory representation of schedulableUnits of tasks for a distro.
//
// TODO Pass all task group tasks, not just dispatchable ones, to the constructor.
type basicCachedDispatcherImpl struct {
	mu          sync.RWMutex
	distroID    string
	order       []string
	units       map[string]schedulableUnit
	ttl         time.Duration
	typeName    string
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
func newDistroTaskDispatchService(taskQueue TaskQueue, typeName string, ttl time.Duration) *basicCachedDispatcherImpl {
	d := &basicCachedDispatcherImpl{
		distroID: taskQueue.Distro,
		ttl:      ttl,
		typeName: typeName,
	}

	if taskQueue.Length() != 0 {
		d.rebuild(taskQueue.Queue)
	}

	grip.Debug(message.Fields{
		"dispatcher":           SchedulableUnitDispatcher,
		"function":             "newDistroTaskDispatchService",
		"message":              "initializing basicCachedDispatcherImpl for a distro",
		"distro_id":            d.distroID,
		"ttl":                  d.ttl,
		"last_updated":         d.lastUpdated,
		"num_schedulableunits": len(d.units),
		"num_orders":           len(d.order),
		"num_taskqueueitems":   taskQueue.Length(),
	})

	return d
}

func (d *basicCachedDispatcherImpl) Type() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.typeName
}

func (d *basicCachedDispatcherImpl) CreatedAt() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastUpdated
}

func (d *basicCachedDispatcherImpl) Refresh() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !shouldRefreshCached(d.ttl, d.lastUpdated) {
		return nil
	}

	taskQueue, err := FindDistroTaskQueue(d.distroID)
	if err != nil {
		return errors.WithStack(err)
	}

	taskQueueItems := taskQueue.Queue
	d.rebuild(taskQueueItems)

	return nil
}

func shouldRefreshCached(ttl time.Duration, lastUpdated time.Time) bool {
	return lastUpdated.IsZero() || time.Since(lastUpdated) > ttl
}

func (d *basicCachedDispatcherImpl) rebuild(items []TaskQueueItem) {
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
			id = compositeGroupID(item.Group, item.BuildVariant, item.Project, item.Version)
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

	d.order = order
	d.units = units
	d.lastUpdated = time.Now()
}

// FindNextTask returns the next dispatchable task in the queue.
func (d *basicCachedDispatcherImpl) FindNextTask(spec TaskSpec) *TaskQueueItem {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.units) == 0 && len(d.order) > 0 {
		d.order = []string{}
		return nil
	}

	var unit schedulableUnit
	var ok bool
	var next *TaskQueueItem
	// If the host just ran a task from a task group, give it another back.
	if spec.Group != "" {
		unit, ok = d.units[compositeGroupID(spec.Group, spec.BuildVariant, spec.Project, spec.Version)]
		if ok {
			if next = d.nextTaskGroupTask(unit); next != nil {
				return next
			}
		}
		// If the task group is not present in the schedulableUnit map, then all its tasks are considered dispatched.
		// Fall through to get a task that's not in this task group.
	}

	var numHosts int
	var err error

	dependencyCaches := make(map[string]task.Task)
	for _, schedulableUnitID := range d.order {
		unit, ok = d.units[schedulableUnitID]
		if !ok {
			continue
		}

		// If maxHosts is not set, this is not a task group.
		if unit.maxHosts == 0 {
			delete(d.units, schedulableUnitID)
			if len(unit.tasks) == 0 {
				grip.Critical(message.Fields{
					"dispatcher":                    SchedulableUnitDispatcher,
					"function":                      "FindNextTask",
					"message":                       "schedulableUnit.maxHosts == 0 - so this is not a task group; but schedulableUnit.tasks is empty - returning nil",
					"distro_id":                     d.distroID,
					"schedulableunit_id":            unit.id,
					"schedulableunit_group":         unit.group,
					"schedulableunit_project":       unit.project,
					"schedulableunit_version":       unit.version,
					"schedulableunit_variant":       unit.variant,
					"schedulableunit_running_hosts": unit.runningHosts,
					"schedulableunit_max_hosts":     unit.maxHosts,
					"schedulableunit_num_tasks":     len(unit.tasks),
				})

				return nil
			}

			// A non-task group schedulableUnit's tasks ([]TaskQueueItem) only contains a single element.
			item := unit.tasks[0]
			var nextTaskFromDB *task.Task
			nextTaskFromDB, err = task.FindOneId(unit.tasks[0].Id)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"dispatcher": SchedulableUnitDispatcher,
					"function":   "FindNextTask",
					"message":    "problem finding task in db",
					"task_id":    item.Id,
					"distro_id":  d.distroID,
				}))
				return nil
			}
			if nextTaskFromDB == nil {
				grip.Warning(message.Fields{
					"dispatcher": SchedulableUnitDispatcher,
					"function":   "FindNextTask",
					"message":    "task from db not found",
					"task_id":    item.Id,
					"distro_id":  d.distroID,
				})
				return nil
			}

			var dependenciesMet bool
			dependenciesMet, err = nextTaskFromDB.DependenciesMet(dependencyCaches)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"dispatcher": SchedulableUnitDispatcher,
					"function":   "FindNextTask",
					"message":    "error checking dependencies for task",
					"outcome":    "skip and continue",
					"task":       item.Id,
					"distro_id":  d.distroID,
				}))
				continue
			}

			if !dependenciesMet {
				continue
			}

			return &unit.tasks[0]
		}

		if unit.runningHosts < unit.maxHosts {
			numHosts, err = host.NumHostsByTaskSpec(unit.group, unit.variant, unit.project, unit.version)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"dispatcher":                    SchedulableUnitDispatcher,
					"function":                      "FindNextTask",
					"message":                       "problem running NumHostsByTaskSpec query - returning nil",
					"distro_id":                     d.distroID,
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
			d.units[schedulableUnitID] = unit

			if unit.runningHosts < unit.maxHosts {
				if next = d.nextTaskGroupTask(unit); next != nil {
					return next
				}
			}
		}
	}

	return nil
}

func compositeGroupID(group, variant, project, version string) string {
	return fmt.Sprintf("%s_%s_%s_%s", group, variant, project, version)
}

func (d *basicCachedDispatcherImpl) nextTaskGroupTask(unit schedulableUnit) *TaskQueueItem {
	for i, nextTaskQueueItem := range unit.tasks {
		// Dispatch this task if all of the following are true:
		// (a) it's not marked as dispatched in the in-memory queue.
		// (b) a record of the task exists in the database.
		// (c) if it belongs to a task group bound to a single host - it's not blocked by a task within the task group that has finished, but did not succeed.
		// (d) it never previously ran on another host.
		// (e) all of its dependencies are satisfied.
		if nextTaskQueueItem.IsDispatched {
			continue
		}

		nextTaskFromDB, err := task.FindOneId(nextTaskQueueItem.Id)
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"dispatcher": SchedulableUnitDispatcher,
				"function":   "nextTaskGroupTask",
				"message":    "task from db not found",
				"task_id":    nextTaskQueueItem.Id,
				"distro_id":  d.distroID,
			}))
			return nil
		}
		if nextTaskFromDB == nil {
			grip.Warning(message.Fields{
				"dispatcher": SchedulableUnitDispatcher,
				"function":   "nextTaskGroupTask",
				"message":    "task from db not found",
				"task_id":    nextTaskQueueItem.Id,
				"distro_id":  d.distroID,
			})
			return nil
		}

		// Cache the task as dispatched from the in-memory queue's point of view.
		// However, it won't actually be dispatched to a host if it doesn't satisfy all constraints.
		d.units[unit.id].tasks[i].IsDispatched = true

		if isBlockedSingleHostTaskGroup(unit, nextTaskFromDB) {
			delete(d.units, unit.id)
			return nil
		}

		if nextTaskFromDB.StartTime != utility.ZeroTime {
			continue
		}

		dependencyCaches := make(map[string]task.Task)
		dependenciesMet, err := nextTaskFromDB.DependenciesMet(dependencyCaches)
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"dispatcher": SchedulableUnitDispatcher,
				"function":   "nextTaskGroupTask",
				"message":    "error checking dependencies for task",
				"outcome":    "skip and continue",
				"task_id":    nextTaskQueueItem.Id,
				"distro_id":  d.distroID,
			}))
			continue
		}

		if !dependenciesMet {
			continue
		}

		// If this is the last task in the schedulableUnit.tasks, delete the task group.
		if i == len(unit.tasks)-1 {
			delete(d.units, unit.id)
		}

		return &nextTaskQueueItem
	}

	return nil
}

// isBlockedSingleHostTaskGroup checks if the task is running in a 1-host task group, has finished,
// and did not succeed. But rely on EndTask to block later tasks.
func isBlockedSingleHostTaskGroup(unit schedulableUnit, dbTask *task.Task) bool {
	return unit.maxHosts == 1 && !utility.IsZeroTime(dbTask.FinishTime) && dbTask.Status != evergreen.TaskSucceeded
}
