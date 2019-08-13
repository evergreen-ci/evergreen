package model

import (
	"fmt"
	"sort"
	"sync"
	"time"

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
	t.itemNodeMap = map[string]graph.Node{}     // map[TaskQueueItem.Id]Node{}
	t.nodeItemMap = map[int64]*TaskQueueItem{}  // map[node.ID()]*TaskQueueItem{}
	t.taskGroups = map[string]schedulableUnit{} // map[compositeGroupId(TaskQueueItem.Group, TaskQueueItem.BuildVariant, TaskQueueItem.Project, TaskQueueItem.Version)]schedulableUnit{}
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
		"initial_num_taskqueueitems": taskQueueItems,
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

func (t *basicCachedDAGDispatcherImpl) addEdge(from string, to string) error {
	fromNodeID := t.itemNodeMap[from].ID()
	toNodeID := t.itemNodeMap[to].ID()

	// Cannot add a self edge!
	if fromNodeID == toNodeID {
		grip.Alert(message.Fields{
			"dispatcher": "dependency-task-dispatcher",
			"function":   "addEdge",
			"message":    "cannot add a self edge to a Node",
			"task_id":    from,
			"node_id":    fromNodeID,
		})

		return errors.New(fmt.Sprintf("cannot add a self edge to task '%s'", from))
	}

	edge := simple.Edge{
		F: simple.Node(fromNodeID),
		T: simple.Node(toNodeID),
	}
	t.graph.SetEdge(edge)

	return nil
}

func (t *basicCachedDAGDispatcherImpl) rebuild(items []TaskQueueItem) error {
	for i := range items {
		// Add each individual <TaskQueueItem> node to the graph
		t.addItem(&items[i])
	}

	// Save the task groups
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

	// Add edges for task dependencies
	for _, item := range items {
		for _, dependency := range item.Dependencies {
			if err := t.addEdge(item.Id, dependency); err != nil {
				return errors.Wrapf(err, "failed to create in-memory task queue of TaskQueueItems for distro '%s'; error defining a DirectedGraph incorporating task dependencies", t.distroID)
			}
		}
	}

	// Sort the graph. Use a lexical sort to resolve ambiguities, because node order is the order that we received these in.
	sorted, err := topo.SortStabilized(t.graph, lexicalReversed)
	if err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"dispatcher": "dependency-task-dispatcher",
			"function":   "rebuild",
			"message":    "problem sorting tasks within the DirectedGraph",
		}))

		return errors.Wrapf(err, "failed to create in-memory task queue of TaskQueueItems for distro '%s'; error sorting a DirectedGraph incorporating task dependencies", t.distroID)
	}

	t.sorted = sorted
	t.lastUpdated = time.Now()

	return nil
}

type byIDReversed []graph.Node

func (n byIDReversed) Len() int           { return len(n) }
func (n byIDReversed) Less(i, j int) bool { return n[i].ID() > n[j].ID() }
func (n byIDReversed) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }
func lexicalReversed(nodes []graph.Node) {
	sort.Sort(byIDReversed(nodes))
}

// FindNextTask returns the next dispatchable task in the queue.
func (t *basicCachedDAGDispatcherImpl) FindNextTask(spec TaskSpec) *TaskQueueItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	// If the host just ran a task group, give it one back.
	if spec.Group != "" {
		taskGroup, ok := t.taskGroups[compositeGroupId(spec.Group, spec.BuildVariant, spec.Project, spec.Version)]
		if ok {
			if next := t.nextTaskGroupTask(taskGroup); next != nil {
				node := t.getNodeByItemID(next.Id)
				item := t.getItemByNodeID(node.ID())
				item.IsDispatched = true
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

	// Iterate backwards through the graph, because dependencies are on the left.
	dependencyCaches := make(map[string]task.Task)
	for i := len(t.sorted) - 1; i >= 0; i-- {
		node := t.sorted[i]
		item := t.getItemByNodeID(node.ID())
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
					"message": "problem finding task in db",
					"task_id": item.Id,
				}))
				return nil
			}
			if nextTaskFromDB == nil {
				grip.Error(message.Fields{
					"message": "task from db not found",
					"task_id": item.Id,
				})
				return nil
			}

			depsMet, err := nextTaskFromDB.DependenciesMet(dependencyCaches)
			if err != nil {
				grip.Warning(message.Fields{
					"dispatcher": "dependency-task-dispatcher",
					"function":   "FindNextTask",
					"message":    "error checking dependencies for task",
					"outcome":    "skip and continue",
					"task":       item.Id,
					"error":      err.Error(),
				})
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
		taskGroup, ok := t.taskGroups[taskGroupID]
		if !ok {
			continue
		}

		if taskGroup.runningHosts < taskGroup.maxHosts {
			numHosts, err := host.NumHostsByTaskSpec(item.Group, item.BuildVariant, item.Project, item.Version)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"dispatcher": "dependency-task-dispatcher",
					"message":    "problem running NumHostsByTaskSpec query - returning nil",
					"group":      item.Group,
					"variant":    item.BuildVariant,
					"project":    item.Project,
					"version":    item.Version,
				}))
				return nil
			}

			taskGroup.runningHosts = numHosts
			t.taskGroups[taskGroupID] = taskGroup
			if taskGroup.runningHosts < taskGroup.maxHosts {
				if next := t.nextTaskGroupTask(taskGroup); next != nil {
					item.IsDispatched = true
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

		if isBlockedSingleHostTaskGroup(unit, nextTaskFromDB) {
			delete(t.taskGroups, unit.id)
			return nil
		}

		// Cache dispatched status.
		t.taskGroups[unit.id].tasks[i].IsDispatched = true
		unit.tasks[i].IsDispatched = true

		if nextTaskFromDB.StartTime != util.ZeroTime {
			continue
		}
		// Don't cache dispatched status when returning the next TaskQueueItem - in case the task fails to start.
		// If this is the last task in the group, delete the task group.
		if i == len(unit.tasks)-1 {
			delete(t.taskGroups, unit.id)
		}

		return &nextTask
	}

	return nil
}
