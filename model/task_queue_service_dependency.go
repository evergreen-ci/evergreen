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

type taskDistroDAGDispatchService struct {
	mu          sync.RWMutex
	distroID    string
	graph       *simple.DirectedGraph
	sorted      []graph.Node
	itemNodeMap map[string]graph.Node
	nodeItemMap map[int64]*TaskQueueItem
	taskGroups  map[string]taskGroupTasks
	ttl         time.Duration
	lastUpdated time.Time
}

type taskGroupTasks struct {
	id           string
	runningHosts int // number of hosts task group is currently running on
	maxHosts     int // number of hosts task group can run on
	tasks        []TaskQueueItem
}

// taskDistroDAGDispatchService creates a taskDistroDAGDispatchService from a slice of TaskQueueItems.
func newDistroTaskDAGDispatchService(distroID string, items []TaskQueueItem, ttl time.Duration) TaskDistroQueueService {
	t := &taskDistroDAGDispatchService{
		distroID: distroID,
		ttl:      ttl,
	}
	t.graph = simple.NewDirectedGraph()
	t.itemNodeMap = map[string]graph.Node{}
	t.nodeItemMap = map[int64]*TaskQueueItem{}
	t.taskGroups = map[string]taskGroupTasks{}
	if len(items) != 0 {
		t.rebuild(items)
	}

	return t
}

func (t *taskDistroDAGDispatchService) Refresh() error {
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

func (t *taskDistroDAGDispatchService) addItem(item TaskQueueItem) {
	node := t.graph.NewNode()
	t.graph.AddNode(node)
	t.nodeItemMap[node.ID()] = &item
	t.itemNodeMap[item.Id] = node
}

func (t *taskDistroDAGDispatchService) getItemByNodeID(id int64) *TaskQueueItem {
	if item, ok := t.nodeItemMap[id]; ok {
		return item
	}
	grip.Error(message.Fields{
		"message": "programmer error, couldn't find node in map",
	})
	return nil
}

func (t *taskDistroDAGDispatchService) getNodeByItemID(id string) graph.Node {
	if node, ok := t.itemNodeMap[id]; ok {
		return node
	}
	grip.Error(message.Fields{
		"message": "programmer error, couldn't find node in map",
	})
	return nil
}

func (t *taskDistroDAGDispatchService) addEdge(from string, to string) {
	edge := simple.Edge{
		F: simple.Node(t.itemNodeMap[from].ID()),
		T: simple.Node(t.itemNodeMap[to].ID()),
	}
	t.graph.SetEdge(edge)
}

func (t *taskDistroDAGDispatchService) rebuild(items []TaskQueueItem) {
	now := time.Now()
	defer func() {
		grip.Info(message.Fields{
			"message":       "finished rebuilding items",
			"duration_secs": time.Since(now).Seconds(),
		})
	}()
	t.lastUpdated = time.Now()

	// Add items to the graph
	for _, item := range items {
		t.addItem(item)
	}

	// Save task groups
	t.taskGroups = map[string]taskGroupTasks{}
	for _, item := range items {
		if item.Group != "" {
			id := compositeGroupId(item.Group, item.BuildVariant, item.Version)
			if _, ok := t.taskGroups[id]; !ok {
				t.taskGroups[id] = taskGroupTasks{
					id:       id,
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
		for _, dep := range item.Dependencies {
			t.addEdge(item.Id, dep)
		}
	}

	// Sort the graph. Use a lexical sort to resolve ambiguities, because node order is the
	// order that we received these in.
	sorted, err := topo.SortStabilized(t.graph, lexicalReversed)
	if err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"message": "problem sorting tasks",
		}))
		return
	}
	t.sorted = sorted

	return
}

type byIDReversed []graph.Node

func (n byIDReversed) Len() int           { return len(n) }
func (n byIDReversed) Less(i, j int) bool { return n[i].ID() > n[j].ID() }
func (n byIDReversed) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }
func lexicalReversed(nodes []graph.Node)  { sort.Sort(byIDReversed(nodes)) }

// FindNextTask returns the next dispatchable task in the queue.
func (t *taskDistroDAGDispatchService) FindNextTask(spec TaskSpec) *TaskQueueItem {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If the host just ran a task group, give it one back
	if spec.Group != "" {
		taskGroup, ok := t.taskGroups[compositeGroupId(spec.Group, spec.BuildVariant, spec.Version)]
		if ok {
			if next := t.nextTaskGroupTask(taskGroup); next != nil {
				node := t.getNodeByItemID(next.Id)
				item := t.getItemByNodeID(node.ID())
				item.IsDispatched = true
				return next
			}
		}
		// If the task group is not present in the task group map, it has been dispatched,
		// so fall through to getting a task not in that group.
	}

	// Iterate backwards through the graph, because dependencies are on the left.
	dependencyCaches := make(map[string]task.Task)
	for i := len(t.sorted) - 1; i >= 0; i-- {
		node := t.sorted[i]
		item := t.getItemByNodeID(node.ID())

		// TODO Consider checking if the state of any task has changed, which could unblock
		// later tasks in the queue. Currently we just wait for the scheduler to rerun.

		// For a task not in a task group, dispatch it if it hasn't been dispatched, and if
		// its dependencies have been met
		if item.GroupMaxHosts == 0 {
			if item.IsDispatched {
				continue
			}
			nextTaskFromDB, err := task.FindOneId(item.Id)
			depsMet, err := nextTaskFromDB.DependenciesMet(dependencyCaches)
			if err != nil {
				grip.Warning(message.Fields{
					"scheduler": "task-queue-service-dependency",
					"message":   "error checking dependencies for task",
					"outcome":   "skipping",
					"task":      item.Id,
					"error":     err.Error(),
				})
				continue
			}
			if !depsMet {
				continue
			}
			item.IsDispatched = true
			return item
		}

		// For a task group task, do some arithmetic to see if the group is dispatchable.
		// Get the next task in the group.
		taskGroupID := compositeGroupId(item.Group, item.BuildVariant, item.Version)
		taskGroup, ok := t.taskGroups[taskGroupID]
		if !ok {
			continue
		}
		if taskGroup.runningHosts < taskGroup.maxHosts {
			numHosts, err := host.NumHostsByTaskSpec(spec.Group, spec.BuildVariant, spec.ProjectID, spec.Version)
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

func (t *taskDistroDAGDispatchService) nextTaskGroupTask(taskGroup taskGroupTasks) *TaskQueueItem {
	for i, nextTask := range taskGroup.tasks {
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

		if t.isBlockedSingleHostTaskGroup(taskGroup, nextTaskFromDB) {
			delete(t.taskGroups, taskGroup.id)
			return nil
		}

		// Cache dispatched status.
		t.taskGroups[taskGroup.id].tasks[i].IsDispatched = true
		taskGroup.tasks[i].IsDispatched = true

		if nextTaskFromDB.StartTime != util.ZeroTime {
			continue
		}
		// Don't cache dispatched status when returning the next TaskQueueItem - in case the task fails to start.
		// If this is the last task in the group, delete the task group.
		if i == len(taskGroup.tasks)-1 {
			delete(t.taskGroups, taskGroup.id)
		}
		return &nextTask
	}
	grip.Alert(message.Fields{
		"message": "programmer error, no dispatchable tasks in group",
	})
	return nil
}

// isBlockedSingleHostTaskGroup checks if the task is running in a 1-host task group, has finished,
// and did not succeed. But rely on EndTask to block later tasks.
func (t *taskDistroDAGDispatchService) isBlockedSingleHostTaskGroup(taskGroup taskGroupTasks, dbTask *task.Task) bool {
	return taskGroup.maxHosts == 1 && !util.IsZeroTime(dbTask.FinishTime) && dbTask.Status != evergreen.TaskSucceeded
}
