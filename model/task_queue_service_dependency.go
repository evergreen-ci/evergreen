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
	t := &basicCachedDAGDispatcherImpl{
		distroID: taskQueue.Distro,
		ttl:      ttl,
	}
	t.graph = simple.NewDirectedGraph()
	t.itemNodeMap = map[string]graph.Node{}     // map[TaskQueueItem.Id]Node
	t.nodeItemMap = map[int64]*TaskQueueItem{}  // map[node.ID()]*TaskQueueItem
	t.taskGroups = map[string]schedulableUnit{} // map[compositeGroupId(TaskQueueItem.Group, TaskQueueItem.BuildVariant, TaskQueueItem.Project, TaskQueueItem.Version)]schedulableUnit
	if taskQueue.Length() != 0 {
		if err := t.rebuild(taskQueue.Queue); err != nil {
			return nil, errors.Wrapf(err, "error creating newDistroTaskDAGDispatchService for distro '%s'", taskQueue.Distro)
		}
	}

	grip.Debug(message.Fields{
		"dispatcher":                 "dependency-task-dispatcher",
		"function":                   "newDistroTaskDAGDispatchService",
		"message":                    "initializing new basicCachedDAGDispatcherImpl for a distro",
		"distro_id":                  t.distroID,
		"ttl":                        t.ttl,
		"last_updated":               t.lastUpdated,
		"num_task_groups":            len(t.taskGroups),
		"initial_num_taskqueueitems": taskQueue.Length(),
		"sorted_num_taskqueueitems":  len(t.sorted),
	})

	return t, nil
}

func (t *basicCachedDAGDispatcherImpl) Refresh() error {
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
	if err := t.rebuild(taskQueueItems); err != nil {
		return errors.Wrapf(err, "error defining the DirectedGraph for distro '%s'", t.distroID)
	}

	grip.Debug(message.Fields{
		"dispatcher":                 "dependency-task-dispatcher",
		"function":                   "Refresh",
		"message":                    "refresh was successful",
		"distro_id":                  t.distroID,
		"num_task_groups":            len(t.taskGroups),
		"initial_num_taskqueueitems": len(taskQueueItems),
		"sorted_num_taskqueueitems":  len(t.sorted),
	})

	return nil
}

func (t *basicCachedDAGDispatcherImpl) addItem(item *TaskQueueItem) {
	node := t.graph.NewNode()
	t.graph.AddNode(node)
	t.nodeItemMap[node.ID()] = item
	t.itemNodeMap[item.Id] = node
}

func (t *basicCachedDAGDispatcherImpl) getItemByNodeID(id int64) *TaskQueueItem {
	if item, ok := t.nodeItemMap[id]; ok {
		return item
	}

	return nil
}

func (t *basicCachedDAGDispatcherImpl) getNodeByItemID(id string) graph.Node {
	if node, ok := t.itemNodeMap[id]; ok {
		return node
	}

	return nil
}

// Each node is a task and each edge definition represents a dependency: an edge (A, B) means that B depends on A.
// Or, in other words, there is a dependency from A to B.
func (t *basicCachedDAGDispatcherImpl) addEdge(from string, to string, dependsOnCache map[string]struct{}) error {
	fromNode := t.getNodeByItemID(from)
	toNode := t.getNodeByItemID(to)
	if fromNode == nil {
		////////////////////////////////////////////////////////////////////////////
		// Scenario:
		// (1) Task A completes successful and some tasks that depend_on it are still enqueued behind it.
		// (2) Some time passes (determined by a ttl value) and a new task_queue is created.
		// (3) As Task A completed successfully, it won't be in the new task_queue.  However, other taskQueueItems in the latest task_queue may depend_on it.
		// (4) A Node for Task A doesn't exists in the DAG so we cannot add an edge from it.
		// (5) So, go to the cache and/or database to check if Task A actually exists. If it does - what is its status?
		// (6) It's redundant to addEdge(A, B) if task A is "status: "success" -- the dependency is already satisfied.
		////////////////////////////////////////////////////////////////////////////
		if _, ok := dependsOnCache[from]; ok {
			return nil
		}

		dependsOnTask, err := task.FindOneId(from)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"dispatcher": "dependency-task-dispatcher",
				"function":   "addEdge",
				"message":    "problem finding task in db",
				"task_id":    from,
				"distro_id":  t.distroID,
			}))

			return errors.Wrapf(err, "error adding edge from '%s' to '%s'", from, to)
		}
		if dependsOnTask == nil {
			grip.Error(message.Fields{
				"dispatcher": "dependency-task-dispatcher",
				"function":   "addEdge",
				"message":    "task from db not found",
				"task_id":    from,
				"distro_id":  t.distroID,
			})

			return errors.Errorf("error adding edge from '%s' to '%s' - task '%s' does not exist in the database", from, to, from)
		}

		if dependsOnTask.Status != evergreen.TaskSucceeded {
			grip.Error(message.Fields{
				"dispatcher": "dependency-task-dispatcher",
				"function":   "addEdge",
				"message":    "a Node for the given task is not present in the DAG and its status is not evergreen.TaskSucceeded in the database",
				"task_id":    from,
				"distro_id":  t.distroID,
			})

			return errors.Errorf("a Node for the task '%s' is not present in the DAG for distro '%s' and its status is not evergreen.TaskSucceeded in the database", from, t.distroID)
		}

		dependsOnCache[from] = struct{}{}

		return nil
	}

	if toNode == nil {
		return errors.Errorf("a Node for taskQueueItem '%s' is not present in the DAG for distro '%s'", to, t.distroID)
	}

	// Cannot add a self edge!
	if fromNode.ID() == toNode.ID() {
		grip.Alert(message.Fields{
			"dispatcher": "dependency-task-dispatcher",
			"function":   "addEdge",
			"message":    "cannot add a self edge to a Node",
			"task_id":    from,
			"node_id":    fromNode.ID(),
			"distro_id":  t.distroID,
		})

		return errors.Errorf("cannot add a self edge to task '%s'", from)
	}

	edge := simple.Edge{
		F: simple.Node(fromNode.ID()),
		T: simple.Node(toNode.ID()),
	}
	t.graph.SetEdge(edge)

	return nil
}

func (t *basicCachedDAGDispatcherImpl) rebuild(items []TaskQueueItem) error {
	for i := range items {
		// Add each individual <TaskQueueItem> node to the graph.
		t.addItem(&items[i])
	}

	// Save the task groups.
	t.taskGroups = map[string]schedulableUnit{}
	for _, item := range items {
		if item.Group != "" {
			// If it's the first time encountering the task group create an entry for it in the taskGroups map.
			// Otherwise, append to the taskQueueItem array in the map.
			id := compositeGroupId(item.Group, item.BuildVariant, item.Project, item.Version)
			if _, ok := t.taskGroups[id]; !ok {
				t.taskGroups[id] = schedulableUnit{
					id:       id,
					group:    item.Group,
					project:  item.Project,
					version:  item.Version,
					variant:  item.BuildVariant,
					maxHosts: item.GroupMaxHosts,
					tasks:    []TaskQueueItem{item},
				}
			} else {
				taskGroup := t.taskGroups[id]
				taskGroup.tasks = append(taskGroup.tasks, item)
				t.taskGroups[id] = taskGroup
			}
		}
	}

	// Reorder the schedulableUnit.tasks by taskQueueItem.GroupIndex.
	// For a single host task group (MaxHosts: 1) this ensures that its tasks are dispatched in the desired order.
	for _, su := range t.taskGroups {
		sort.SliceStable(su.tasks, func(i, j int) bool { return su.tasks[i].GroupIndex < su.tasks[j].GroupIndex })
	}

	dependsOnCache := make(map[string]struct{})
	for _, item := range items {
		for _, dependency := range item.Dependencies {
			// addEdge(A, B) means that B depends on A.
			if err := t.addEdge(dependency, item.Id, dependsOnCache); err != nil {
				return errors.Wrapf(err, "failed to create in-memory task queue of TaskQueueItems for distro '%s'; error defining a DirectedGraph incorporating task dependencies", t.distroID)
			}
		}
	}

	sorted, err := topo.SortStabilized(t.graph, nil)
	if err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"dispatcher": "dependency-task-dispatcher",
			"function":   "rebuild",
			"message":    "problem ordering the tasks and associated dependencies within the DirectedGraph",
			"distro_id":  t.distroID,
		}))

		return errors.Wrapf(err, "failed to create in-memory task queue of TaskQueueItems for distro '%s'; error ordering a DirectedGraph incorporating task dependencies", t.distroID)
	}

	t.sorted = sorted
	t.lastUpdated = time.Now()

	return nil
}

// FindNextTask returns the next dispatchable task in the queue.
func (t *basicCachedDAGDispatcherImpl) FindNextTask(spec TaskSpec) *TaskQueueItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	// If the host just ran a task group, give it one back.
	if spec.Group != "" {
		taskGroupID := compositeGroupId(spec.Group, spec.BuildVariant, spec.Project, spec.Version)
		taskGroupUnit, ok := t.taskGroups[taskGroupID] // taskGroupUnit is a schedulableUnit.
		if ok {
			if next := t.nextTaskGroupTask(taskGroupUnit); next != nil {
				// next is a *TaskQueueItem, sourced for t.taskGroups (map[string]schedulableUnit) tasks' field, which in turn is a []TaskQueueItem.
				// taskGroupTask is a *TaskQueueItem sourced from t.nodeItemMap, which is a map[node.ID()]*TaskQueueItem.
				node := t.getNodeByItemID(next.Id)
				taskGroupTask := t.getItemByNodeID(node.ID())
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
			"distro_id":                t.distroID,
		})
	}
	dependencyCaches := make(map[string]task.Task)
	for i := range t.sorted {
		node := t.sorted[i]
		item := t.getItemByNodeID(node.ID()) // item is a *TaskQueueItem sourced from t.nodeItemMap, which is a map[node.ID()]*TaskQueueItem.

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
					"distro_id":  t.distroID,
				}))
				return nil
			}
			if nextTaskFromDB == nil {
				grip.Error(message.Fields{
					"dispatcher": "dependency-task-dispatcher",
					"function":   "FindNextTask",
					"message":    "task from db not found",
					"task_id":    item.Id,
					"distro_id":  t.distroID,
				})
				return nil
			}

			depsMet, err := nextTaskFromDB.DependenciesMet(dependencyCaches)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"dispatcher": "dependency-task-dispatcher",
					"function":   "FindNextTask",
					"message":    "error checking dependencies for task",
					"outcome":    "skip and continue",
					"task":       item.Id,
					"distro_id":  t.distroID,
				}))
				continue
			}

			if !depsMet {
				continue
			}

			item.IsDispatched = true

			return item
		}

		// For a task group task, do some arithmetic to see if the group's next task is dispatchable.
		taskGroupID := compositeGroupId(item.Group, item.BuildVariant, item.Project, item.Version)
		taskGroupUnit, ok := t.taskGroups[compositeGroupId(item.Group, item.BuildVariant, item.Project, item.Version)]
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
					"distro_id":  t.distroID,
				}))
				return nil
			}

			taskGroupUnit.runningHosts = numHosts
			t.taskGroups[taskGroupID] = taskGroupUnit
			if taskGroupUnit.runningHosts < taskGroupUnit.maxHosts {
				if next := t.nextTaskGroupTask(taskGroupUnit); next != nil {
					node := t.getNodeByItemID(next.Id)
					taskGroupTask := t.getItemByNodeID(node.ID()) // *TaskQueueItem
					taskGroupTask.IsDispatched = true

					return next
				}
			}
		}
	}

	return nil
}

func (t *basicCachedDAGDispatcherImpl) nextTaskGroupTask(unit schedulableUnit) *TaskQueueItem {
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
				"distro_id":  t.distroID,
			}))
			return nil
		}
		if nextTaskFromDB == nil {
			grip.Error(message.Fields{
				"dispatcher": "dependency-task-dispatcher",
				"function":   "nextTaskGroupTask",
				"message":    "task from db not found",
				"task":       nextTask.Id,
				"distro_id":  t.distroID,
			})
			return nil
		}

		// Check if its dependencies have been met.
		dependencyCaches := make(map[string]task.Task)
		depsMet, err := nextTaskFromDB.DependenciesMet(dependencyCaches)
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"dispatcher": "dependency-task-dispatcher",
				"function":   "nextTaskGroupTask",
				"message":    "error checking dependencies for task",
				"outcome":    "skip and continue",
				"task":       nextTask.Id,
				"distro_id":  t.distroID,
			}))
			continue
		}

		if !depsMet {
			// Regardless, set IsDispatch = true for this *TaskQueueItem, while awaiting the next refresh of the in-memory queue.
			t.taskGroups[unit.id].tasks[i].IsDispatched = true
			// unit.tasks[i].IsDispatched = true
			continue
		}

		if isBlockedSingleHostTaskGroup(unit, nextTaskFromDB) {
			delete(t.taskGroups, unit.id)
			return nil
		}

		// Cache dispatched status.
		t.taskGroups[unit.id].tasks[i].IsDispatched = true
		// unit.tasks[i].IsDispatched = true

		if nextTaskFromDB.StartTime != util.ZeroTime {
			continue
		}

		// If this is the last task in the group, delete the task group.
		if i == len(unit.tasks)-1 {
			delete(t.taskGroups, unit.id)
		}

		return &nextTask
	}

	return nil
}
