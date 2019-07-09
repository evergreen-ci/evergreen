package model

import (
	"fmt"
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
func newDistroTaskDAGDispatchService(distroID string, items []TaskQueueItem, ttl time.Duration) (TaskDistroQueueService, error) {
	t := &taskDistroDAGDispatchService{
		distroID: distroID,
		ttl:      ttl,
	}
	t.graph = simple.NewDirectedGraph()
	t.itemNodeMap = map[string]graph.Node{}    // map[<TaskQueueItem.Id>]Node{}
	t.nodeItemMap = map[int64]*TaskQueueItem{} // map[node.ID()]*TaskQueueItem{}
	t.taskGroups = map[string]taskGroupTasks{} // map[compositeGroupId(TaskQueueItem.Group, TaskQueueItem.BuildVariant, TaskQueueItem.Version)]taskGroupTasks{}

	if len(items) != 0 {
		if err := t.rebuild(items); err != nil {
			return nil, errors.Wrapf(err, "error creating newDistroTaskDAGDispatchService for distro '%s'", distroID)
		}
	}

	return t, nil
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

func (t *taskDistroDAGDispatchService) addItem(item *TaskQueueItem) {
	node := t.graph.NewNode()
	t.graph.AddNode(node)
	t.nodeItemMap[node.ID()] = item
	t.itemNodeMap[item.Id] = node
}

func (t *taskDistroDAGDispatchService) getItemByNodeID(id int64) *TaskQueueItem {
	if item, ok := t.nodeItemMap[id]; ok {
		return item
	}

	return nil
}

func (t *taskDistroDAGDispatchService) getNodeByItemID(id string) graph.Node {
	if node, ok := t.itemNodeMap[id]; ok {
		return node
	}

	return nil
}

func (t *taskDistroDAGDispatchService) addEdge(from string, to string) error {
	fromNodeID := t.itemNodeMap[from].ID()
	toNodeID := t.itemNodeMap[to].ID()

	// Cannot add a self edge!
	if fromNodeID == toNodeID {
		// STU: what to do here?
		// grip.Errorf("cannot add a self edge from Node.ID() %d to itself", toNodeID)
		return fmt.Errorf("cannot add a self edge from Node.ID() %d to itself", toNodeID)
	}

	edge := simple.Edge{
		F: simple.Node(fromNodeID),
		T: simple.Node(toNodeID),
	}
	t.graph.SetEdge(edge)

	return nil
}

// STU: this should return an error
func (t *taskDistroDAGDispatchService) rebuild(items []TaskQueueItem) error {
	// now := time.Now()
	// STU: deferred logging can be ambiguous
	// defer func() {
	// 	grip.Info(message.Fields{
	// 		"message":       "finished rebuilding items",
	// 		"duration_secs": time.Since(now).Seconds(),
	// 	})
	// }()
	t.lastUpdated = time.Now()

	// Add each individual <TaskQueueItem> node to the graph
	for i := range items {
		t.addItem(&items[i])
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
			if err := t.addEdge(item.Id, dep); err != nil {
				return errors.Wrapf(err, "Failed to create in-memory task queue of TaskQueueItems for distro '%s'; error defining a DirectedGraph of task dependencies", t.distroID)
			}
		}
	}

	// BRIAN?

	// Sort the graph. Use a lexical sort to resolve ambiguities, because node order is the
	// order that we received these in.

	// graph       *simple.DirectedGraph
	// func lexicalReversed(nodes []graph.Node)  { sort.Sort(byIDReversed(nodes)) }

	// func SortStabilized(g graph.Directed, order func([]graph.Node)) (sorted []graph.Node, err error) {
	sorted, err := topo.SortStabilized(t.graph, lexicalReversed)
	if err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"message": "problem sorting tasks",
		}))
		// STU: what to do here?
		return errors.New("What to do here")
	}
	t.sorted = sorted

	return nil
}

type byIDReversed []graph.Node

func (n byIDReversed) Len() int           { return len(n) }
func (n byIDReversed) Less(i, j int) bool { return n[i].ID() > n[j].ID() }
func (n byIDReversed) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }
func lexicalReversed(nodes []graph.Node) {
	// if len(nodes) > 0 {
	// 	for _, n := range nodes {
	// 		// fmt.Println("len(nodes) is: " + strconv.Itoa(len(nodes)))
	// 		id := int(n.ID())
	// 		// fmt.Println("My name is: " + strconv.Itoa(id))
	// 	}
	// }
	sort.Sort(byIDReversed(nodes))
}

// My name is: 26
// My name is: 11
// My name is: 47
// My name is: 71
// My name is: 82
// My name is: 96
// My name is: 24
// My name is: 17
// My name is: 37
// My name is: 43
// My name is: 46
// My name is: 64
// My name is: 65
// My name is: 72
// My name is: 10
// My name is: 6
// My name is: 52
// My name is: 75
// My name is: 80
// My name is: 2
// My name is: 57
// My name is: 60
// My name is: 98
// My name is: 99
// My name is: 21
// My name is: 23
// My name is: 25
// My name is: 27
// My name is: 30
// My name is: 45
// My name is: 49
// My name is: 59
// My name is: 1
// My name is: 7
// My name is: 20
// My name is: 36
// My name is: 39
// My name is: 66
// My name is: 76
// My name is: 83
// My name is: 3
// My name is: 92
// My name is: 22
// My name is: 29
// My name is: 41
// My name is: 63
// My name is: 68
// My name is: 74
// My name is: 90
// My name is: 12
// My name is: 14
// My name is: 31
// My name is: 38
// My name is: 42
// My name is: 50
// My name is: 56
// My name is: 61
// My name is: 13
// My name is: 86
// My name is: 89
// My name is: 69
// My name is: 5
// My name is: 16
// My name is: 73
// My name is: 85
// My name is: 97
// My name is: 0
// My name is: 44
// My name is: 53
// My name is: 62
// My name is: 78
// My name is: 81
// My name is: 35
// My name is: 28
// My name is: 48
// My name is: 55
// My name is: 67
// My name is: 79
// My name is: 87
// My name is: 88
// My name is: 19
// My name is: 94
// My name is: 93
// My name is: 9
// My name is: 32
// My name is: 40
// My name is: 70
// My name is: 77
// My name is: 8
// My name is: 51
// My name is: 33
// My name is: 34
// My name is: 58
// My name is: 91
// My name is: 95
// My name is: 18
// My name is: 15
// My name is: 54
// My name is: 84
// My name is: 4
// My name is: 40
// My name is: 45
// My name is: 50
// My name is: 70
// My name is: 75
// My name is: 80

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

	return nil
}

// isBlockedSingleHostTaskGroup checks if the task is running in a 1-host task group, has finished,
// and did not succeed. But rely on EndTask to block later tasks.
func (t *taskDistroDAGDispatchService) isBlockedSingleHostTaskGroup(taskGroup taskGroupTasks, dbTask *task.Task) bool {
	return taskGroup.maxHosts == 1 && !util.IsZeroTime(dbTask.FinishTime) && dbTask.Status != evergreen.TaskSucceeded
}
