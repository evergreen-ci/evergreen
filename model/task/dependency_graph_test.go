package task

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskNodeString(t *testing.T) {
	assert.Equal(t, "t0", TaskNode{ID: "t0", Name: "task0", Variant: "BV0"}.String())
	assert.Equal(t, "BV0/task0", TaskNode{Name: "task0", Variant: "BV0"}.String())
}

func TestBuildFromTasks(t *testing.T) {
	tasks := []Task{
		{
			Id: "t0",
			DependsOn: []Dependency{
				{
					TaskId: "t1",
					Status: evergreen.TaskSucceeded,
				},
			},
		},
		{
			Id: "t1",
			DependsOn: []Dependency{
				{TaskId: "t2"},
				{TaskId: "t3"},
			},
		},
		{Id: "t2"},
		{Id: "t3"},
	}

	t.Run("ForwardEdges", func(t *testing.T) {
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)

		for _, task := range tasks {
			require.Contains(t, g.tasksToNodes, task.ToTaskNode())
			assert.Equal(t, task.ToTaskNode(), g.nodesToTasks[g.tasksToNodes[task.ToTaskNode()]])
		}

		assert.Equal(t, 0, g.graph.To(g.tasksToNodes[tasks[0].ToTaskNode()].ID()).Len())
		assert.Equal(t, 1, g.graph.From(g.tasksToNodes[tasks[0].ToTaskNode()].ID()).Len())

		assert.Equal(t, 1, g.graph.To(g.tasksToNodes[tasks[1].ToTaskNode()].ID()).Len())
		assert.Equal(t, 2, g.graph.From(g.tasksToNodes[tasks[1].ToTaskNode()].ID()).Len())

		assert.Equal(t, 1, g.graph.To(g.tasksToNodes[tasks[2].ToTaskNode()].ID()).Len())
		assert.Equal(t, 0, g.graph.From(g.tasksToNodes[tasks[2].ToTaskNode()].ID()).Len())

		assert.Equal(t, 1, g.graph.To(g.tasksToNodes[tasks[3].ToTaskNode()].ID()).Len())
		assert.Equal(t, 0, g.graph.From(g.tasksToNodes[tasks[3].ToTaskNode()].ID()).Len())

		assert.Len(t, g.edgesToDependencies, 3)

		assert.True(t, g.graph.HasEdgeFromTo(g.tasksToNodes[tasks[0].ToTaskNode()].ID(), g.tasksToNodes[tasks[1].ToTaskNode()].ID()))
		assert.Equal(t,
			DependencyEdge{From: tasks[0].ToTaskNode(), To: tasks[1].ToTaskNode(), Status: evergreen.TaskSucceeded},
			g.edgesToDependencies[edgeKey{from: tasks[0].ToTaskNode(), to: tasks[1].ToTaskNode()}],
		)

		assert.True(t, g.graph.HasEdgeFromTo(g.tasksToNodes[tasks[1].ToTaskNode()].ID(), g.tasksToNodes[tasks[2].ToTaskNode()].ID()))
		assert.Equal(t,
			DependencyEdge{From: tasks[1].ToTaskNode(), To: tasks[2].ToTaskNode()},
			g.edgesToDependencies[edgeKey{from: tasks[1].ToTaskNode(), to: tasks[2].ToTaskNode()}],
		)

		assert.True(t, g.graph.HasEdgeFromTo(g.tasksToNodes[tasks[1].ToTaskNode()].ID(), g.tasksToNodes[tasks[3].ToTaskNode()].ID()))
		assert.Equal(t,
			DependencyEdge{From: tasks[1].ToTaskNode(), To: tasks[3].ToTaskNode()},
			g.edgesToDependencies[edgeKey{from: tasks[1].ToTaskNode(), to: tasks[3].ToTaskNode()}],
		)
	})

	t.Run("ReversedEdges", func(t *testing.T) {
		g := NewDependencyGraph(true)
		g.buildFromTasks(tasks)

		for _, task := range tasks {
			require.Contains(t, g.tasksToNodes, task.ToTaskNode())
			assert.Equal(t, task.ToTaskNode(), g.nodesToTasks[g.tasksToNodes[task.ToTaskNode()]])
		}

		assert.Equal(t, 1, g.graph.To(g.tasksToNodes[tasks[0].ToTaskNode()].ID()).Len())
		assert.Equal(t, 0, g.graph.From(g.tasksToNodes[tasks[0].ToTaskNode()].ID()).Len())

		assert.Equal(t, 2, g.graph.To(g.tasksToNodes[tasks[1].ToTaskNode()].ID()).Len())
		assert.Equal(t, 1, g.graph.From(g.tasksToNodes[tasks[1].ToTaskNode()].ID()).Len())

		assert.Equal(t, 0, g.graph.To(g.tasksToNodes[tasks[2].ToTaskNode()].ID()).Len())
		assert.Equal(t, 1, g.graph.From(g.tasksToNodes[tasks[2].ToTaskNode()].ID()).Len())

		assert.Equal(t, 0, g.graph.To(g.tasksToNodes[tasks[3].ToTaskNode()].ID()).Len())
		assert.Equal(t, 1, g.graph.From(g.tasksToNodes[tasks[3].ToTaskNode()].ID()).Len())

		assert.Len(t, g.edgesToDependencies, 3)

		assert.True(t, g.graph.HasEdgeFromTo(g.tasksToNodes[tasks[1].ToTaskNode()].ID(), g.tasksToNodes[tasks[0].ToTaskNode()].ID()))
		assert.Equal(t,
			DependencyEdge{From: tasks[1].ToTaskNode(), To: tasks[0].ToTaskNode(), Status: evergreen.TaskSucceeded},
			g.edgesToDependencies[edgeKey{from: tasks[1].ToTaskNode(), to: tasks[0].ToTaskNode()}],
		)

		assert.True(t, g.graph.HasEdgeFromTo(g.tasksToNodes[tasks[2].ToTaskNode()].ID(), g.tasksToNodes[tasks[1].ToTaskNode()].ID()))
		assert.Equal(t,
			DependencyEdge{From: tasks[2].ToTaskNode(), To: tasks[1].ToTaskNode()},
			g.edgesToDependencies[edgeKey{from: tasks[2].ToTaskNode(), to: tasks[1].ToTaskNode()}],
		)

		assert.True(t, g.graph.HasEdgeFromTo(g.tasksToNodes[tasks[3].ToTaskNode()].ID(), g.tasksToNodes[tasks[1].ToTaskNode()].ID()))
		assert.Equal(t,
			DependencyEdge{From: tasks[3].ToTaskNode(), To: tasks[1].ToTaskNode()},
			g.edgesToDependencies[edgeKey{from: tasks[3].ToTaskNode(), to: tasks[1].ToTaskNode()}],
		)
	})
}

func TestAddTaskNode(t *testing.T) {
	tasks := []Task{
		{Id: "t0"},
	}

	t.Run("NewNode", func(t *testing.T) {
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)
		assert.Len(t, g.nodesToTasks, 1)
		assert.Len(t, g.tasksToNodes, 1)
		assert.Equal(t, 1, g.graph.Nodes().Len())

		g.AddTaskNode(TaskNode{ID: "t1"})
		assert.Len(t, g.nodesToTasks, 2)
		assert.Len(t, g.tasksToNodes, 2)
		assert.Equal(t, 2, g.graph.Nodes().Len())
	})

	t.Run("PreexistingNode", func(t *testing.T) {
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)
		assert.Len(t, g.nodesToTasks, 1)
		assert.Len(t, g.tasksToNodes, 1)
		assert.Equal(t, 1, g.graph.Nodes().Len())

		g.AddTaskNode(TaskNode{ID: "t0"})
		assert.Len(t, g.nodesToTasks, 1)
		assert.Len(t, g.tasksToNodes, 1)
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
		assert.Equal(t, 1, g.graph.Edges().Len())
		assert.Len(t, g.edgesToDependencies, 1)

		g.addEdgeToGraph(DependencyEdge{From: tasks[0].ToTaskNode(), To: tasks[1].ToTaskNode()})
		assert.Equal(t, 2, g.graph.Edges().Len())
		assert.True(t, g.graph.HasEdgeFromTo(g.tasksToNodes[tasks[0].ToTaskNode()].ID(), g.tasksToNodes[tasks[1].ToTaskNode()].ID()))
		assert.Len(t, g.edgesToDependencies, 2)
	})

	t.Run("PreexistingEdge", func(t *testing.T) {
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)
		assert.Equal(t, 1, g.graph.Edges().Len())
		assert.Len(t, g.edgesToDependencies, 1)

		g.addEdgeToGraph(DependencyEdge{From: tasks[1].ToTaskNode(), To: tasks[2].ToTaskNode()})
		assert.Equal(t, 1, g.graph.Edges().Len())
		assert.Len(t, g.edgesToDependencies, 1)
	})

	t.Run("EdgeToMissingNode", func(t *testing.T) {
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)
		assert.Equal(t, 1, g.graph.Edges().Len())
		assert.Len(t, g.edgesToDependencies, 1)

		g.addEdgeToGraph(DependencyEdge{From: tasks[0].ToTaskNode(), To: TaskNode{ID: "t3"}})
		assert.Equal(t, 1, g.graph.Edges().Len())
		assert.Len(t, g.edgesToDependencies, 1)
	})

	t.Run("Cyclic", func(t *testing.T) {
		g := NewDependencyGraph(false)
		g.buildFromTasks(tasks)
		assert.Equal(t, 1, g.graph.Edges().Len())
		assert.Len(t, g.edgesToDependencies, 1)

		g.addEdgeToGraph(DependencyEdge{From: tasks[0].ToTaskNode(), To: tasks[1].ToTaskNode()})
		g.addEdgeToGraph(DependencyEdge{From: tasks[1].ToTaskNode(), To: tasks[0].ToTaskNode()})
		assert.Equal(t, 3, g.graph.Edges().Len())
		assert.True(t, g.graph.HasEdgeFromTo(g.tasksToNodes[tasks[0].ToTaskNode()].ID(), g.tasksToNodes[tasks[1].ToTaskNode()].ID()))
		assert.True(t, g.graph.HasEdgeFromTo(g.tasksToNodes[tasks[1].ToTaskNode()].ID(), g.tasksToNodes[tasks[0].ToTaskNode()].ID()))
		assert.Len(t, g.edgesToDependencies, 3)
	})
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

	assert.Empty(t, g.edgesIntoTask(tasks[0].ToTaskNode()))
	edges := g.edgesIntoTask(tasks[1].ToTaskNode())
	require.Len(t, edges, 1)
	assert.Equal(t, tasks[0].Id, edges[0].From.ID)
}

func TestReachableFromNode(t *testing.T) {
	tasks := []Task{
		{Id: "t0", DependsOn: []Dependency{{TaskId: "t1"}, {TaskId: "t2"}}},
		{Id: "t1", DependsOn: []Dependency{{TaskId: "t3"}}},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t4"}}},
		{Id: "t3"},
		{Id: "t4"},
	}
	g := NewDependencyGraph(false)
	g.buildFromTasks(tasks)

	reachable := g.reachableFromNode(tasks[0].ToTaskNode())
	assert.Len(t, reachable, 4)
	assert.Contains(t, reachable, tasks[1].ToTaskNode())
	assert.Contains(t, reachable, tasks[2].ToTaskNode())
	assert.Contains(t, reachable, tasks[3].ToTaskNode())
	assert.Contains(t, reachable, tasks[4].ToTaskNode())

	reachable = g.reachableFromNode(tasks[1].ToTaskNode())
	assert.Len(t, reachable, 1)
	assert.Contains(t, reachable, tasks[3].ToTaskNode())

	reachable = g.reachableFromNode(tasks[3].ToTaskNode())
	assert.Empty(t, reachable)
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

		assert.Empty(t, g.Cycles())
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
			if edge.To == tasks[1].ToTaskNode() {
				return false
			}
			return true
		}))
	})

	t.Run("TraversalBlockedAtEdge", func(t *testing.T) {
		assert.False(t, g.DepthFirstSearch(tasks[0].ToTaskNode(), tasks[2].ToTaskNode(), func(edge DependencyEdge) bool {
			if edge.Status == evergreen.TaskSucceeded {
				return true
			}
			return false
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
