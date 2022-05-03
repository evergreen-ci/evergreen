package task

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/multi"
	"gonum.org/v1/gonum/graph/topo"
	"gonum.org/v1/gonum/graph/traverse"
)

// DependencyGraph models task dependency relationships as a directed graph.
// Use NewDependencyGraph() to initialize a new DependencyGraph.
type DependencyGraph struct {
	graph               *multi.DirectedGraph
	tasksToNodes        map[TaskNode]graph.Node
	nodesToTasks        map[graph.Node]TaskNode
	edgesToDependencies map[edgeKey]DependencyEdge
}

// NewDependencyGraph returns an initialized DependencyGraph.
func NewDependencyGraph() DependencyGraph {
	return DependencyGraph{
		graph:               multi.NewDirectedGraph(),
		tasksToNodes:        make(map[TaskNode]graph.Node),
		nodesToTasks:        make(map[graph.Node]TaskNode),
		edgesToDependencies: make(map[edgeKey]DependencyEdge),
	}
}

type edgeKey struct {
	from TaskNode
	to   TaskNode
}

// DependencyEdge is a representation of a dependency in the graph.
type DependencyEdge struct {
	// Status is the status specified by the dependency, if any.
	Status string
	// From is the node the edge begins from.
	From TaskNode
	// To is the node the edge points to.
	To TaskNode
}

// TaskNode is the representation of a task in the graph.
type TaskNode struct {
	// Name is the display name of the task.
	Name string
	// Variant is the build variant of the task.
	Variant string
	// ID is the task's ID.
	ID string
}

// String represents TaskNode as a string.
func (t TaskNode) String() string {
	if t.ID != "" {
		return t.ID
	}

	return fmt.Sprintf("%s/%s", t.Variant, t.Name)
}

// VersionDependencyGraph finds all the tasks from the version given by versionID and constructs a DependencyGraph from them.
func VersionDependencyGraph(versionID string) (DependencyGraph, error) {
	tasks, err := FindAllTasksFromVersionWithDependencies(versionID)
	if err != nil {
		return DependencyGraph{}, errors.Wrapf(err, "getting tasks for version '%s'", versionID)
	}

	return taskDependencyGraph(tasks), nil
}

func taskDependencyGraph(tasks []Task) DependencyGraph {
	g := NewDependencyGraph()
	g.buildFromTasks(tasks, false)
	return g
}

// reversedTaskDependencyGraph constructs a DependencyGraph from tasks.
// Edges in the graph are reversed so they point from depended on tasks to dependent tasks.
func reversedTaskDependencyGraph(tasks []Task) DependencyGraph {
	g := NewDependencyGraph()
	g.buildFromTasks(tasks, true)
	return g
}

func (g *DependencyGraph) buildFromTasks(tasks []Task, reversed bool) {
	taskIDToNode := make(map[string]TaskNode)
	for _, task := range tasks {
		tNode := task.ToTaskNode()
		g.AddTaskNode(tNode)
		taskIDToNode[task.Id] = tNode
	}

	for _, task := range tasks {
		dependentTaskNode := task.ToTaskNode()
		for _, dep := range task.DependsOn {
			dependedOnTaskNode := taskIDToNode[dep.TaskId]
			if reversed {
				g.AddReversedEdge(dependentTaskNode, dependedOnTaskNode, dep.Status)
			} else {
				g.AddEdge(dependentTaskNode, dependedOnTaskNode, dep.Status)
			}
		}
	}
}

// AddTaskNode adds a node to the graph.
func (g *DependencyGraph) AddTaskNode(tNode TaskNode) {
	if _, ok := g.tasksToNodes[tNode]; ok {
		return
	}

	node := g.graph.NewNode()
	g.graph.AddNode(node)
	g.tasksToNodes[tNode] = node
	g.nodesToTasks[node] = tNode
}

// AddEdge adds an edge from the dependent task to the depended on task.
// Noop if one of the nodes doesn't exist in the graph.
func (g *DependencyGraph) AddEdge(dependentTask, dependedOnTask TaskNode, status string) {
	g.addEdgeToGraph(DependencyEdge{From: dependentTask, To: dependedOnTask, Status: status})
}

// AddReversedEdge adds an edge from the depended on task to the dependent task.
// Noops if one of the nodes doesn't exist in the graph.
func (g *DependencyGraph) AddReversedEdge(dependentTask, dependedOnTask TaskNode, status string) {
	g.addEdgeToGraph(DependencyEdge{From: dependedOnTask, To: dependentTask, Status: status})
}

func (g *DependencyGraph) addEdgeToGraph(edge DependencyEdge) {
	fromNode, fromExists := g.tasksToNodes[edge.From]
	toNode, toExists := g.tasksToNodes[edge.To]
	if !(fromExists && toExists) {
		return
	}

	line := g.graph.NewLine(fromNode, toNode)
	g.graph.SetLine(line)
	g.edgesToDependencies[edgeKey{from: edge.From, to: edge.To}] = edge
}

// EdgesIntoTask returns all the edges that point to t.
// For a regular graph these edges are tasks that directly depend on t.
// If the graph is reversed these edges are tasks t directly depends on.
func (g *DependencyGraph) EdgesIntoTask(t TaskNode) []DependencyEdge {
	node := g.tasksToNodes[t]
	if node == nil {
		return nil
	}

	var edges []DependencyEdge
	nodes := g.graph.To(node.ID())
	for nodes.Next() {
		edges = append(edges, g.edgesToDependencies[edgeKey{from: g.nodesToTasks[nodes.Node()], to: t}])
	}

	return edges
}

// GetDependencyEdge returns a pointer to the edge from fromNode to toNode.
// If the edge doesn't exist it returns nil.
func (g *DependencyGraph) GetDependencyEdge(fromTask, toTask TaskNode) *DependencyEdge {
	depEdge, ok := g.edgesToDependencies[edgeKey{from: fromTask, to: toTask}]
	if !ok {
		return nil
	}

	return &depEdge
}

// DependencyCycles is a jagged array of node cycles.
type DependencyCycles [][]TaskNode

// String represents DependencyCycles as a string.
func (dc DependencyCycles) String() string {
	cycles := make([]string, 0, len(dc))
	for _, cycle := range dc {
		cycleStrings := make([]string, 0, len(cycle))
		for _, node := range cycle {
			cycleStrings = append(cycleStrings, node.String())
		}
		cycles = append(cycles, "["+strings.Join(cycleStrings, ", ")+"]")
	}

	return strings.Join(cycles, ", ")
}

// Cycles returns cycles in the graph, if any.
// Self-loops are not included as cycles.
func (g *DependencyGraph) Cycles() DependencyCycles {
	var cycles DependencyCycles
	stronglyConnectedComponenets := topo.TarjanSCC(g.graph)
	for _, scc := range stronglyConnectedComponenets {
		if len(scc) <= 1 {
			continue
		}

		var cycle []TaskNode
		for _, node := range scc {
			taskInCycle := g.nodesToTasks[node]
			cycle = append(cycle, taskInCycle)
		}
		cycles = append(cycles, cycle)
	}

	return cycles
}

// DepthFirstSearch begins a DFS from start and returns whether target is reachable.
// If traverseEdge is not nil an edge is only traversed if traverseEdge returns true on that edge.
func (g *DependencyGraph) DepthFirstSearch(start, target TaskNode, traverseEdge func(edge DependencyEdge) bool) bool {
	_, startExists := g.tasksToNodes[start]
	_, targetExists := g.tasksToNodes[target]
	if !(startExists && targetExists) {
		return false
	}

	traversal := traverse.DepthFirst{
		Traverse: func(e graph.Edge) bool {
			if traverseEdge == nil {
				return true
			}

			from := g.nodesToTasks[e.From()]
			to := g.nodesToTasks[e.To()]
			edge := g.edgesToDependencies[edgeKey{from: from, to: to}]

			return traverseEdge(edge)
		},
	}

	return traversal.Walk(g.graph, g.tasksToNodes[start], func(n graph.Node) bool { return g.nodesToTasks[n] == target }) != nil
}

// TopologicalStableSort sorts the nodes in the graph topologically. It is stable in the sense that when a topological ordering
// is ambiguous the order the tasks were added to the graph prevails.
// To sort with all dependent tasks before the tasks they depend on use the default graph.
// To sort with all depended on tasks before the tasks that depend on them use a reversed graph.
func (g *DependencyGraph) TopologicalStableSort() ([]TaskNode, error) {
	sortedNodes, err := topo.SortStabilized(g.graph, nil)

	if err != nil {
		_, ok := err.(topo.Unorderable)
		if !ok {
			return nil, errors.Wrap(err, "sorting the graph")
		}
	}

	sortedTasks := make([]TaskNode, 0, len(sortedNodes))
	for _, node := range sortedNodes {
		if node != nil {
			sortedTasks = append(sortedTasks, g.nodesToTasks[node])
		}
	}

	return sortedTasks, nil
}

// reachableFromNode returns all the dependencies recursively depended on by start.
// In the case of a reversed graph it returns all the dependencies recursively depending on start.
// The start node is not included in the result.
func (g *DependencyGraph) reachableFromNode(start TaskNode) []TaskNode {
	var reachable []TaskNode
	traversal := traverse.DepthFirst{
		Visit: func(node graph.Node) {
			if g.nodesToTasks[node] != start {
				reachable = append(reachable, g.nodesToTasks[node])
			}
		},
	}
	_ = traversal.Walk(g.graph, g.tasksToNodes[start], nil)

	return reachable
}
