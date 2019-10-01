package model

import (
	"sort"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
)

type basicCachedDAGDispatcherImpl struct {
	mu          sync.RWMutex
	distroID    string
	graph       *simple.DirectedGraph
	sorted      []graph.Node
	itemNodeMap map[string]graph.Node
	nodeItemMap map[int64]*TaskQueueItem
	taskGroups  map[string]schedulableUnit
	ttl         time.Duration
	lastUpdated time.Time
}

// newDistroTaskDAGDispatchService creates a basicCachedDAGDispatcherImpl from a slice of TaskQueueItems.
func newDistroTaskDAGDispatchService(taskQueue TaskQueue, ttl time.Duration) (*basicCachedDAGDispatcherImpl, error) {
	d := &basicCachedDAGDispatcherImpl{
		distroID: taskQueue.Distro,
		ttl:      ttl,
	}
	d.graph = simple.NewDirectedGraph()
	d.itemNodeMap = map[string]graph.Node{}     // map[TaskQueueItem.Id]Node
	d.nodeItemMap = map[int64]*TaskQueueItem{}  // map[node.ID()]*TaskQueueItem
	d.taskGroups = map[string]schedulableUnit{} // map[compositeGroupId(TaskQueueItem.Group, TaskQueueItem.BuildVariant, TaskQueueItem.Project, TaskQueueItem.Version)]schedulableUnit
	if taskQueue.Length() != 0 {
		if err := d.rebuild(taskQueue.Queue); err != nil {
			return nil, errors.Wrapf(err, "error creating newDistroTaskDAGDispatchService for distro '%s'", taskQueue.Distro)
		}
	}

	grip.Debug(message.Fields{
		"dispatcher":                 "dependency-task-dispatcher",
		"function":                   "newDistroTaskDAGDispatchService",
		"message":                    "initializing new basicCachedDAGDispatcherImpl for a distro",
		"distro_id":                  d.distroID,
		"ttl":                        d.ttl,
		"last_updated":               d.lastUpdated,
		"num_task_groups":            len(d.taskGroups),
		"initial_num_taskqueueitems": taskQueue.Length(),
		"sorted_num_taskqueueitems":  len(d.sorted),
	})

	return d, nil
}

func (d *basicCachedDAGDispatcherImpl) Refresh() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !shouldRefreshCached(d.ttl, d.lastUpdated, d.distroID) {
		return nil
	}

	taskQueue, err := FindDistroTaskQueue(d.distroID)
	if err != nil {
		return errors.WithStack(err)
	}

	taskQueueItems := taskQueue.Queue
	if err := d.rebuild(taskQueueItems); err != nil {
		return errors.Wrapf(err, "error defining the DirectedGraph for distro '%s'", d.distroID)
	}

	grip.Debug(message.Fields{
		"dispatcher":                 "dependency-task-dispatcher",
		"function":                   "Refresh",
		"message":                    "refresh was successful",
		"distro_id":                  d.distroID,
		"num_task_groups":            len(d.taskGroups),
		"initial_num_taskqueueitems": len(taskQueueItems),
		"sorted_num_taskqueueitems":  len(d.sorted),
	})

	return nil
}

func (d *basicCachedDAGDispatcherImpl) addItem(item *TaskQueueItem) {
	node := d.graph.NewNode()
	d.graph.AddNode(node)
	d.nodeItemMap[node.ID()] = item
	d.itemNodeMap[item.Id] = node
}

func (d *basicCachedDAGDispatcherImpl) getItemByNodeID(id int64) *TaskQueueItem {
	if item, ok := d.nodeItemMap[id]; ok {
		return item
	}

	return nil
}

func (d *basicCachedDAGDispatcherImpl) getNodeByItemID(id string) graph.Node {
	if node, ok := d.itemNodeMap[id]; ok {
		return node
	}

	return nil
}

// Each node is a task and each edge definition represents a dependency: an edge (A, B) means that B depends on A.
// There is a dependency <from> A <to> B.
func (d *basicCachedDAGDispatcherImpl) addEdge(fromID string, toID string, dependsOnCache map[string]struct{}) error {
	fromNode := d.getNodeByItemID(fromID)
	toNode := d.getNodeByItemID(toID)

	// Scenario:
	//
	// (1) Task A completes successful and some tasks that depend_on it are still enqueued behind it.
	// (2) Some time passes (determined by a ttl value) and a new task_queue is created.
	// (3) As Task A completed successfully, it won't be in the new task_queue.  However, other taskQueueItems in the latest task_queue may depend_on it.
	// (4) A Node for Task A doesn't exists in the DAG so we cannot add an edge from it.
	// (5) Go to the cache or database and check if Task A is there?
	// (6) If it is: has it been satisfied or overridden for Task A, making it redundant to addEdge(<from>, <to>)?

	// A Node for the depends_on <from> task to be satisfied is not present in the DAG - why?
	if fromNode == nil {
		if _, ok := dependsOnCache[fromID]; ok {
			return nil
		}

		// Get the "dependent" <to> task from the database.
		toTask, err := task.FindOneId(toID)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"dispatcher": "dependency-task-dispatcher",
				"function":   "addEdge",
				"message":    "problem finding task in db",
				"task_id":    toID,
				"distro_id":  d.distroID,
			}))

			return errors.Wrapf(err, "error adding edge from '%s' to '%s' - database problem while finding task '%s'", fromID, toID, toID)
		}
		if toTask == nil {
			grip.Error(message.Fields{
				"dispatcher": "dependency-task-dispatcher",
				"function":   "addEdge",
				"message":    "task from db not found",
				"task_id":    toID,
				"distro_id":  d.distroID,
			})

			return errors.Errorf("error adding edge from '%s' to '%s' - task '%s' does not exist in the database", fromID, toID, toID)
		}
		// Is the "dependent" <to> task explicitly ignoring its "depends_on" <from> task?
		if toTask.OverrideDependencies {
			return nil
		}

		// Get the "depends_on" <from> task to be satisfied from the database.
		fromTask, err := task.FindOneId(fromID)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"dispatcher": "dependency-task-dispatcher",
				"function":   "addEdge",
				"message":    "problem finding task in db",
				"task_id":    fromID,
				"distro_id":  d.distroID,
			}))

			return errors.Wrapf(err, "error adding edge from '%s' to '%s' - database problem while finding task '%s'", fromID, toID, fromID)
		}
		if fromTask == nil {
			grip.Error(message.Fields{
				"dispatcher": "dependency-task-dispatcher",
				"function":   "addEdge",
				"message":    "task from db not found",
				"task_id":    fromID,
				"distro_id":  d.distroID,
			})

			return errors.Errorf("error adding edge from '%s' to '%s' - task '%s' does not exist in the database", fromID, toID, fromID)
		}

		if fromTask.Status == evergreen.TaskSucceeded {
			dependsOnCache[fromID] = struct{}{}
			return nil
		}

		// Has the "depends_on" <from> task for the "dependent" <to> task been satisfied?  If it hasn't, there is a problem.
		if !toTask.SatisfiesDependency(fromTask) {
			grip.Error(message.Fields{
				"dispatcher":   "dependency-task-dispatcher",
				"function":     "addEdge",
				"message":      "the depends_on/from task has not be satisfied",
				"from_task_id": fromID,
				"to_task_id":   toID,
				"distro_id":    d.distroID,
			})

			return errors.Errorf("error adding edge from '%s' to '%s'; a Node for taskQueueItem '%s' is not present in the DAG for distro '%s' and it's a depends_on task that has not been satisfied", fromID, toID, fromID, d.distroID)
		}

		return nil
	}

	// A Node for the "dependent" <to> task is not present in the DAG.
	if toNode == nil {
		return errors.Errorf("a Node for taskQueueItem '%s' is not present in the DAG for distro '%s'", toID, d.distroID)
	}

	// Cannot add a self edge within the DAG!
	if fromNode.ID() == toNode.ID() {
		grip.Alert(message.Fields{
			"dispatcher": "dependency-task-dispatcher",
			"function":   "addEdge",
			"message":    "cannot add a self edge to a Node",
			"task_id":    fromID,
			"node_id":    fromNode.ID(),
			"distro_id":  d.distroID,
		})

		return errors.Errorf("cannot add a self edge to task '%s'", fromID)
	}

	edge := simple.Edge{
		F: simple.Node(fromNode.ID()),
		T: simple.Node(toNode.ID()),
	}
	d.graph.SetEdge(edge)

	return nil
}

func (d *basicCachedDAGDispatcherImpl) rebuild(items []TaskQueueItem) error {
	for i := range items {
		// Add each individual <TaskQueueItem> node to the graph.
		d.addItem(&items[i])
	}

	// Save the task groups.
	d.taskGroups = map[string]schedulableUnit{}
	for _, item := range items {
		if item.Group != "" {
			// If it's the first time encountering the task group create an entry for it in the taskGroups map.
			// Otherwise, append to the taskQueueItem array in the map.
			id := compositeGroupId(item.Group, item.BuildVariant, item.Project, item.Version)
			if _, ok := d.taskGroups[id]; !ok {
				d.taskGroups[id] = schedulableUnit{
					id:       id,
					group:    item.Group,
					project:  item.Project,
					version:  item.Version,
					variant:  item.BuildVariant,
					maxHosts: item.GroupMaxHosts,
					tasks:    []TaskQueueItem{item},
				}
			} else {
				taskGroup := d.taskGroups[id]
				taskGroup.tasks = append(taskGroup.tasks, item)
				d.taskGroups[id] = taskGroup
			}
		}
	}

	// Reorder the schedulableUnit.tasks by taskQueueItem.GroupIndex.
	// For a single host task group (MaxHosts: 1) this ensures that its tasks are dispatched in the desired order.
	for _, su := range d.taskGroups {
		sort.SliceStable(su.tasks, func(i, j int) bool { return su.tasks[i].GroupIndex < su.tasks[j].GroupIndex })
	}

	dependsOnCache := make(map[string]struct{})
	for _, item := range items {
		for _, dependency := range item.Dependencies {
			// addEdge(A, B) means that B depends on A.
			if err := d.addEdge(dependency, item.Id, dependsOnCache); err != nil {
				return errors.Wrapf(err, "failed to create in-memory task queue of TaskQueueItems for distro '%s'; error defining a DirectedGraph incorporating task dependencies", d.distroID)
			}
		}
	}

	sorted, err := topo.SortStabilized(d.graph, nil)
	if err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"dispatcher": "dependency-task-dispatcher",
			"function":   "rebuild",
			"message":    "problem ordering the tasks and associated dependencies within the DirectedGraph",
			"distro_id":  d.distroID,
		}))

		return errors.Wrapf(err, "failed to create in-memory task queue of TaskQueueItems for distro '%s'; error ordering a DirectedGraph incorporating task dependencies", d.distroID)
	}

	d.sorted = sorted
	d.lastUpdated = time.Now()

	return nil
}

// FindNextTask returns the next dispatchable task in the queue.
func (d *basicCachedDAGDispatcherImpl) FindNextTask(spec TaskSpec) *TaskQueueItem {
	d.mu.Lock()
	defer d.mu.Unlock()
	// If the host just ran a task group, give it one back.
	if spec.Group != "" {
		taskGroupID := compositeGroupId(spec.Group, spec.BuildVariant, spec.Project, spec.Version)
		taskGroupUnit, ok := d.taskGroups[taskGroupID] // taskGroupUnit is a schedulableUnit.
		if ok {
			if next := d.nextTaskGroupTask(taskGroupUnit); next != nil {
				// next is a *TaskQueueItem, sourced for d.taskGroups (map[string]schedulableUnit) tasks' field, which in turn is a []TaskQueueItem.
				// taskGroupTask is a *TaskQueueItem sourced from d.nodeItemMap, which is a map[node.ID()]*TaskQueueItem.
				node := d.getNodeByItemID(next.Id)
				taskGroupTask := d.getItemByNodeID(node.ID())
				taskGroupTask.IsDispatched = true

				return next
			}
		}
		// If the task group is not present in the task group map, it has been dispatched.
		// Fall through to get a task that's not in that task group.
		grip.Debug(message.Fields{
			"dispatcher":               "dependency-task-dispatcher",
			"function":                 "FindNextTask",
			"message":                  "basicCachedDAGDispatcherImpl.taskGroupTasks[key] was not found - assuming it has been dispatched; falling through to try and get a task not in the current task group",
			"key":                      compositeGroupId(spec.Group, spec.BuildVariant, spec.Project, spec.Version),
			"taskspec_group":           spec.Group,
			"taskspec_build_variant":   spec.BuildVariant,
			"taskspec_version":         spec.Version,
			"taskspec_project":         spec.Project,
			"taskspec_group_max_hosts": spec.GroupMaxHosts,
			"distro_id":                d.distroID,
		})
	}
	dependencyCaches := make(map[string]task.Task)
	for i := range d.sorted {
		node := d.sorted[i]
		item := d.getItemByNodeID(node.ID()) // item is a *TaskQueueItem sourced from d.nodeItemMap, which is a map[node.ID()]*TaskQueueItem.

		// TODO Consider checking if the state of any task has changed, which could unblock it.
		// later tasks in the queue. Currently we just wait for the dispatcher to rerun.

		// If maxHosts is not set, this is not a task group.
		if item.GroupMaxHosts == 0 {
			// Dispatch this standalone task if all of the following are true:
			// (a) it hasn't already been dispatched.
			// (b) a record of the task exists in the database.
			// (c) its dependencies have been met.

			if item.IsDispatched {
				continue
			}

			nextTaskFromDB, err := task.FindOneId(item.Id)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"dispatcher": "dependency-task-dispatcher",
					"function":   "FindNextTask",
					"message":    "problem finding task in db",
					"task_id":    item.Id,
					"distro_id":  d.distroID,
				}))
				return nil
			}
			if nextTaskFromDB == nil {
				grip.Error(message.Fields{
					"dispatcher": "dependency-task-dispatcher",
					"function":   "FindNextTask",
					"message":    "task from db not found",
					"task_id":    item.Id,
					"distro_id":  d.distroID,
				})
				return nil
			}

			dependenciesMet, err := nextTaskFromDB.DependenciesMet(dependencyCaches)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"dispatcher": "dependency-task-dispatcher",
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

			item.IsDispatched = true

			return item
		}

		// For a task group task, do some arithmetic to see if the group's next task is dispatchable.
		taskGroupID := compositeGroupId(item.Group, item.BuildVariant, item.Project, item.Version)
		taskGroupUnit, ok := d.taskGroups[compositeGroupId(item.Group, item.BuildVariant, item.Project, item.Version)]
		if !ok {
			continue
		}

		if taskGroupUnit.runningHosts < taskGroupUnit.maxHosts {
			numHosts, err := host.NumHostsByTaskSpec(item.Group, item.BuildVariant, item.Project, item.Version)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"dispatcher": "dependency-task-dispatcher",
					"function":   "FindNextTask",
					"message":    "problem running NumHostsByTaskSpec query - returning nil",
					"group":      item.Group,
					"variant":    item.BuildVariant,
					"project":    item.Project,
					"version":    item.Version,
					"distro_id":  d.distroID,
				}))
				return nil
			}

			taskGroupUnit.runningHosts = numHosts
			d.taskGroups[taskGroupID] = taskGroupUnit
			if taskGroupUnit.runningHosts < taskGroupUnit.maxHosts {
				if next := d.nextTaskGroupTask(taskGroupUnit); next != nil {
					node := d.getNodeByItemID(next.Id)
					taskGroupTask := d.getItemByNodeID(node.ID()) // *TaskQueueItem
					taskGroupTask.IsDispatched = true

					return next
				}
			}
		}
	}

	return nil
}

func (d *basicCachedDAGDispatcherImpl) nextTaskGroupTask(unit schedulableUnit) *TaskQueueItem {
	for i, nextTask := range unit.tasks {
		if nextTask.IsDispatched == true {
			continue
		}

		nextTaskFromDB, err := task.FindOneId(nextTask.Id)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"dispatcher": "dependency-task-dispatcher",
				"function":   "nextTaskGroupTask",
				"message":    "problem finding task in db",
				"task":       nextTask.Id,
				"distro_id":  d.distroID,
			}))
			return nil
		}
		if nextTaskFromDB == nil {
			grip.Error(message.Fields{
				"dispatcher": "dependency-task-dispatcher",
				"function":   "nextTaskGroupTask",
				"message":    "task from db not found",
				"task":       nextTask.Id,
				"distro_id":  d.distroID,
			})
			return nil
		}

		// Check if its dependencies have been met.
		dependencyCaches := make(map[string]task.Task)
		dependenciesMet, err := nextTaskFromDB.DependenciesMet(dependencyCaches)
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"dispatcher": "dependency-task-dispatcher",
				"function":   "nextTaskGroupTask",
				"message":    "error checking dependencies for task",
				"outcome":    "skip and continue",
				"task":       nextTask.Id,
				"distro_id":  d.distroID,
			}))
			continue
		}

		if !dependenciesMet {
			// Regardless, set IsDispatch = true for this *TaskQueueItem, while awaiting the next refresh of the in-memory queue.
			d.taskGroups[unit.id].tasks[i].IsDispatched = true
			continue
		}

		if isBlockedSingleHostTaskGroup(unit, nextTaskFromDB) {
			delete(d.taskGroups, unit.id)
			return nil
		}

		// Cache dispatched status.
		d.taskGroups[unit.id].tasks[i].IsDispatched = true

		if nextTaskFromDB.StartTime != util.ZeroTime {
			continue
		}

		// If this is the last task in the group, delete the task group.
		if i == len(unit.tasks)-1 {
			delete(d.taskGroups, unit.id)
		}

		return &nextTask
	}

	return nil
}
