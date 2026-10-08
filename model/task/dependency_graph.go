package task

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/iterator"
	"gonum.org/v1/gonum/graph/topo"
	"gonum.org/v1/gonum/graph/traverse"
)

// DependencyGraph models task dependency relationships as a directed graph.
// Use NewDependencyGraph to initialize a new DependencyGraph.
type DependencyGraph struct {
	transposed bool
	graph      *compactDependencyGraph
}

// NewDependencyGraph returns an initialized DependencyGraph.
// transposed determines the direction of edges in the graph.
// If transposed is false, edges point from dependent tasks to the tasks they depend on.
// If transposed is true, edges point from depended on tasks to the tasks that depend on them.
func NewDependencyGraph(transposed bool) DependencyGraph {
	return DependencyGraph{
		transposed: transposed,
		graph: &compactDependencyGraph{
			tasksToNodes: make(map[TaskNode]compactNode),
			statuses:     make(map[edgeKey]string),
		},
	}
}

type compactNode int64

func (node compactNode) ID() int64 { return int64(node) }

type edgeKey struct {
	from compactNode
	to   compactNode
}

type compactEdge struct {
	from graph.Node
	to   graph.Node
}

func (edge compactEdge) From() graph.Node { return edge.from }

func (edge compactEdge) To() graph.Node { return edge.to }

func (edge compactEdge) ReversedEdge() graph.Edge { return compactEdge{from: edge.to, to: edge.from} }

type compactDependencyGraph struct {
	tasksToNodes map[TaskNode]compactNode
	nodesToTasks []TaskNode
	nodes        []graph.Node
	outgoing     [][]compactNode
	incoming     [][]compactNode
	statuses     map[edgeKey]string
}

var _ graph.Directed = (*compactDependencyGraph)(nil)

func (g *compactDependencyGraph) Node(id int64) graph.Node {
	if id < 0 || id >= int64(len(g.nodesToTasks)) {
		return nil
	}
	return g.nodes[id]
}

func (g *compactDependencyGraph) Nodes() graph.Nodes {
	return iterator.NewImplicitNodes(0, len(g.nodesToTasks), func(id int) graph.Node {
		return g.nodes[id]
	})
}

func (g *compactDependencyGraph) From(id int64) graph.Nodes {
	if id < 0 || id >= int64(len(g.outgoing)) {
		return graph.Empty
	}
	nodes := g.outgoing[id]
	return iterator.NewImplicitNodes(0, len(nodes), func(index int) graph.Node {
		return g.nodes[nodes[index]]
	})
}

func (g *compactDependencyGraph) To(id int64) graph.Nodes {
	if id < 0 || id >= int64(len(g.incoming)) {
		return graph.Empty
	}
	nodes := g.incoming[id]
	return iterator.NewImplicitNodes(0, len(nodes), func(index int) graph.Node {
		return g.nodes[nodes[index]]
	})
}

func (g *compactDependencyGraph) HasEdgeFromTo(from, to int64) bool {
	_, exists := g.statuses[edgeKey{from: compactNode(from), to: compactNode(to)}]
	return exists
}

func (g *compactDependencyGraph) HasEdgeBetween(from, to int64) bool {
	return g.HasEdgeFromTo(from, to) || g.HasEdgeFromTo(to, from)
}

func (g *compactDependencyGraph) Edge(from, to int64) graph.Edge {
	if !g.HasEdgeFromTo(from, to) {
		return nil
	}
	return compactEdge{from: g.nodes[from], to: g.nodes[to]}
}

func (g *compactDependencyGraph) dependencyEdge(key edgeKey) DependencyEdge {
	return DependencyEdge{
		From:   g.nodesToTasks[key.from],
		To:     g.nodesToTasks[key.to],
		Status: g.statuses[key],
	}
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
func VersionDependencyGraph(ctx context.Context, versionID string, transposed bool) (DependencyGraph, error) {
	tasks, err := FindWithFields(ctx, ByVersion(versionID), DependsOnKey, BuildVariantKey, DisplayNameKey)
	if err != nil {
		return DependencyGraph{}, errors.Wrapf(err, "getting tasks for version '%s'", versionID)
	}

	return taskDependencyGraph(tasks, transposed), nil
}

func taskDependencyGraph(tasks []Task, transposed bool) DependencyGraph {
	g := NewDependencyGraph(transposed)
	g.buildFromTasks(tasks)
	return g
}

func (g *DependencyGraph) buildFromTasks(tasks []Task) {
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
			g.AddEdge(dependentTaskNode, dependedOnTaskNode, dep.Status)
		}
	}
}

// Nodes returns a slice of all the task nodes in the graph.
func (g *DependencyGraph) Nodes() []TaskNode {
	tNodes := make([]TaskNode, len(g.graph.nodesToTasks))
	copy(tNodes, g.graph.nodesToTasks)
	return tNodes
}

// AddTaskNode adds a node to the graph.
func (g *DependencyGraph) AddTaskNode(tNode TaskNode) {
	if _, ok := g.graph.tasksToNodes[tNode]; ok {
		return
	}

	node := compactNode(len(g.graph.nodesToTasks))
	g.graph.tasksToNodes[tNode] = node
	g.graph.nodesToTasks = append(g.graph.nodesToTasks, tNode)
	g.graph.nodes = append(g.graph.nodes, node)
	g.graph.outgoing = append(g.graph.outgoing, nil)
	g.graph.incoming = append(g.graph.incoming, nil)
}

// AddEdge adds an edge between tasks in the graph.
// The edge direction is determined by whether the DependencyGraph is transposed.
// Noop if one of the nodes doesn't exist in the graph.
func (g *DependencyGraph) AddEdge(dependentTask, dependedOnTask TaskNode, status string) {
	if g.transposed {
		g.addEdgeToGraph(DependencyEdge{From: dependedOnTask, To: dependentTask, Status: status})
	} else {
		g.addEdgeToGraph(DependencyEdge{From: dependentTask, To: dependedOnTask, Status: status})
	}
}

func (g *DependencyGraph) addEdgeToGraph(edge DependencyEdge) {
	fromNode, fromExists := g.graph.tasksToNodes[edge.From]
	toNode, toExists := g.graph.tasksToNodes[edge.To]
	if !(fromExists && toExists) {
		return
	}

	key := edgeKey{from: fromNode, to: toNode}
	if _, exists := g.graph.statuses[key]; !exists {
		g.graph.outgoing[fromNode] = append(g.graph.outgoing[fromNode], toNode)
		g.graph.incoming[toNode] = append(g.graph.incoming[toNode], fromNode)
	}
	g.graph.statuses[key] = edge.Status
}

// EdgesIntoTask returns all the edges that point to t.
// For a regular graph these edges are tasks that directly depend on t.
// If the graph is transposed these edges are tasks t directly depends on.
func (g *DependencyGraph) EdgesIntoTask(t TaskNode) []DependencyEdge {
	node, exists := g.graph.tasksToNodes[t]
	if !exists || len(g.graph.incoming[node]) == 0 {
		return nil
	}

	edges := make([]DependencyEdge, 0, len(g.graph.incoming[node]))
	for _, from := range g.graph.incoming[node] {
		edges = append(edges, g.graph.dependencyEdge(edgeKey{from: from, to: node}))
	}

	return edges
}

// GetDependencyEdge returns a pointer to the edge from fromNode to toNode.
// If the edge doesn't exist it returns nil.
func (g *DependencyGraph) GetDependencyEdge(fromTask, toTask TaskNode) *DependencyEdge {
	from, fromExists := g.graph.tasksToNodes[fromTask]
	to, toExists := g.graph.tasksToNodes[toTask]
	if !fromExists || !toExists {
		return nil
	}
	key := edgeKey{from: from, to: to}
	if _, exists := g.graph.statuses[key]; !exists {
		return nil
	}
	depEdge := g.graph.dependencyEdge(key)
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
		cycles = append(cycles, fmt.Sprintf("[%s]", strings.Join(cycleStrings, ", ")))
	}

	return strings.Join(cycles, ", ")
}

// Cycles returns cycles in the graph, if any.
// Self-loops are also considered cycles.
func (g *DependencyGraph) Cycles() DependencyCycles {
	var cycles DependencyCycles
	stronglyConnectedComponents := topo.TarjanSCC(g.graph)
	for _, scc := range stronglyConnectedComponents {
		if len(scc) == 1 {
			if g.graph.HasEdgeBetween(scc[0].ID(), scc[0].ID()) {
				cycles = append(cycles, []TaskNode{g.graph.nodesToTasks[scc[0].ID()], g.graph.nodesToTasks[scc[0].ID()]})
			}
		} else {
			var cycle []TaskNode
			for _, node := range scc {
				taskInCycle := g.graph.nodesToTasks[node.ID()]
				cycle = append(cycle, taskInCycle)
			}
			cycles = append(cycles, cycle)
		}
	}

	return cycles
}

// DepthFirstSearch begins a DFS from start and returns whether target is reachable.
// If traverseEdge is not nil an edge is only traversed if traverseEdge returns true on that edge.
func (g *DependencyGraph) DepthFirstSearch(start, target TaskNode, traverseEdge func(edge DependencyEdge) bool) bool {
	startNode, startExists := g.graph.tasksToNodes[start]
	targetNode, targetExists := g.graph.tasksToNodes[target]
	if !(startExists && targetExists) {
		return false
	}

	traversal := traverse.DepthFirst{
		Traverse: func(e graph.Edge) bool {
			if traverseEdge == nil {
				return true
			}

			edge := g.graph.dependencyEdge(edgeKey{from: compactNode(e.From().ID()), to: compactNode(e.To().ID())})

			return traverseEdge(edge)
		},
	}

	return traversal.Walk(g.graph, g.graph.nodes[startNode], func(n graph.Node) bool { return n.ID() == targetNode.ID() }) != nil
}

// TopologicalStableSort sorts the nodes in the graph topologically. It is stable in the sense that when a topological ordering
// is ambiguous the order the tasks were added to the graph prevails.
// To sort with all dependent tasks before the tasks they depend on use the default graph.
// To sort with all depended on tasks before the tasks that depend on them use a transposed graph.
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
			sortedTasks = append(sortedTasks, g.graph.nodesToTasks[node.ID()])
		}
	}

	return sortedTasks, nil
}
