package task

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/multi"
	"gonum.org/v1/gonum/graph/topo"
	"gonum.org/v1/gonum/graph/traverse"
)

func TestTaskNodeString(t *testing.T) {
	assert.Equal(t, "t0", TaskNode{ID: "t0", Name: "task0", Variant: "BV0"}.String())
	assert.Equal(t, "BV0/task0", TaskNode{Name: "task0", Variant: "BV0"}.String())
}

func TestBuildFromTasks(t *testing.T) {
	tasks := []Task{
		{Id: "t0", DependsOn: []Dependency{{TaskId: "t1", Status: evergreen.TaskSucceeded}}},
		{Id: "t1", DependsOn: []Dependency{{TaskId: "t2"}, {TaskId: "t3"}}},
		{Id: "t2"},
		{Id: "t3"},
	}
	for _, testCase := range []struct {
		name       string
		transposed bool
	}{
		{name: "ForwardEdges"},
		{name: "ReversedEdges", transposed: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dependencyGraph := NewDependencyGraph(testCase.transposed)
			dependencyGraph.buildFromTasks(tasks)
			assert.Len(t, dependencyGraph.Nodes(), len(tasks))
			assert.Len(t, dependencyGraph.graph.statuses, 3)
			for _, task := range tasks {
				node := task.ToTaskNode()
				require.Contains(t, dependencyGraph.graph.tasksToNodes, node)
				assert.Equal(t, node, dependencyGraph.graph.nodesToTasks[dependencyGraph.graph.tasksToNodes[node]])
				expectedIncoming, expectedOutgoing := 1, len(task.DependsOn)
				if task.Id == "t0" {
					expectedIncoming = 0
				}
				if testCase.transposed {
					expectedIncoming, expectedOutgoing = expectedOutgoing, expectedIncoming
				}
				assert.Len(t, dependencyGraph.EdgesIntoTask(node), expectedIncoming)
				assert.Equal(t, expectedOutgoing, dependencyGraph.graph.From(dependencyGraph.graph.tasksToNodes[node].ID()).Len())
				for _, dependency := range task.DependsOn {
					from, to := node, TaskNode{ID: dependency.TaskId}
					if testCase.transposed {
						from, to = to, from
					}
					assert.Equal(t, &DependencyEdge{From: from, To: to, Status: dependency.Status}, dependencyGraph.GetDependencyEdge(from, to))
				}
			}
		})
	}
}

func TestAddTaskNode(t *testing.T) {
	tasks := []Task{
		{Id: "t0"},
	}

	t.Run("NewNode", func(t *testing.T) {
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)
		assert.Len(t, g.graph.nodesToTasks, 1)
		assert.Len(t, g.graph.tasksToNodes, 1)
		assert.Equal(t, 1, g.graph.Nodes().Len())

		g.AddTaskNode(TaskNode{ID: "t1"})
		assert.Len(t, g.graph.nodesToTasks, 2)
		assert.Len(t, g.graph.tasksToNodes, 2)
		assert.Equal(t, 2, g.graph.Nodes().Len())
	})

	t.Run("PreexistingNode", func(t *testing.T) {
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)
		assert.Len(t, g.graph.nodesToTasks, 1)
		assert.Len(t, g.graph.tasksToNodes, 1)
		assert.Equal(t, 1, g.graph.Nodes().Len())

		g.AddTaskNode(TaskNode{ID: "t0"})
		assert.Len(t, g.graph.nodesToTasks, 1)
		assert.Len(t, g.graph.tasksToNodes, 1)
		assert.Equal(t, 1, g.graph.Nodes().Len())
	})
}

func TestAddEdgeToGraph(t *testing.T) {
	tasks := []Task{
		{Id: "t0"},
		{Id: "t1", DependsOn: []Dependency{{TaskId: "t2"}}},
		{Id: "t2"},
	}

	t.Run("NewEdge", func(t *testing.T) {
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)
		assert.Len(t, g.graph.statuses, 1)

		g.addEdgeToGraph(DependencyEdge{From: tasks[0].ToTaskNode(), To: tasks[1].ToTaskNode()})
		assert.NotNil(t, g.GetDependencyEdge(tasks[0].ToTaskNode(), tasks[1].ToTaskNode()))
		assert.Len(t, g.graph.statuses, 2)
	})

	t.Run("PreexistingEdge", func(t *testing.T) {
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)
		assert.Len(t, g.graph.statuses, 1)

		g.addEdgeToGraph(DependencyEdge{From: tasks[1].ToTaskNode(), To: tasks[2].ToTaskNode()})
		assert.Len(t, g.graph.statuses, 1)
		assert.Len(t, g.graph.outgoing[g.graph.tasksToNodes[tasks[1].ToTaskNode()]], 1)
		assert.Len(t, g.graph.incoming[g.graph.tasksToNodes[tasks[2].ToTaskNode()]], 1)
	})

	t.Run("EdgeToMissingNode", func(t *testing.T) {
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)
		assert.Len(t, g.graph.statuses, 1)

		g.addEdgeToGraph(DependencyEdge{From: tasks[0].ToTaskNode(), To: TaskNode{ID: "t3"}})
		assert.Len(t, g.graph.statuses, 1)
	})

	t.Run("Cyclic", func(t *testing.T) {
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)
		assert.Len(t, g.graph.statuses, 1)

		g.addEdgeToGraph(DependencyEdge{From: tasks[0].ToTaskNode(), To: tasks[1].ToTaskNode()})
		g.addEdgeToGraph(DependencyEdge{From: tasks[1].ToTaskNode(), To: tasks[0].ToTaskNode()})
		assert.NotNil(t, g.GetDependencyEdge(tasks[0].ToTaskNode(), tasks[1].ToTaskNode()))
		assert.NotNil(t, g.GetDependencyEdge(tasks[1].ToTaskNode(), tasks[0].ToTaskNode()))
		assert.Len(t, g.graph.statuses, 3)
	})
}

func TestRepeatedDependencyEdgesPreserveStatusAndGraphBehavior(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		transposed bool
		selfLoop   bool
	}{
		{name: "NormalGraph"},
		{name: "TransposedGraph", transposed: true},
		{name: "SelfLoop", selfLoop: true},
		{name: "TransposedSelfLoop", transposed: true, selfLoop: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dependencyGraph := NewDependencyGraph(testCase.transposed)
			dependent := TaskNode{ID: "dependent"}
			dependency := TaskNode{ID: "dependency"}
			if testCase.selfLoop {
				dependency = dependent
			}
			dependencyGraph.AddTaskNode(dependent)
			dependencyGraph.AddTaskNode(dependency)
			dependencyGraph.AddEdge(dependent, dependency, evergreen.TaskSucceeded)
			originalSort, err := dependencyGraph.TopologicalStableSort()
			require.NoError(t, err)

			fromTask, toTask := dependent, dependency
			if testCase.transposed {
				fromTask, toTask = dependency, dependent
			}
			for _, status := range []string{evergreen.TaskSucceeded, evergreen.TaskFailed, ""} {
				dependencyGraph.AddEdge(dependent, dependency, status)
				edge := dependencyGraph.GetDependencyEdge(fromTask, toTask)
				require.NotNil(t, edge)
				assert.Equal(t, status, edge.Status)
				assert.Len(t, dependencyGraph.graph.outgoing[dependencyGraph.graph.tasksToNodes[fromTask]], 1)
				assert.Len(t, dependencyGraph.graph.incoming[dependencyGraph.graph.tasksToNodes[toTask]], 1)
				assert.Len(t, dependencyGraph.graph.statuses, 1)
				assert.Equal(t, []DependencyEdge{*edge}, dependencyGraph.EdgesIntoTask(toTask))
				assert.True(t, dependencyGraph.DepthFirstSearch(fromTask, toTask, nil))
			}

			sortedTasks, err := dependencyGraph.TopologicalStableSort()
			require.NoError(t, err)
			assert.Equal(t, originalSort, sortedTasks)
			if testCase.selfLoop {
				assert.Equal(t, DependencyCycles{{dependent, dependent}}, dependencyGraph.Cycles())
			} else {
				assert.Empty(t, dependencyGraph.Cycles())
				dependencyGraph.AddEdge(dependency, dependent, "")
				dependencyGraph.AddEdge(dependency, dependent, "")
				cycles := dependencyGraph.Cycles()
				require.Len(t, cycles, 1)
				assert.ElementsMatch(t, []TaskNode{dependent, dependency}, cycles[0])
				assert.Len(t, dependencyGraph.graph.statuses, 2)
				assert.Len(t, dependencyGraph.graph.outgoing[dependencyGraph.graph.tasksToNodes[toTask]], 1)
				assert.Len(t, dependencyGraph.graph.incoming[dependencyGraph.graph.tasksToNodes[fromTask]], 1)
			}
		})
	}
}

func BenchmarkDependencyGraphRepeatedEdgeInsertion(benchmark *testing.B) {
	dependent := TaskNode{ID: "dependent"}
	dependency := TaskNode{ID: "dependency"}
	benchmark.ReportAllocs()
	for range benchmark.N {
		dependencyGraph := NewDependencyGraph(false)
		dependencyGraph.AddTaskNode(dependent)
		dependencyGraph.AddTaskNode(dependency)
		for range 1000 {
			dependencyGraph.AddEdge(dependent, dependency, evergreen.TaskSucceeded)
		}
	}
}

func TestDependencyGraphCopiesShareNodesEdgesAndStatuses(t *testing.T) {
	original := NewDependencyGraph(false)
	first := TaskNode{ID: "first"}
	original.AddTaskNode(first)
	copied := original
	for index := range 300 {
		copied.AddTaskNode(TaskNode{ID: fmt.Sprintf("task-%d", index)})
	}
	last := TaskNode{ID: "task-299"}
	copied.AddEdge(first, last, evergreen.TaskSucceeded)
	assert.Len(t, original.Nodes(), 301)
	assert.Equal(t, &DependencyEdge{From: first, To: last, Status: evergreen.TaskSucceeded}, original.GetDependencyEdge(first, last))
	original.AddEdge(first, last, evergreen.TaskFailed)
	edge := copied.GetDependencyEdge(first, last)
	require.NotNil(t, edge)
	assert.Equal(t, evergreen.TaskFailed, edge.Status)
	edge.Status = "changed"
	assert.Equal(t, evergreen.TaskFailed, original.GetDependencyEdge(first, last).Status)
	nodes := copied.Nodes()
	nodes[0] = TaskNode{ID: "changed"}
	assert.Contains(t, original.Nodes(), first)
	assert.True(t, original.DepthFirstSearch(first, last, nil))
	assert.Nil(t, original.GetDependencyEdge(TaskNode{ID: "missing"}, first))
	assert.Nil(t, original.GetDependencyEdge(first, TaskNode{ID: "missing"}))
}

func TestCompactDependencyGraphPreservesTaskIdentityAndIteratorBehavior(t *testing.T) {
	dependencyGraph := NewDependencyGraph(false)
	assert.Nil(t, dependencyGraph.graph.Node(0))
	assert.Zero(t, dependencyGraph.graph.Nodes().Len())
	nodes := []TaskNode{
		{Name: "compile", Variant: "ubuntu"},
		{Name: "compile", Variant: "rhel"},
		{Name: "compile", Variant: "ubuntu", ID: "execution-1"},
		{Name: "compile", Variant: "ubuntu", ID: "execution-2"},
	}
	for _, node := range nodes {
		dependencyGraph.AddTaskNode(node)
		dependencyGraph.AddTaskNode(node)
	}
	assert.Equal(t, nodes, dependencyGraph.Nodes())
	dependencyGraph.AddEdge(nodes[0], nodes[1], evergreen.TaskSucceeded)
	dependencyGraph.AddEdge(nodes[0], nodes[2], evergreen.TaskFailed)
	dependencyGraph.AddEdge(nodes[2], nodes[3], "")
	for _, id := range []int64{-1, int64(len(nodes))} {
		assert.Nil(t, dependencyGraph.graph.Node(id))
		assert.Zero(t, dependencyGraph.graph.From(id).Len())
		assert.Zero(t, dependencyGraph.graph.To(id).Len())
		assert.False(t, dependencyGraph.graph.HasEdgeBetween(id, 0))
		assert.Nil(t, dependencyGraph.graph.Edge(id, 0))
	}
	for _, testCase := range []struct {
		name     string
		iterator graph.Nodes
		expected []int64
	}{
		{name: "Nodes", iterator: dependencyGraph.graph.Nodes(), expected: []int64{0, 1, 2, 3}},
		{name: "Outgoing", iterator: dependencyGraph.graph.From(0), expected: []int64{1, 2}},
		{name: "Incoming", iterator: dependencyGraph.graph.To(3), expected: []int64{2}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			for range 2 {
				assert.Equal(t, len(testCase.expected), testCase.iterator.Len())
				for index, id := range testCase.expected {
					require.True(t, testCase.iterator.Next())
					assert.Equal(t, id, testCase.iterator.Node().ID())
					assert.Equal(t, len(testCase.expected)-index-1, testCase.iterator.Len())
				}
				assert.False(t, testCase.iterator.Next())
				assert.False(t, testCase.iterator.Next())
				testCase.iterator.Reset()
			}
		})
	}
	edge := dependencyGraph.graph.Edge(0, 1)
	require.NotNil(t, edge)
	assert.Equal(t, int64(1), edge.ReversedEdge().From().ID())
	assert.Equal(t, int64(0), edge.ReversedEdge().To().ID())
	assert.Nil(t, dependencyGraph.graph.Edge(1, 0))
	assert.True(t, dependencyGraph.graph.HasEdgeBetween(1, 0))
	assert.False(t, dependencyGraph.DepthFirstSearch(nodes[1], nodes[3], nil))
	assert.True(t, dependencyGraph.DepthFirstSearch(nodes[0], nodes[3], nil))
}

func TestCompactDependencyGraphMatchesMultigraphAlgorithms(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		transposed bool
		cyclic     bool
	}{
		{name: "DAG"},
		{name: "TransposedDAG", transposed: true},
		{name: "Cycles", cyclic: true},
		{name: "TransposedCycles", transposed: true, cyclic: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			const nodeCount = 350
			dependencyGraph := NewDependencyGraph(testCase.transposed)
			reference := multi.NewDirectedGraph()
			nodes := make([]TaskNode, nodeCount)
			for index := range nodes {
				nodes[index] = TaskNode{ID: fmt.Sprintf("task-%d", index)}
				dependencyGraph.AddTaskNode(nodes[index])
				reference.AddNode(reference.NewNode())
			}
			statuses := make(map[edgeKey]string)
			random := rand.New(rand.NewPCG(1, 2))
			for range 1500 {
				from, to := random.IntN(nodeCount), random.IntN(nodeCount)
				if !testCase.cyclic && from >= to {
					continue
				}
				status := []string{"", evergreen.TaskSucceeded, evergreen.TaskFailed}[random.IntN(3)]
				dependencyGraph.AddEdge(nodes[from], nodes[to], status)
				if testCase.transposed {
					from, to = to, from
				}
				key := edgeKey{from: compactNode(from), to: compactNode(to)}
				if _, exists := statuses[key]; !exists {
					reference.SetLine(reference.NewLine(reference.Node(int64(from)), reference.Node(int64(to))))
				}
				statuses[key] = status
			}
			for key, status := range statuses {
				assert.Equal(t, &DependencyEdge{From: nodes[key.from], To: nodes[key.to], Status: status}, dependencyGraph.GetDependencyEdge(nodes[key.from], nodes[key.to]))
			}
			for index, node := range nodes {
				var expected []DependencyEdge
				incoming := reference.To(int64(index))
				for incoming.Next() {
					from := incoming.Node().ID()
					expected = append(expected, DependencyEdge{From: nodes[from], To: node, Status: statuses[edgeKey{from: compactNode(from), to: compactNode(index)}]})
				}
				assert.ElementsMatch(t, expected, dependencyGraph.EdgesIntoTask(node))
			}
			for range 100 {
				from, to := random.IntN(nodeCount), random.IntN(nodeCount)
				for _, filterStatus := range []string{"", evergreen.TaskSucceeded, evergreen.TaskFailed} {
					traversal := traverse.DepthFirst{Traverse: func(edge graph.Edge) bool {
						return statuses[edgeKey{from: compactNode(edge.From().ID()), to: compactNode(edge.To().ID())}] == filterStatus
					}}
					expected := traversal.Walk(reference, reference.Node(int64(from)), func(node graph.Node) bool { return node.ID() == int64(to) }) != nil
					assert.Equal(t, expected, dependencyGraph.DepthFirstSearch(nodes[from], nodes[to], func(edge DependencyEdge) bool { return edge.Status == filterStatus }))
				}
			}
			var expectedCycles []string
			for _, component := range topo.TarjanSCC(reference) {
				if len(component) == 1 && !reference.HasEdgeBetween(component[0].ID(), component[0].ID()) {
					continue
				}
				var ids []string
				for _, node := range component {
					ids = append(ids, nodes[node.ID()].ID)
				}
				if len(component) == 1 {
					ids = append(ids, ids[0])
				}
				slices.Sort(ids)
				expectedCycles = append(expectedCycles, fmt.Sprint(ids))
			}
			var actualCycles []string
			for _, cycle := range dependencyGraph.Cycles() {
				var ids []string
				for _, node := range cycle {
					ids = append(ids, node.ID)
				}
				slices.Sort(ids)
				actualCycles = append(actualCycles, fmt.Sprint(ids))
			}
			assert.ElementsMatch(t, expectedCycles, actualCycles)
			expectedOrder, referenceErr := topo.SortStabilized(reference, nil)
			if referenceErr != nil {
				require.IsType(t, topo.Unorderable{}, referenceErr)
			}
			expectedNodes := make([]TaskNode, 0, len(expectedOrder))
			for _, node := range expectedOrder {
				if node != nil {
					expectedNodes = append(expectedNodes, nodes[node.ID()])
				}
			}
			actualNodes, err := dependencyGraph.TopologicalStableSort()
			require.NoError(t, err)
			assert.Equal(t, expectedNodes, actualNodes)
		})
	}
}

func TestGetDependencyEdge(t *testing.T) {
	tasks := []Task{
		{Id: "t0", DependsOn: []Dependency{{TaskId: "t1"}}},
		{Id: "t1", DependsOn: []Dependency{{TaskId: "t2", Status: evergreen.TaskSucceeded}}},
		{Id: "t2"},
	}
	g := NewDependencyGraph(false)
	g.buildFromTasks(tasks)

	t.Run("ExistingEdgeWithStatus", func(t *testing.T) {
		edge := g.GetDependencyEdge(tasks[1].ToTaskNode(), tasks[2].ToTaskNode())
		require.NotNil(t, edge)
		assert.Equal(t, evergreen.TaskSucceeded, edge.Status)
	})

	t.Run("ExistingEdgeNoStatus", func(t *testing.T) {
		edge := g.GetDependencyEdge(tasks[0].ToTaskNode(), tasks[1].ToTaskNode())
		require.NotNil(t, edge)
		assert.Empty(t, edge.Status)
	})

	t.Run("NonexistentEdge", func(t *testing.T) {
		edge := g.GetDependencyEdge(tasks[2].ToTaskNode(), tasks[0].ToTaskNode())
		assert.Nil(t, edge)
	})
}

func TestTasksDependingOnTask(t *testing.T) {
	tasks := []Task{
		{Id: "t0", DependsOn: []Dependency{{TaskId: "t1"}}},
		{Id: "t1"},
	}
	g := NewDependencyGraph(false)
	g.buildFromTasks(tasks)

	assert.Empty(t, g.EdgesIntoTask(tasks[0].ToTaskNode()))
	edges := g.EdgesIntoTask(tasks[1].ToTaskNode())
	require.Len(t, edges, 1)
	assert.Equal(t, tasks[0].Id, edges[0].From.ID)
}

func TestCycles(t *testing.T) {
	t.Run("EmptyGraph", func(t *testing.T) {
		g := NewDependencyGraph(false)
		assert.Empty(t, g.Cycles())
	})

	t.Run("NoCycles", func(t *testing.T) {
		tasks := []Task{
			{Id: "t0", DependsOn: []Dependency{{TaskId: "t1"}}},
			{Id: "t1"},
		}
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)

		assert.Empty(t, g.Cycles())
	})

	t.Run("Loops", func(t *testing.T) {
		tasks := []Task{
			{Id: "t0", DependsOn: []Dependency{{TaskId: "t0"}}},
		}
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)

		cycles := g.Cycles()
		assert.Len(t, cycles, 1)
	})

	t.Run("TwoConnectedCycles", func(t *testing.T) {
		tasks := []Task{
			{Id: "t0", DependsOn: []Dependency{{TaskId: "t1"}}},
			{Id: "t1", DependsOn: []Dependency{{TaskId: "t0"}, {TaskId: "t2"}}},
			{Id: "t2", DependsOn: []Dependency{{TaskId: "t3"}}},
			{Id: "t3", DependsOn: []Dependency{{TaskId: "t2"}}},
		}
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)

		cycles := g.Cycles()
		assert.Len(t, cycles, 2)
	})

	t.Run("TwoDisconnectedCycles", func(t *testing.T) {
		tasks := []Task{
			{Id: "t0", DependsOn: []Dependency{{TaskId: "t1"}}},
			{Id: "t1", DependsOn: []Dependency{{TaskId: "t0"}}},
			{Id: "t2", DependsOn: []Dependency{{TaskId: "t3"}}},
			{Id: "t3", DependsOn: []Dependency{{TaskId: "t2"}}},
		}
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)

		cycles := g.Cycles()
		assert.Len(t, cycles, 2)
	})
}

func TestDependencyCyclesString(t *testing.T) {
	t.Run("NoCycles", func(t *testing.T) {
		dc := DependencyCycles{}
		assert.Empty(t, dc.String())
	})

	t.Run("OneCycle", func(t *testing.T) {
		ids := []string{"t0", "t1"}
		dc := DependencyCycles{
			{{ID: ids[0]}, {ID: ids[1]}},
		}
		assert.Equal(t, fmt.Sprintf("[%s, %s]", ids[0], ids[1]), dc.String())
	})

	t.Run("TwoCycles", func(t *testing.T) {
		ids := []string{"t0", "t1", "t2", "t3"}
		dc := DependencyCycles{
			{{ID: ids[0]}, {ID: ids[1]}},
			{{ID: ids[2]}, {ID: ids[3]}},
		}
		assert.Equal(t, fmt.Sprintf("[%s, %s], [%s, %s]", ids[0], ids[1], ids[2], ids[3]), dc.String())
	})
}

func TestDepthFirstSearch(t *testing.T) {
	tasks := []Task{
		{Id: "t0", DependsOn: []Dependency{{TaskId: "t1", Status: evergreen.TaskSucceeded}}},
		{Id: "t1", DependsOn: []Dependency{{TaskId: "t2"}}},
		{Id: "t2"},
		{Id: "t3"},
	}
	g := NewDependencyGraph(false)
	g.buildFromTasks(tasks)

	t.Run("NilTraverseEdge", func(t *testing.T) {
		assert.True(t, g.DepthFirstSearch(tasks[0].ToTaskNode(), tasks[2].ToTaskNode(), nil))
		assert.False(t, g.DepthFirstSearch(tasks[1].ToTaskNode(), tasks[0].ToTaskNode(), nil))
		assert.False(t, g.DepthFirstSearch(tasks[3].ToTaskNode(), tasks[0].ToTaskNode(), nil))
	})

	t.Run("TraversalBlockedAtNode", func(t *testing.T) {
		assert.False(t, g.DepthFirstSearch(tasks[0].ToTaskNode(), tasks[2].ToTaskNode(), func(edge DependencyEdge) bool {
			return edge.To != tasks[1].ToTaskNode()
		}))
	})

	t.Run("TraversalBlockedAtEdge", func(t *testing.T) {
		assert.False(t, g.DepthFirstSearch(tasks[0].ToTaskNode(), tasks[2].ToTaskNode(), func(edge DependencyEdge) bool {
			return edge.Status == evergreen.TaskSucceeded
		}))
	})

	t.Run("StartMissingFromGraph", func(t *testing.T) {
		assert.False(t, g.DepthFirstSearch(TaskNode{ID: "t4"}, tasks[0].ToTaskNode(), nil))
	})

	t.Run("TargetMissingFromGraph", func(t *testing.T) {
		assert.False(t, g.DepthFirstSearch(tasks[0].ToTaskNode(), TaskNode{ID: "t4"}, nil))
	})
}

func TestTopologicalStableSort(t *testing.T) {
	t.Run("StableSortNoDependencies", func(t *testing.T) {
		tasks := []Task{
			{Id: "t0"},
			{Id: "t1"},
			{Id: "t2"},
		}
		g := NewDependencyGraph(true)
		g.buildFromTasks(tasks)

		sortedNodes, err := g.TopologicalStableSort()
		assert.NoError(t, err)
		require.Len(t, sortedNodes, 3)
		assert.Equal(t, tasks[0].ToTaskNode(), sortedNodes[0])
		assert.Equal(t, tasks[1].ToTaskNode(), sortedNodes[1])
		assert.Equal(t, tasks[2].ToTaskNode(), sortedNodes[2])
	})

	t.Run("StableSortWithDependencies", func(t *testing.T) {
		tasks := []Task{
			{Id: "t0", DependsOn: []Dependency{{TaskId: "t1"}}},
			{Id: "t1"},
			{Id: "t2"},
		}
		g := NewDependencyGraph(true)
		g.buildFromTasks(tasks)

		sortedNodes, err := g.TopologicalStableSort()
		assert.NoError(t, err)
		require.Len(t, sortedNodes, 3)
		assert.Equal(t, tasks[1].ToTaskNode(), sortedNodes[0])
		assert.Equal(t, tasks[0].ToTaskNode(), sortedNodes[1])
		assert.Equal(t, tasks[2].ToTaskNode(), sortedNodes[2])
	})

	t.Run("Cycle", func(t *testing.T) {
		tasks := []Task{
			{Id: "t0", DependsOn: []Dependency{{TaskId: "t1"}}},
			{Id: "t1", DependsOn: []Dependency{{TaskId: "t0"}}},
			{Id: "t2"},
		}
		g := NewDependencyGraph(true)
		g.buildFromTasks(tasks)

		sortedNodes, err := g.TopologicalStableSort()
		assert.NoError(t, err)
		require.Len(t, sortedNodes, 1)
		assert.Equal(t, tasks[2].ToTaskNode(), sortedNodes[0])
	})

	t.Run("EmptyGraph", func(t *testing.T) {
		g := NewDependencyGraph(true)

		sortedNodes, err := g.TopologicalStableSort()
		assert.NoError(t, err)
		assert.Empty(t, sortedNodes)
	})
}
