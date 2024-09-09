package model

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/multi"
	"gonum.org/v1/gonum/graph/topo"
)

const (
	DAGDispatcher = "DAG-task-dispatcher"
)

type basicCachedDAGDispatcherImpl struct {
	mu          sync.RWMutex
	distroID    string
	graph       *multi.DirectedGraph
	sorted      []graph.Node
	itemNodeMap map[string]graph.Node      // map[TaskQueueItem.Id]Node
	nodeItemMap map[int64]*TaskQueueItem   // map[node.ID()]*TaskQueueItem
	taskGroups  map[string]schedulableUnit // map[compositeGroupID(TaskQueueItem.Group, TaskQueueItem.BuildVariant, TaskQueueItem.Project, TaskQueueItem.Version)]schedulableUnit
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

// newDistroTaskDAGDispatchService creates a basicCachedDAGDispatcherImpl from a slice of TaskQueueItems.
func newDistroTaskDAGDispatchService(taskQueue TaskQueue, ttl time.Duration) (*basicCachedDAGDispatcherImpl, error) {
	d := &basicCachedDAGDispatcherImpl{
		distroID: taskQueue.Distro,
		ttl:      ttl,
	}

	if taskQueue.Length() != 0 {
		if err := d.rebuild(taskQueue.Queue); err != nil {
			return nil, errors.Wrapf(err, "creating distro DAG task dispatch service for distro '%s'", taskQueue.Distro)
		}
	}

	return d, nil
}

func (d *basicCachedDAGDispatcherImpl) Type() string {
	return evergreen.DispatcherVersionRevisedWithDependencies
}

func (d *basicCachedDAGDispatcherImpl) Refresh() error {
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
	if err := d.rebuild(taskQueueItems); err != nil {
		return errors.Wrapf(err, "building the directed graph for distro '%s'", d.distroID)
	}

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
func (d *basicCachedDAGDispatcherImpl) addEdge(fromID string, toID string) error {
	fromNode := d.getNodeByItemID(fromID)
	toNode := d.getNodeByItemID(toID)

	// The "depend_on" <from> task is not in the DAG so we don't need an edge.
	if fromNode == nil {
		return nil
	}

	// A Node for the "dependent" <to> task is not present in the DAG.
	if toNode == nil {
		grip.Warning(message.Fields{
			"dispatcher":         DAGDispatcher,
			"function":           "addEdge",
			"message":            "a Node for a dependent taskQueueItem is not present in the DAG",
			"depends_on_task_id": fromID,
			"dependent_task_id":  toID,
			"distro_id":          d.distroID,
		})

		return errors.Errorf("a node for the dependent task queue item '%s' is not present in the DAG for distro '%s'", toID, d.distroID)
	}

	line := multi.Line{
		F: multi.Node(fromNode.ID()),
		T: multi.Node(toNode.ID()),
	}
	d.graph.SetLine(line)

	return nil
}

func (d *basicCachedDAGDispatcherImpl) rebuild(items []TaskQueueItem) error {
	d.graph = multi.NewDirectedGraph()
	d.sorted = []graph.Node{}
	d.itemNodeMap = map[string]graph.Node{}     // map[TaskQueueItem.Id]Node
	d.nodeItemMap = map[int64]*TaskQueueItem{}  // map[node.ID()]*TaskQueueItem
	d.taskGroups = map[string]schedulableUnit{} // map[compositeGroupID(TaskQueueItem.Group, TaskQueueItem.BuildVariant, TaskQueueItem.Project, TaskQueueItem.Version)]schedulableUnit

	for i := range items {
		// Add each individual <TaskQueueItem> node to the graph.
		d.addItem(&items[i])
	}

	// Save the task groups.
	for _, item := range items {
		if item.Group != "" {
			// If it's the first time encountering the task group create an entry for it in the taskGroups map.
			// Otherwise, append to the taskQueueItem array in the map.
			id := compositeGroupID(item.Group, item.BuildVariant, item.Project, item.Version)
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

	for _, item := range items {
		for _, dependency := range item.Dependencies {
			// addEdge(A, B) means that B depends on A.
			if err := d.addEdge(dependency, item.Id); err != nil {
				return errors.Wrap(err, "adding edge")
			}
		}
	}

	sorted, err := topo.SortStabilized(d.graph, nil)
	if err != nil {
		unorderableNodes, ok := err.(topo.Unorderable)
		if !ok {
			grip.Alert(message.WrapError(err, message.Fields{
				"dispatcher":                 DAGDispatcher,
				"function":                   "rebuild",
				"message":                    "problem ordering the tasks and associated dependencies within the DirectedGraph",
				"distro_id":                  d.distroID,
				"initial_num_taskqueueitems": len(items),
				"num_task_groups":            len(d.taskGroups),
			}))

			return errors.Wrap(err, "topologically sorting the dependency graph")
		}

		cycles := make([][]string, 0, len(unorderableNodes))
		for _, cycle := range unorderableNodes {
			cycleIDs := make([]string, 0, len(cycle))
			for _, node := range cycle {
				cycleIDs = append(cycleIDs, d.nodeItemMap[node.ID()].Id)
			}
			cycles = append(cycles, cycleIDs)
		}
		grip.Error(message.Fields{
			"dispatcher": DAGDispatcher,
			"function":   "rebuild",
			"message":    "tasks in the queue form dependency cycle(s)",
			"cycles":     cycles,
			"distro_id":  d.distroID,
		})
	}

	d.sorted = sorted
	d.lastUpdated = time.Now()

	return nil
}

// FindNextTask returns the next dispatchable task in the queue, and returns the tasks that need to be checked for dependencies.
func (d *basicCachedDAGDispatcherImpl) FindNextTask(ctx context.Context, spec TaskSpec, amiUpdatedTime time.Time) *TaskQueueItem {
	d.mu.Lock()
	defer d.mu.Unlock()
	// If the host just ran a task group, give it one back.
	if spec.Group != "" {
		taskGroupID := compositeGroupID(spec.Group, spec.BuildVariant, spec.Project, spec.Version)
		taskGroupUnit, ok := d.taskGroups[taskGroupID] // schedulableUnit
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
		// If the task group is not present in the TaskGroups map, then all its tasks are considered dispatched.
		// Fall through to get a task that's not in this task group.
	}

	dependencyCaches := make(map[string]task.Task)
	for i := range d.sorted {
		node := d.sorted[i]
		// topo.SortStabilized represents nodes in a dependency cycle with a nil Node.
		if node == nil {
			continue
		}

		item := d.getItemByNodeID(node.ID()) // item is a *TaskQueueItem sourced from d.nodeItemMap, which is a map[node.ID()]*TaskQueueItem.
		if item == nil {
			continue
		}

		// TODO Consider checking if the state of any task has changed, which could unblock later tasks in the queue.
		// Currently, we just wait for the dispatcher's in-memory queue to refresh.

		// If maxHosts is not set, this is not a task group.
		if item.GroupMaxHosts == 0 {
			// Dispatch this standalone task if all of the following are true:
			// (a) it's not marked as dispatched in the in-memory queue.
			// (b) a record of the task exists in the database.
			// (c) it never previously ran on another host.
			// (d) all of its dependencies are satisfied.

			if item.IsDispatched {
				continue
			}

			nextTaskFromDB, err := task.FindOneId(item.Id)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"dispatcher": DAGDispatcher,
					"function":   "FindNextTask",
					"message":    "problem finding task in db",
					"task_id":    item.Id,
					"distro_id":  d.distroID,
				}))
				return nil
			}
			if nextTaskFromDB == nil {
				grip.Warning(message.Fields{
					"dispatcher": DAGDispatcher,
					"function":   "FindNextTask",
					"message":    "task from db not found",
					"task_id":    item.Id,
					"distro_id":  d.distroID,
				})
				return nil
			}

			// Cache the task as dispatched from the in-memory queue's point of view.
			// However, it won't actually be dispatched to a host if it doesn't satisfy all constraints.
			item.IsDispatched = true // *TaskQueueItem

			if !utility.IsZeroTime(nextTaskFromDB.StartTime) {
				continue
			}

			// Skip the task if it's estimated to create more tasks than the generate task limit.
			settings, err := evergreen.GetConfig(ctx)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"dispatcher": DAGDispatcher,
					"function":   "FindNextTask",
					"message":    "problem getting evergreen settings",
					"task_id":    item.Id,
					"distro_id":  d.distroID,
				}))
				continue
			}
			generateTasksLimit := settings.TaskLimits.MaxPendingGeneratedTasks
			tasksToGenerate := utility.FromIntPtr(nextTaskFromDB.EstimatedNumGeneratedTasks)
			if generateTasksLimit > 0 && tasksToGenerate > 0 {
				pendingGenerateTasks, err := task.GetPendingGenerateTasks(ctx)
				if err != nil {
					grip.Warning(message.WrapError(err, message.Fields{
						"dispatcher": DAGDispatcher,
						"function":   "FindNextTask",
						"message":    "problem getting pending generate tasks",
						"task_id":    item.Id,
						"distro_id":  d.distroID,
					}))
					continue
				}
				if pendingGenerateTasks+tasksToGenerate >= generateTasksLimit {
					grip.Info(message.Fields{
						"dispatcher":             DAGDispatcher,
						"function":               "FindNextTask",
						"message":                "skipping task because it would exceed the generate task limit",
						"task_id":                item.Id,
						"distro_id":              d.distroID,
						"generate_task_limit":    generateTasksLimit,
						"pending_generate_tasks": pendingGenerateTasks,
						"tasks_to_generate":      tasksToGenerate,
					})
					continue
				}
			}

			dependenciesMet, err := nextTaskFromDB.DependenciesMet(dependencyCaches)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"dispatcher": DAGDispatcher,
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

			// AMI Updated time is only provided if the host is running with an outdated AMI.
			// If the task was created after the time that the AMI was updated, then we should wait for an updated host.
			if !utility.IsZeroTime(amiUpdatedTime) && nextTaskFromDB.IngestTime.After(amiUpdatedTime) {
				grip.Debug(message.Fields{
					"dispatcher":       DAGDispatcher,
					"function":         "FindNextTask",
					"message":          "skipping because AMI is outdated",
					"task_id":          nextTaskFromDB.Id,
					"distro_id":        d.distroID,
					"ami_updated_time": amiUpdatedTime,
					"ingest_time":      nextTaskFromDB.IngestTime,
				})
				continue
			}
			return item
		}

		// For a task group task, do some arithmetic to see if the group's next task is dispatchable.
		taskGroupID := compositeGroupID(item.Group, item.BuildVariant, item.Project, item.Version)
		taskGroupUnit, ok := d.taskGroups[taskGroupID]
		if !ok {
			continue
		}

		if taskGroupUnit.runningHosts < taskGroupUnit.maxHosts {
			numHosts, err := host.NumHostsByTaskSpec(ctx, item.Group, item.BuildVariant, item.Project, item.Version)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"dispatcher": DAGDispatcher,
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
	for i, nextTaskQueueItem := range unit.tasks {
		// Dispatch this task if all of the following are true:
		// (a) it's not marked as dispatched in the in-memory queue.
		// (b) a record of the task exists in the database.
		// (c) if it belongs to a TaskGroup bound to a single host - it's not blocked by a previous task within the TaskGroup that failed.
		// (d) it never previously ran on another host.
		// (e) all of its dependencies are satisfied.

		if nextTaskQueueItem.IsDispatched {
			continue
		}

		nextTaskFromDB, err := task.FindOneId(nextTaskQueueItem.Id)
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"dispatcher": DAGDispatcher,
				"function":   "nextTaskGroupTask",
				"message":    "problem finding task in db",
				"task_id":    nextTaskQueueItem.Id,
				"distro_id":  d.distroID,
			}))
			return nil
		}
		if nextTaskFromDB == nil {
			grip.Warning(message.Fields{
				"dispatcher": DAGDispatcher,
				"function":   "nextTaskGroupTask",
				"message":    "task from db not found",
				"task_id":    nextTaskQueueItem.Id,
				"distro_id":  d.distroID,
			})
			return nil
		}

		// Cache the task as dispatched from the in-memory queue's point of view.
		// However, it won't actually be dispatched to a host if it doesn't satisfy all constraints.
		d.taskGroups[unit.id].tasks[i].IsDispatched = true
		// unit.tasks[i].IsDispatched = true

		if isBlockedSingleHostTaskGroup(unit, nextTaskFromDB) {
			delete(d.taskGroups, unit.id)
			return nil
		}

		if nextTaskFromDB.StartTime != utility.ZeroTime {
			continue
		}

		dependencyCaches := make(map[string]task.Task)
		dependenciesMet, err := nextTaskFromDB.DependenciesMet(dependencyCaches)
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"dispatcher": DAGDispatcher,
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
			delete(d.taskGroups, unit.id)
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

func compositeGroupID(group, variant, project, version string) string {
	return fmt.Sprintf("%s_%s_%s_%s", group, variant, project, version)
}

func shouldRefreshCached(ttl time.Duration, lastUpdated time.Time) bool {
	return lastUpdated.IsZero() || time.Since(lastUpdated) > ttl
}
