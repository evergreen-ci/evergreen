package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDequeueTask(t *testing.T) {
	var taskIds []string
	var distroId string
	var taskQueue *TaskQueue

	Convey("When attempting to pull a task from a task queue", t, func() {

		taskIds = []string{"t1", "t2", "t3"}
		distroId = "d1"
		taskQueue = &TaskQueue{
			Distro: distroId,
			Queue:  []TaskQueueItem{},
		}

		So(db.Clear(TaskQueuesCollection), ShouldBeNil)

		Convey("if the task queue is empty, an error should not be thrown", func() {
			So(taskQueue.Save(), ShouldBeNil)
			So(taskQueue.DequeueTask(taskIds[0]), ShouldBeNil)
		})

		Convey("if the task is not present in the queue, an error should not be"+
			" thrown", func() {
			taskQueue.Queue = append(taskQueue.Queue,
				TaskQueueItem{Id: taskIds[1]})
			So(taskQueue.Save(), ShouldBeNil)
			So(taskQueue.DequeueTask(taskIds[0]), ShouldBeNil)
		})

		Convey("if the task is present in the in-memory queue but not in the db queue"+
			", an error should not be thrown", func() {
			taskQueue.Queue = append(taskQueue.Queue,
				TaskQueueItem{Id: taskIds[1]})
			So(taskQueue.Save(), ShouldBeNil)
			taskQueue.Queue = append(taskQueue.Queue,
				TaskQueueItem{Id: taskIds[0]})
			So(taskQueue.DequeueTask(taskIds[0]), ShouldBeNil)
		})

		Convey("if the task is present in the queue, it should be removed"+
			" from the in-memory and db versions of the queue", func() {
			taskQueue.Queue = []TaskQueueItem{
				{Id: taskIds[0]},
				{Id: taskIds[1]},
				{Id: taskIds[2]},
			}
			So(taskQueue.Save(), ShouldBeNil)
			So(taskQueue.DequeueTask(taskIds[1]), ShouldBeNil)

			// make sure the queue was updated in memory
			So(taskQueue.Length(), ShouldEqual, 2)
			So(taskQueue.Queue[0].Id, ShouldEqual, taskIds[0])
			So(taskQueue.Queue[1].Id, ShouldEqual, taskIds[2])

			var err error
			// make sure the db representation was updated
			taskQueue, err = LoadTaskQueue(distroId)
			So(err, ShouldBeNil)
			So(taskQueue.Length(), ShouldEqual, 2)
			So(taskQueue.Queue[0].Id, ShouldEqual, taskIds[0])
			So(taskQueue.Queue[1].Id, ShouldEqual, taskIds[2])

			// should be safe to remove the last item
			So(taskQueue.DequeueTask(taskIds[2]), ShouldBeNil)
			So(taskQueue.Length(), ShouldEqual, 1)

			So(taskQueue.DequeueTask(taskIds[0]), ShouldBeNil)
			So(taskQueue.Length(), ShouldEqual, 0)

			So(taskQueue.DequeueTask("foo"), ShouldBeNil)
			So(taskQueue.Length(), ShouldEqual, 0)
		})
		Convey("modern: duplicate tasks shouldn't lead to anics", func() {
			taskQueue.Queue = []TaskQueueItem{
				{Id: taskIds[0]},
				{Id: taskIds[1]},
				{Id: taskIds[0]},
			}
			So(taskQueue.Save(), ShouldBeNil)

			So(taskQueue.DequeueTask(taskIds[0]), ShouldBeNil)
			So(taskQueue.Length(), ShouldEqual, 1)
		})
	})
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
	assert.NoError(queue.Save())
	otherQueue := NewTaskQueue(otherDistro, tasks, info)
	assert.Len(otherQueue.Queue, 3)
	assert.NoError(otherQueue.Save())

	assert.NoError(ClearTaskQueue(distro))
	queueFromDb, err := LoadTaskQueue(distro)
	assert.NoError(err)
	assert.Len(queueFromDb.Queue, 0)
	otherQueueFromDb, err := LoadTaskQueue(otherDistro)
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
	assert.NoError(taskQueueIn.Save())

	taskQueueOut, err := FindDistroTaskQueue(distroID)
	assert.NoError(err)
	assert.Equal(distroID, taskQueueOut.Distro)
	assert.Len(taskQueueOut.Queue, 8)
	assert.Equal(taskQueueOut.DistroQueueInfo.Length, 8)
	assert.Len(taskQueueOut.Queue[0].Dependencies, 1)
	assert.Len(taskQueueOut.DistroQueueInfo.TaskGroupInfos, 1)
	assert.Equal(taskQueueOut.DistroQueueInfo.TaskGroupInfos[0].Name, "taskGroupInfo1")
	assert.Equal(taskQueueOut.DistroQueueInfo.TaskGroupInfos[0].Count, 8)
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
	assert.NoError(taskQueueIn.Save())

	distroQueueInfoOut, err := GetDistroQueueInfo(distroID)
	assert.NoError(err)
	assert.Equal(distroQueueInfoOut.Length, 8)
	assert.Len(distroQueueInfoOut.TaskGroupInfos, 1)
	assert.Equal(distroQueueInfoOut.TaskGroupInfos[0].Name, "taskGroupInfo1")
	assert.Equal(distroQueueInfoOut.TaskGroupInfos[0].Count, 8)
	assert.Equal(distroQueueInfoOut.TaskGroupInfos[0].ExpectedDuration, time.Duration(2600127105386))
}

func TestFindDuplicateEnqueuedTasks(t *testing.T) {
	const coll = TaskQueuesCollection
	makeTaskQueue := func(t *testing.T, distroID string, ids ...string) *TaskQueue {
		tq := &TaskQueue{Distro: distroID}
		for _, id := range ids {
			tq.Queue = append(tq.Queue, TaskQueueItem{Id: id})
		}
		require.NoError(t, tq.Save())
		return tq
	}
	for testName, testCase := range map[string]func(t *testing.T){
		"MatchesDuplicatesAcrossDifferentQueues": func(t *testing.T) {
			_ = makeTaskQueue(t, "d1", "task1", "task2", "task3")
			_ = makeTaskQueue(t, "d2", "task1", "task3", "task4", "task5", "task6")
			_ = makeTaskQueue(t, "d3", "task3")
			dups, err := FindDuplicateEnqueuedTasks(coll)
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
			dups, err := FindDuplicateEnqueuedTasks(coll)
			assert.NoError(t, err)
			assert.Empty(t, dups)
		},
		"DoesNotMatchEmptyQueues": func(t *testing.T) {
			_ = makeTaskQueue(t, "d1")
			dups, err := FindDuplicateEnqueuedTasks(coll)
			assert.NoError(t, err)
			assert.Empty(t, dups)
		},
		"DoesNotMatchAllUnique": func(t *testing.T) {
			_ = makeTaskQueue(t, "d1", "task1", "task2")
			_ = makeTaskQueue(t, "d2", "task3", "task4")
			dups, err := FindDuplicateEnqueuedTasks(coll)
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
