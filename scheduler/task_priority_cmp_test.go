package scheduler

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskImportanceComparators(t *testing.T) {

	var taskComparator *CmpBasedTaskComparator
	var taskIds []string
	var tasks []task.Task

	Convey("When using the task importance comparators", t, func() {

		taskComparator = &CmpBasedTaskComparator{}

		taskIds = []string{"t1", "t2"}

		tasks = []task.Task{
			{Id: taskIds[0]},
			{Id: taskIds[1]},
		}

		Convey("the explicit priority comparator should prioritize a task"+
			" if its priority is higher", func() {

			tasks[0].Priority = 2
			tasks[1].Priority = 2

			cmpResult, err := byPriority(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			tasks[0].Priority = 2
			tasks[1].Priority = 1
			cmpResult, err = byPriority(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			tasks[0].Priority = 1
			tasks[1].Priority = 2
			cmpResult, err = byPriority(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)
		})

		Convey("all other things being equal, the longer"+
			"running tasks should be before tests", func() {

			tasks[0].TimeTaken = 20 * time.Minute
			tasks[1].TimeTaken = time.Hour

			taskComparator.previousTasksCache = map[string]task.Task{}
			taskComparator.previousTasksCache[tasks[0].Id] = tasks[0]
			taskComparator.previousTasksCache[tasks[1].Id] = tasks[1]

			result, err := byRuntime(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, -1)

			result, err = byRuntime(tasks[1], tasks[0], taskComparator)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, 1)

			result, err = byRuntime(tasks[0], tasks[0], taskComparator)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, 0)

			tasks[0].TimeTaken = time.Duration(0)
			taskComparator.previousTasksCache[tasks[0].Id] = tasks[0]

			result, err = byRuntime(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, 0)

			result, err = byRuntime(tasks[1], tasks[0], taskComparator)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, 0)
		})

		Convey("the dependent count comparator should prioritize a task"+
			" if its number of dependents is higher", func() {

			cmpResult, err := byNumDeps(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			tasks[0].NumDependents = 1
			cmpResult, err = byNumDeps(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			cmpResult, err = byNumDeps(tasks[1], tasks[0],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			tasks[1].NumDependents = 1
			cmpResult, err = byNumDeps(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

		})

		Convey("the commit order number comparator should prioritize a task"+
			" whose commit order number is higher, providing the tasks are"+
			" part of the same project and are commit tasks", func() {

			tasks[0].Requester = evergreen.RepotrackerVersionRequester
			tasks[1].Requester = evergreen.RepotrackerVersionRequester

			cmpResult, err := byAge(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			tasks[0].RevisionOrderNumber = 1
			cmpResult, err = byAge(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			cmpResult, err = byAge(tasks[1], tasks[0],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			tasks[0].Project = "project"
			cmpResult, err = byAge(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

		})

		Convey("the create time comparator should prioritize a task whose"+
			" create time is higher, providing the tasks are from different"+
			" projects", func() {

			tasks[0].Requester = evergreen.PatchVersionRequester
			tasks[1].Requester = evergreen.PatchVersionRequester

			cmpResult, err := byAge(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			// change one create time - should still be zero since the
			// projects are the same
			tasks[0].CreateTime = time.Now()
			cmpResult, err = byAge(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			tasks[0].Project = "project"
			cmpResult, err = byAge(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			cmpResult, err = byAge(tasks[1], tasks[0], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			tasks[1].CreateTime = tasks[0].CreateTime
			cmpResult, err = byAge(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			cmpResult, err = byAge(tasks[1], tasks[0], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)
		})

		Convey("the recent failure comparator should prioritize a task"+
			" whose last execution failed", func() {

			prevTaskIds := []string{"pt1", "pt2"}

			prevTasks := map[string]task.Task{
				taskIds[0]: {Id: prevTaskIds[0]},
				taskIds[1]: {Id: prevTaskIds[1]},
			}

			taskComparator.previousTasksCache = prevTasks

			cmpResult, err := byRecentlyFailing(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			pt1 := taskComparator.previousTasksCache[taskIds[0]]
			pt1.Status = evergreen.TaskFailed
			taskComparator.previousTasksCache[taskIds[0]] = pt1

			cmpResult, err = byRecentlyFailing(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			cmpResult, err = byRecentlyFailing(tasks[1], tasks[0],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			pt2 := taskComparator.previousTasksCache[taskIds[1]]
			pt2.Status = evergreen.TaskFailed
			taskComparator.previousTasksCache[taskIds[1]] = pt2

			cmpResult, err = byRecentlyFailing(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

		})

		Convey("the similar failing comparator should prioritize a task if "+
			"it has more similar failing tasks", func() {
			prlTaskIds := []string{"t1", "t2"}

			similarFailingCountMap := map[string]int{
				prlTaskIds[0]: 3,
				prlTaskIds[1]: 3,
			}

			taskComparator.similarFailingCount = similarFailingCountMap

			cmpResult, err := bySimilarFailing(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			taskComparator.similarFailingCount[prlTaskIds[0]] = 4

			cmpResult, err = bySimilarFailing(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			taskComparator.similarFailingCount[prlTaskIds[1]] = 5
			cmpResult, err = bySimilarFailing(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			taskComparator.similarFailingCount[prlTaskIds[0]] = 5

			cmpResult, err = bySimilarFailing(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

		})
	})

}

func TestByTaskGroupOrder(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(version.Collection))
	defer db.ClearCollections(version.Collection)

	taskComparator := &CmpBasedTaskComparator{}
	tasks := []task.Task{
		{Id: "t1"},
		{Id: "t2"},
	}

	// empty task groups
	result, err := byTaskGroupOrder(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(0, result)
	tasks[0].TaskGroup = "example_task_group"
	result, err = byTaskGroupOrder(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(0, result)
	tasks[0].TaskGroup = ""
	tasks[1].TaskGroup = "example_task_group"
	result, err = byTaskGroupOrder(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(0, result)

	// mismatched task groups
	tasks[0].TaskGroup = "example_task_group"
	tasks[1].TaskGroup = "another_task_group"
	result, err = byTaskGroupOrder(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(0, result)

	// mismatched builds
	tasks[0].TaskGroup = "example_task_group"
	tasks[1].TaskGroup = "example_task_group"
	tasks[0].BuildId = "build_id"
	tasks[1].BuildId = "another_build_id"
	result, err = byTaskGroupOrder(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(0, result)

	// t1 is earlier
	tasks[0].TaskGroup = "example_task_group"
	tasks[1].TaskGroup = "example_task_group"
	tasks[0].BuildId = "build_id"
	tasks[1].BuildId = "build_id"
	tasks[0].Version = "version_id"
	tasks[1].Version = "version_id"
	tasks[0].DisplayName = "first_task"
	tasks[1].DisplayName = "another_task"
	yml := `
task_groups:
- name: example_task_group
  tasks:
  - first_task
  - another_task
`
	v := &version.Version{
		Id:     "version_id",
		Config: yml,
	}
	require.NoError(v.Insert())
	result, err = byTaskGroupOrder(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(1, result)

	// t2 is earlier
	require.NoError(db.ClearCollections(version.Collection))
	yml = `
task_groups:
- name: example_task_group
  tasks:
  - another_task
  - first_task
`
	v.Config = yml
	require.NoError(v.Insert())
	result, err = byTaskGroupOrder(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(-1, result)
}
