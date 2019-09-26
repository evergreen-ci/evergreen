package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	taskQueueTestConf = testutil.TestConfig()
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

func TestFindTask(t *testing.T) {
	assert := assert.New(t)

	q := &TaskQueue{
		Queue: []TaskQueueItem{
			{Id: "one", Group: "foo", Project: "a", Version: "b", BuildVariant: "a"},
			{Id: "two", Group: "bar", Project: "a", Version: "b", BuildVariant: "a"},
			{Id: "three", Project: "a", Version: "b", BuildVariant: "a"},
			{Id: "four", Project: "a", Version: "b", BuildVariant: "a"},
			{Id: "five", Group: "foo", Project: "aa", Version: "bb", BuildVariant: "a"},
			{Id: "six", Group: "bar", Project: "aa", Version: "bb", BuildVariant: "a"},
			{Id: "seven", Project: "aa", Version: "bb", BuildVariant: "a"},
			{Id: "eight", Project: "aa", Version: "bb", BuildVariant: "a"},
		},
	}

	// ensure that it's always the first task if the group name isn't specified
	assert.Equal("one", q.FindNextTask(TaskSpec{}).Id)
	assert.Equal("one", q.FindNextTask(TaskSpec{BuildVariant: "a"}).Id)
	assert.Equal("one", q.FindNextTask(TaskSpec{Project: "a"}).Id)
	assert.Equal("one", q.FindNextTask(TaskSpec{Version: "b"}).Id)
	assert.Equal("one", q.FindNextTask(TaskSpec{BuildVariant: "a", Project: "a"}).Id)
	assert.Equal("one", q.FindNextTask(TaskSpec{BuildVariant: "a", Version: "b"}).Id)
	assert.Equal("one", q.FindNextTask(TaskSpec{Project: "a", Version: "b"}).Id)

	// ensure that we can get the task groups that we expect
	assert.Equal("five", q.FindNextTask(TaskSpec{Group: "foo", Project: "aa", Version: "bb", BuildVariant: "a"}).Id)
	assert.Equal("one", q.FindNextTask(TaskSpec{Group: "foo", Project: "a", Version: "b", BuildVariant: "a"}).Id)
	assert.Equal("six", q.FindNextTask(TaskSpec{Group: "bar", Project: "aa", Version: "bb", BuildVariant: "a"}).Id)
	assert.Equal("two", q.FindNextTask(TaskSpec{Group: "bar", Project: "a", Version: "b", BuildVariant: "a"}).Id)
}

func TestBlockTaskGroupTasks(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(TaskQueuesCollection, task.Collection, ProjectRefCollection, VersionCollection))

	projectRef := &ProjectRef{Identifier: "a"}
	assert.Nil(projectRef.Insert())
	yml := `
task_groups:
- name: foo
  tasks:
  - task_id
  - one
tasks:
- name: task_id
- name: one
`
	v := Version{
		Id:        "b",
		Requester: evergreen.RepotrackerVersionRequester,
		Config:    yml,
	}
	require.NoError(v.Insert())
	tasks := []task.Task{
		{
			Id:                "task_id_1",
			DisplayName:       "task_id",
			TaskGroup:         "foo",
			TaskGroupMaxHosts: 1,
			BuildVariant:      "a",
			Version:           "b",
			Project:           "a",
			Revision:          "b",
			DistroId:          "distro_1",
		},
		{
			Id:                "one_1",
			DisplayName:       "one",
			TaskGroup:         "foo",
			TaskGroupMaxHosts: 1,
			BuildVariant:      "a",
			Version:           "b",
			Project:           "a",
			Revision:          "b",
			DistroId:          "distro_1",
		},
	}
	for _, t := range tasks {
		require.NoError(t.Insert())
	}
	assert.NoError(updateTaskQueue(
		"distro_1",
		[]TaskQueueItem{
			{
				Id: tasks[1].Id,
			},
		},
		DistroQueueInfo{
			Length: 1,
		},
	))
	queue, err := LoadTaskQueue("distro_1")
	assert.NoError(err)
	assert.Len(queue.Queue, 1)

	assert.NoError(BlockTaskGroupTasks("task_id_1"))
	found, err := task.FindOneId("one_1")
	assert.NoError(err)
	assert.Equal("task_id_1", found.DependsOn[0].TaskId)

	queue, err = LoadTaskQueue("distro_1")
	assert.NoError(err)
	assert.Nil(queue)
}

func TestBlockTaskGroupTasksFailsWithCircularDependencies(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(TaskQueuesCollection, task.Collection))

	tasks := []task.Task{
		{
			Id:                "task_id",
			TaskGroup:         "foo",
			TaskGroupMaxHosts: 1,
			BuildVariant:      "a",
			Version:           "b",
			DependsOn: []task.Dependency{
				{
					TaskId: "outside_group",
					Status: evergreen.TaskSucceeded,
				},
			},
		},
		{
			Id:           "one",
			TaskGroup:    "foo",
			Project:      "a",
			Version:      "b",
			BuildVariant: "a",
		},
		{
			Id:           "outside_group",
			Project:      "a",
			Version:      "b",
			BuildVariant: "a",
			DependsOn: []task.Dependency{
				{
					TaskId: "one",
					Status: evergreen.TaskSucceeded,
				},
			},
		},
	}
	for _, t := range tasks {
		require.NoError(t.Insert())
	}
	assert.Error(BlockTaskGroupTasks("task_id"))
	found, err := task.FindOneId("one")
	assert.NoError(err)
	assert.Empty(found.DependsOn)
}

func TestFindNextTaskEmptySpec(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(host.Collection, task.Collection))
	defer db.ClearCollections(host.Collection, task.Collection)

	hosts := []host.Host{}
	for i := 0; i < 10; i++ {
		hosts = append(hosts, host.Host{
			Id:                      fmt.Sprintf("host_%d", i),
			Status:                  "running",
			RunningTask:             fmt.Sprintf("task_id_%d", i),
			RunningTaskGroup:        "task_group",
			RunningTaskBuildVariant: "build_variant",
			RunningTaskVersion:      "task_version",
			RunningTaskProject:      "task_project",
		})
	}
	for _, h := range hosts {
		require.NoError(h.Insert())
	}
	tasks := []task.Task{}
	for i := 0; i < 10; i++ {
		tasks = append(tasks, task.Task{
			Id:           fmt.Sprintf("task_%d", i),
			TaskGroup:    "task_group",
			BuildVariant: "build_variant",
			Version:      "task_version",
			Project:      "task_project",
		})
	}
	queueTask := task.Task{
		Id:           "first_item",
		TaskGroup:    "task_group",
		BuildVariant: "build_variant",
		Version:      "task_version",
		Project:      "task_project",
	}
	require.NoError(queueTask.Insert())
	for _, t := range tasks {
		require.NoError(t.Insert())
	}

	// Return the task if the task group is empty.
	queue := TaskQueue{
		Queue: []TaskQueueItem{
			TaskQueueItem{Id: "first_item"},
		},
	}
	next := queue.FindNextTask(TaskSpec{})
	assert.NotNil(next)
	assert.Equal("first_item", next.Id)

	// Return a task if it's running on less than maxhosts
	queue = TaskQueue{
		Queue: []TaskQueueItem{
			TaskQueueItem{
				Id:            "first_item",
				Group:         "task_group",
				BuildVariant:  "build_variant",
				Version:       "task_version",
				Project:       "task_project",
				GroupMaxHosts: 100,
			},
		},
	}
	next = queue.FindNextTask(TaskSpec{})
	assert.NotNil(next)
	assert.Equal("first_item", next.Id)

	// Don't return a task if it's running on more than maxhosts
	queue.Queue[0].GroupMaxHosts = 1
	next = queue.FindNextTask(TaskSpec{})
	assert.Nil(next)

	// Check that all four fields must match to be in task group.
	// If all match, FindNextTask(TaskSpec{}) should return nil, because the task is
	// running on more than max_hosts. If any one does not match, FindNextTask(TaskSpec{})
	// should return not nil, because no hosts are running this group.
	//
	// All four match:
	queue.Queue[0].GroupMaxHosts = 1
	next = queue.FindNextTask(TaskSpec{})
	assert.Nil(next)
	// Group does not match.
	tmp := queue.Queue[0].Group
	queue.Queue[0].Group = "foo"
	next = queue.FindNextTask(TaskSpec{})
	assert.NotNil(next)
	assert.Equal("first_item", next.Id)
	queue.Queue[0].Group = tmp
	// BuildVariant does not match.
	tmp = queue.Queue[0].BuildVariant
	queue.Queue[0].BuildVariant = "foo"
	next = queue.FindNextTask(TaskSpec{})
	assert.NotNil(next)
	assert.Equal("first_item", next.Id)
	queue.Queue[0].BuildVariant = tmp
	// Version does not match.
	tmp = queue.Queue[0].Version
	queue.Queue[0].Version = "foo"
	next = queue.FindNextTask(TaskSpec{})
	assert.NotNil(next)
	assert.Equal("first_item", next.Id)
	queue.Queue[0].Version = tmp
	// Project does not match.
	queue.Queue[0].Project = "foo"
	next = queue.FindNextTask(TaskSpec{})
	assert.NotNil(next)
	assert.Equal("first_item", next.Id)
}

func TestFindNextTaskWithLastTask(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(host.Collection, task.Collection))
	defer db.ClearCollections(host.Collection, task.Collection)

	hosts := []host.Host{}
	for i := 0; i < 10; i++ {
		hosts = append(hosts, host.Host{
			Id:               fmt.Sprintf("host_%d", i),
			Status:           "running",
			LastTask:         fmt.Sprintf("task_id_%d", i),
			LastGroup:        "task_group",
			LastBuildVariant: "build_variant",
			LastVersion:      "task_version",
			LastProject:      "task_project",
		})
	}
	for _, h := range hosts {
		require.NoError(h.Insert())
	}
	tasks := []task.Task{}
	for i := 0; i < 10; i++ {
		tasks = append(tasks, task.Task{
			Id:           fmt.Sprintf("task_%d", i),
			TaskGroup:    "task_group",
			BuildVariant: "build_variant",
			Version:      "task_version",
			Project:      "task_project",
		})
	}
	queueTask := task.Task{
		Id:           "first_item",
		TaskGroup:    "task_group",
		BuildVariant: "build_variant",
		Version:      "task_version",
		Project:      "task_project",
	}
	require.NoError(queueTask.Insert())
	for _, t := range tasks {
		require.NoError(t.Insert())
	}

	queue := TaskQueue{
		Queue: []TaskQueueItem{
			TaskQueueItem{
				Id:            "first_item",
				Group:         "task_group",
				BuildVariant:  "build_variant",
				Version:       "task_version",
				Project:       "task_project",
				GroupMaxHosts: 1,
			},
		},
	}
	next := queue.FindNextTask(TaskSpec{})
	assert.Nil(next)
}

func TestTaskQueueGenerationTimes(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(TaskQueuesCollection))
	defer db.ClearCollections(TaskQueuesCollection)

	now := time.Now().Round(time.Millisecond).UTC()
	taskQueue := &TaskQueue{
		Distro:      "foo",
		GeneratedAt: now,
	}

	assert.NoError(db.Insert(TaskQueuesCollection, taskQueue))

	times, err := FindTaskQueueLastGenerationTimes()
	assert.NoError(err)
	assert.NotNil(times)
	assert.Len(times, 1)
	genTime, ok := times["foo"]
	assert.True(ok)
	assert.Equal(now, genTime)
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

	queue := NewTaskQueue(distro, tasks, DistroQueueInfo{})
	assert.Len(queue.Queue, 3)
	assert.NoError(queue.Save())
	otherQueue := NewTaskQueue(otherDistro, tasks, DistroQueueInfo{})
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
	defer db.ClearCollections(TaskQueuesCollection)

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
	assert.Len(taskQueueOut.DistroQueueInfo.TaskGroupInfos, 1)
	assert.Equal(taskQueueOut.DistroQueueInfo.TaskGroupInfos[0].Name, "taskGroupInfo1")
	assert.Equal(taskQueueOut.DistroQueueInfo.TaskGroupInfos[0].Count, 8)
	assert.Equal(taskQueueOut.DistroQueueInfo.TaskGroupInfos[0].ExpectedDuration, time.Duration(2600127105386))
}

func TestGetDistroQueueInfo(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(TaskQueuesCollection))
	defer db.ClearCollections(TaskQueuesCollection)

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
