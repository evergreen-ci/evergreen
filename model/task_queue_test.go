package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDequeueTask(t *testing.T) {
	const distroID = "d1"
	taskIDs := []string{"t1", "t2", "t3"}

	// remainingTaskIDs returns the tasks still waiting in the distro's queue.
	// LoadTaskQueue returns a nil queue once every item is dispatched.
	remainingTaskIDs := func(t *testing.T) []string {
		queue, err := LoadTaskQueue(t.Context(), distroID)
		require.NoError(t, err)
		ids := []string{}
		if queue == nil {
			return ids
		}
		for _, item := range queue.Queue {
			ids = append(ids, item.Id)
		}
		return ids
	}

	for tName, tCase := range map[string]func(t *testing.T){
		"EmptyQueueShouldNotError": func(t *testing.T) {
			require.NoError(t, NewTaskQueue(distroID, []TaskQueueItem{}, DistroQueueInfo{}).Save(t.Context()))
			assert.NoError(t, DequeueTask(t.Context(), taskIDs[0], distroID))
		},
		"TaskMissingFromQueueShouldNotError": func(t *testing.T) {
			require.NoError(t, NewTaskQueue(distroID, []TaskQueueItem{{Id: taskIDs[1]}}, DistroQueueInfo{}).Save(t.Context()))
			assert.NoError(t, DequeueTask(t.Context(), taskIDs[0], distroID))
			assert.Equal(t, []string{taskIDs[1]}, remainingTaskIDs(t))
		},
		"NonexistentDistroQueueShouldNotError": func(t *testing.T) {
			assert.NoError(t, DequeueTask(t.Context(), taskIDs[0], "nonexistent"))
		},
		"QueuedTaskShouldNoLongerBeWaiting": func(t *testing.T) {
			items := []TaskQueueItem{{Id: taskIDs[0]}, {Id: taskIDs[1]}, {Id: taskIDs[2]}}
			require.NoError(t, NewTaskQueue(distroID, items, DistroQueueInfo{}).Save(t.Context()))

			require.NoError(t, DequeueTask(t.Context(), taskIDs[1], distroID))
			assert.Equal(t, []string{taskIDs[0], taskIDs[2]}, remainingTaskIDs(t))

			require.NoError(t, DequeueTask(t.Context(), taskIDs[2], distroID))
			require.NoError(t, DequeueTask(t.Context(), taskIDs[0], distroID))
			assert.Empty(t, remainingTaskIDs(t))
		},
		"DuplicateTaskShouldOnlyDequeueTheFirstOccurrence": func(t *testing.T) {
			items := []TaskQueueItem{{Id: taskIDs[0]}, {Id: taskIDs[1]}, {Id: taskIDs[0]}}
			require.NoError(t, NewTaskQueue(distroID, items, DistroQueueInfo{}).Save(t.Context()))

			require.NoError(t, DequeueTask(t.Context(), taskIDs[0], distroID))
			assert.Equal(t, []string{taskIDs[1], taskIDs[0]}, remainingTaskIDs(t))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(TaskQueuesCollection))
			tCase(t)
		})
	}
}

func TestGetTaskQueueLengths(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"NoQueueForDistroShouldReturnNil": func(t *testing.T) {
			lengths, err := GetTaskQueueLengths(t.Context(), "nonexistent", TaskQueuesCollection)
			require.NoError(t, err)
			assert.Nil(t, lengths)
		},
		"EmptyQueueShouldReturnZeroLengths": func(t *testing.T) {
			require.NoError(t, NewTaskQueue("d1", []TaskQueueItem{}, DistroQueueInfo{}).Save(t.Context()))
			lengths, err := GetTaskQueueLengths(t.Context(), "d1", TaskQueuesCollection)
			require.NoError(t, err)
			require.NotNil(t, lengths)
			assert.Zero(t, lengths.Undispatched)
			assert.Zero(t, lengths.UndispatchedWithDependenciesMet)
		},
		"ShouldOnlyCountUndispatchedItems": func(t *testing.T) {
			items := []TaskQueueItem{
				{Id: "t1", DependenciesMet: true},
				{Id: "t2", DependenciesMet: true, IsDispatched: true},
				{Id: "t3"},
				{Id: "t4", IsDispatched: true},
			}
			require.NoError(t, NewTaskQueue("d1", items, DistroQueueInfo{Length: 4}).Save(t.Context()))

			lengths, err := GetTaskQueueLengths(t.Context(), "d1", TaskQueuesCollection)
			require.NoError(t, err)
			require.NotNil(t, lengths)
			assert.Equal(t, 2, lengths.Undispatched)
			assert.Equal(t, 1, lengths.UndispatchedWithDependenciesMet)
		},
		"ShouldMatchLoadTaskQueueLength": func(t *testing.T) {
			items := []TaskQueueItem{
				{Id: "t1", DependenciesMet: true},
				{Id: "t2", IsDispatched: true},
				{Id: "t3", DependenciesMet: true},
			}
			require.NoError(t, NewTaskQueue("d1", items, DistroQueueInfo{Length: 3}).Save(t.Context()))

			queue, err := LoadTaskQueue(t.Context(), "d1")
			require.NoError(t, err)
			require.NotNil(t, queue)
			lengths, err := GetTaskQueueLengths(t.Context(), "d1", TaskQueuesCollection)
			require.NoError(t, err)
			require.NotNil(t, lengths)
			assert.Equal(t, queue.Length(), lengths.Undispatched)
		},
		"ShouldReadFromTheSecondaryQueueCollection": func(t *testing.T) {
			require.NoError(t, NewTaskQueue("d1", []TaskQueueItem{{Id: "t1"}}, DistroQueueInfo{}).Save(t.Context()))
			secondary := NewTaskQueue("d1", []TaskQueueItem{{Id: "t2"}, {Id: "t3"}}, DistroQueueInfo{SecondaryQueue: true})
			require.NoError(t, secondary.Save(t.Context()))

			lengths, err := GetTaskQueueLengths(t.Context(), "d1", TaskSecondaryQueuesCollection)
			require.NoError(t, err)
			require.NotNil(t, lengths)
			assert.Equal(t, 2, lengths.Undispatched)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(TaskQueuesCollection, TaskSecondaryQueuesCollection))
			tCase(t)
		})
	}
}

func TestClearTaskQueue(t *testing.T) {
	assert := assert.New(t)
	distro := "distro"
	otherDistro := "otherDistro"
	tasks := []TaskQueueItem{
		{
			Id: "task1",
		},
		{
			Id: "task2",
		},
		{
			Id: "task3",
		},
	}
	info := DistroQueueInfo{
		Length: 3,
		TaskGroupInfos: []TaskGroupInfo{
			{
				Name:             "taskGroupInfo1",
				Count:            8,
				ExpectedDuration: 2600127105386,
			},
		},
	}

	queue := NewTaskQueue(distro, tasks, info)
	assert.Len(queue.Queue, 3)
	assert.NoError(queue.Save(t.Context()))
	otherQueue := NewTaskQueue(otherDistro, tasks, info)
	assert.Len(otherQueue.Queue, 3)
	assert.NoError(otherQueue.Save(t.Context()))

	assert.NoError(ClearTaskQueue(t.Context(), distro))
	queueFromDb, err := LoadTaskQueue(t.Context(), distro)
	assert.NoError(err)
	assert.Empty(queueFromDb.Queue)
	otherQueueFromDb, err := LoadTaskQueue(t.Context(), otherDistro)
	assert.NoError(err)
	assert.Len(otherQueueFromDb.Queue, 3)
}

func TestFindDistroTaskQueue(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(TaskQueuesCollection))
	defer func() {
		assert.NoError(db.ClearCollections(TaskQueuesCollection))
	}()

	distroID := "distro1"
	info := DistroQueueInfo{
		Length: 8,
		TaskGroupInfos: []TaskGroupInfo{
			{
				Name:             "taskGroupInfo1",
				Count:            8,
				ExpectedDuration: 2600127105386,
			},
		},
	}
	taskQueueItems := []TaskQueueItem{
		{Id: "a", Dependencies: []string{"b"}},
		{Id: "b"},
		{Id: "c"},
		{Id: "d"},
		{Id: "e"},
		{Id: "f"},
		{Id: "g"},
		{Id: "h"},
	}

	taskQueueIn := NewTaskQueue(distroID, taskQueueItems, info)
	assert.NoError(taskQueueIn.Save(t.Context()))

	taskQueueOut, err := FindDistroTaskQueue(t.Context(), distroID)
	assert.NoError(err)
	assert.Equal(distroID, taskQueueOut.Distro)
	assert.Len(taskQueueOut.Queue, 8)
	assert.Equal(8, taskQueueOut.DistroQueueInfo.Length)
	assert.Len(taskQueueOut.Queue[0].Dependencies, 1)
	assert.Len(taskQueueOut.DistroQueueInfo.TaskGroupInfos, 1)
	assert.Equal("taskGroupInfo1", taskQueueOut.DistroQueueInfo.TaskGroupInfos[0].Name)
	assert.Equal(8, taskQueueOut.DistroQueueInfo.TaskGroupInfos[0].Count)
	assert.Equal(taskQueueOut.DistroQueueInfo.TaskGroupInfos[0].ExpectedDuration, time.Duration(2600127105386))
}

func TestGetDistroQueueInfo(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(TaskQueuesCollection))
	defer func() {
		assert.NoError(db.ClearCollections(TaskQueuesCollection))
	}()

	distroID := "distro1"
	info := DistroQueueInfo{
		Length: 8,
		TaskGroupInfos: []TaskGroupInfo{
			{
				Name:             "taskGroupInfo1",
				Count:            8,
				ExpectedDuration: 2600127105386,
			},
		},
	}
	taskQueueItems := []TaskQueueItem{
		{Id: "a"},
		{Id: "b"},
		{Id: "c"},
	}

	taskQueueIn := NewTaskQueue(distroID, taskQueueItems, info)
	assert.NoError(taskQueueIn.Save(t.Context()))

	distroQueueInfoOut, err := GetDistroQueueInfo(t.Context(), distroID)
	assert.NoError(err)
	assert.Equal(8, distroQueueInfoOut.Length)
	assert.Len(distroQueueInfoOut.TaskGroupInfos, 1)
	assert.Equal("taskGroupInfo1", distroQueueInfoOut.TaskGroupInfos[0].Name)
	assert.Equal(8, distroQueueInfoOut.TaskGroupInfos[0].Count)
	assert.Equal(distroQueueInfoOut.TaskGroupInfos[0].ExpectedDuration, time.Duration(2600127105386))
}

func TestFindDuplicateEnqueuedTasks(t *testing.T) {
	const coll = TaskQueuesCollection
	makeTaskQueue := func(t *testing.T, distroID string, ids ...string) *TaskQueue {
		tq := &TaskQueue{Distro: distroID}
		for _, id := range ids {
			tq.Queue = append(tq.Queue, TaskQueueItem{Id: id})
		}
		require.NoError(t, tq.Save(t.Context()))
		return tq
	}
	for testName, testCase := range map[string]func(t *testing.T){
		"MatchesDuplicatesAcrossDifferentQueues": func(t *testing.T) {
			_ = makeTaskQueue(t, "d1", "task1", "task2", "task3")
			_ = makeTaskQueue(t, "d2", "task1", "task3", "task4", "task5", "task6")
			_ = makeTaskQueue(t, "d3", "task3")
			dups, err := FindDuplicateEnqueuedTasks(t.Context(), coll)
			require.NoError(t, err)
			require.Len(t, dups, 2)
			var task1Found, task3Found bool
			for _, dup := range dups {
				if dup.TaskID == "task1" {
					expectedDistros := []string{"d1", "d2"}
					assert.Subset(t, dup.DistroIDs, expectedDistros)
					assert.Subset(t, expectedDistros, dup.DistroIDs)
					task1Found = true
				}
				if dup.TaskID == "task3" {
					expectedDistros := []string{"d1", "d2", "d3"}
					assert.Subset(t, dup.DistroIDs, expectedDistros)
					assert.Subset(t, expectedDistros, dup.DistroIDs)
					task3Found = true
				}
			}
			assert.True(t, task1Found)
			assert.True(t, task3Found)
		},
		"DoesNotMatchDuplicatesWithinSameQueue": func(t *testing.T) {
			_ = makeTaskQueue(t, "d1", "task1", "task1", "task2")
			dups, err := FindDuplicateEnqueuedTasks(t.Context(), coll)
			assert.NoError(t, err)
			assert.Empty(t, dups)
		},
		"DoesNotMatchEmptyQueues": func(t *testing.T) {
			_ = makeTaskQueue(t, "d1")
			dups, err := FindDuplicateEnqueuedTasks(t.Context(), coll)
			assert.NoError(t, err)
			assert.Empty(t, dups)
		},
		"DoesNotMatchAllUnique": func(t *testing.T) {
			_ = makeTaskQueue(t, "d1", "task1", "task2")
			_ = makeTaskQueue(t, "d2", "task3", "task4")
			dups, err := FindDuplicateEnqueuedTasks(t.Context(), coll)
			assert.NoError(t, err)
			assert.Empty(t, dups)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(coll))
			defer func() {
				assert.NoError(t, db.Clear(coll))
			}()
			testCase(t)
		})
	}
}
