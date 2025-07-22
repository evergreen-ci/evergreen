package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	byPriorityCmp       = &byPriority{}
	byRuntimeCmp        = &byRuntime{}
	byNumDepsCmp        = &byNumDeps{}
	byAgeCmp            = &byAge{}
	byTaskGroupOrderCmp = &byTaskGroupOrder{}
	byGenerateTasksCmp  = &byGenerateTasks{}
	byCommitQueueCmp    = &byCommitQueue{}
)

func TestTaskImportanceComparators(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var taskComparator *CmpBasedTaskComparator
	var taskIds []string
	var tasks []task.Task

	Convey("When using the task importance comparators", t, func() {

		taskComparator = NewCmpBasedTaskComparator(ctx, "runtime-id")

		taskIds = []string{"t1", "t2"}

		tasks = []task.Task{
			{Id: taskIds[0]},
			{Id: taskIds[1]},
		}

		Convey("the explicit priority comparator should prioritize a task"+
			" if its priority is higher", func() {

			tasks[0].Priority = 2
			tasks[1].Priority = 2

			cmpResult, _, err := byPriorityCmp.compare(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			tasks[0].Priority = 2
			tasks[1].Priority = 1
			cmpResult, _, err = byPriorityCmp.compare(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			tasks[0].Priority = 1
			tasks[1].Priority = 2
			cmpResult, _, err = byPriorityCmp.compare(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)
		})

		Convey("all other things being equal, the longer"+
			"running tasks should be before tests", func() {

			result, _, err := byRuntimeCmp.compare(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, 0)

			tasks[0].ExpectedDuration = 20 * time.Minute
			tasks[1].ExpectedDuration = time.Hour

			result, _, err = byRuntimeCmp.compare(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, -1)

			result, _, err = byRuntimeCmp.compare(tasks[1], tasks[0], taskComparator)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, 1)

			result, _, err = byRuntimeCmp.compare(tasks[0], tasks[0], taskComparator)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, 0)

			tasks[0].ExpectedDuration = time.Duration(1)
			tasks[0].DurationPrediction.Value = time.Duration(1)
			tasks[0].DurationPrediction.TTL = time.Hour

			result, _, err = byRuntimeCmp.compare(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, -1)

			result, _, err = byRuntimeCmp.compare(tasks[1], tasks[0], taskComparator)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, 1)
		})

		Convey("the dependent count comparator should prioritize a task"+
			" if its number of dependents is higher", func() {

			cmpResult, _, err := byNumDepsCmp.compare(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			tasks[0].NumDependents = 1
			cmpResult, _, err = byNumDepsCmp.compare(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			cmpResult, _, err = byNumDepsCmp.compare(tasks[1], tasks[0],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			tasks[1].NumDependents = 1
			cmpResult, _, err = byNumDepsCmp.compare(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

		})

		Convey("the commit order number comparator should prioritize a task"+
			" whose commit order number is higher, providing the tasks are"+
			" part of the same project and are commit tasks", func() {

			tasks[0].Requester = evergreen.RepotrackerVersionRequester
			tasks[1].Requester = evergreen.RepotrackerVersionRequester

			cmpResult, _, err := byAgeCmp.compare(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			tasks[0].RevisionOrderNumber = 1
			cmpResult, _, err = byAgeCmp.compare(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			cmpResult, _, err = byAgeCmp.compare(tasks[1], tasks[0],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			tasks[0].Project = "project"
			cmpResult, _, err = byAgeCmp.compare(tasks[0], tasks[1],
				taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

		})

		Convey("the create time comparator should prioritize a task whose"+
			" ingest time is higher, providing the tasks are from different"+
			" projects", func() {

			tasks[0].Requester = evergreen.PatchVersionRequester
			tasks[1].Requester = evergreen.PatchVersionRequester

			cmpResult, _, err := byAgeCmp.compare(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			// change one ingest time - should still be zero since the
			// projects are the same
			tasks[0].IngestTime = time.Now()
			cmpResult, _, err = byAgeCmp.compare(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			tasks[0].Project = "project"
			cmpResult, _, err = byAgeCmp.compare(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			cmpResult, _, err = byAgeCmp.compare(tasks[1], tasks[0], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			tasks[1].IngestTime = tasks[0].IngestTime
			cmpResult, _, err = byAgeCmp.compare(tasks[0], tasks[1], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			cmpResult, _, err = byAgeCmp.compare(tasks[1], tasks[0], taskComparator)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)
		})
	})
}

func TestByTaskGroupOrder(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(model.VersionCollection))
	defer func() {
		assert.NoError(db.ClearCollections(model.VersionCollection))
	}()

	taskComparator := &CmpBasedTaskComparator{}
	tasks := []task.Task{
		{Id: "t1"},
		{Id: "t2"},
	}

	// empty task groups
	result, _, err := byTaskGroupOrderCmp.compare(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(0, result)
	tasks[0].TaskGroup = "example_task_group"
	result, _, err = byTaskGroupOrderCmp.compare(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(1, result)
	tasks[0].TaskGroup = ""
	tasks[1].TaskGroup = "example_task_group"
	result, _, err = byTaskGroupOrderCmp.compare(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(-1, result)

	// mismatched task groups
	tasks[0].TaskGroup = "example_task_group"
	tasks[1].TaskGroup = "another_task_group"
	result, _, err = byTaskGroupOrderCmp.compare(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(-1, result)

	// mismatched builds
	tasks[0].TaskGroup = "example_task_group"
	tasks[1].TaskGroup = "example_task_group"
	tasks[0].BuildId = "build_id"
	tasks[1].BuildId = "another_build_id"
	result, _, err = byTaskGroupOrderCmp.compare(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(-1, result)

	// t1 is earlier
	tasks[0].TaskGroup = "example_task_group"
	tasks[1].TaskGroup = "example_task_group"
	tasks[0].BuildId = "build_id"
	tasks[1].BuildId = "build_id"
	tasks[0].Version = "version_id"
	tasks[1].Version = "version_id"
	tasks[0].DisplayName = "first_task"
	tasks[1].DisplayName = "another_task"
	tasks[0].TaskGroupOrder = 1
	tasks[1].TaskGroupOrder = 2

	v := &model.Version{
		Id: "version_id",
	}
	require.NoError(v.Insert(t.Context()))
	taskComparator.tasks = tasks
	taskComparator.versions = map[string]model.Version{"version_id": *v}
	result, _, err = byTaskGroupOrderCmp.compare(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(1, result)

	// t2 is earlier
	tasks[0].TaskGroupOrder = 2
	tasks[1].TaskGroupOrder = 1
	result, _, err = byTaskGroupOrderCmp.compare(tasks[0], tasks[1], taskComparator)
	assert.NoError(err)
	assert.Equal(-1, result)
}

func TestPrioritizeTasksWithSameTaskGroupsAndDifferentBuilds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(model.VersionCollection, task.Collection))
	defer func() {
		require.NoError(db.ClearCollections(model.VersionCollection, task.Collection))
	}()

	v := model.Version{
		Id: "version_1",
	}
	versions := map[string]model.Version{"version_1": v}
	require.NoError(v.Insert(t.Context()))
	v = model.Version{
		Id: "version_2",
	}
	versions["version_2"] = v
	require.NoError(v.Insert(t.Context()))
	tasks := task.Tasks{
		{
			Id:             "task_1",
			BuildId:        "build_1",
			DisplayName:    "another_task",
			Version:        "version_1",
			Requester:      evergreen.PatchVersionRequester,
			TaskGroup:      "example_task_group",
			TaskGroupOrder: 2,
		},
		{
			Id:             "task_2",
			BuildId:        "build_2",
			DisplayName:    "first_task",
			Version:        "version_1",
			Requester:      evergreen.PatchVersionRequester,
			TaskGroup:      "example_task_group",
			TaskGroupOrder: 1,
		},
		{
			Id:             "task_3",
			BuildId:        "build_2",
			DisplayName:    "another_task",
			Version:        "version_1",
			Requester:      evergreen.PatchVersionRequester,
			TaskGroup:      "example_task_group",
			TaskGroupOrder: 2,
		},
		{
			Id:             "task_4",
			BuildId:        "build_1",
			DisplayName:    "first_task",
			Version:        "version_1",
			Requester:      evergreen.PatchVersionRequester,
			TaskGroup:      "example_task_group",
			TaskGroupOrder: 1,
		},
	}
	require.NoError(tasks.Insert(t.Context()))

	prioritizer := &CmpBasedTaskPrioritizer{}
	sorted, _, err := prioritizer.PrioritizeTasks(ctx, "distro", tasks.Export(), versions)
	require.NoError(err)
	assert.Equal("task_4", sorted[0].Id)
	assert.Equal("task_1", sorted[1].Id)
	assert.Equal("task_2", sorted[2].Id)
	assert.Equal("task_3", sorted[3].Id)
}

func TestTaskGroupsNotOutOfOrderFromOtherComparators(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(model.VersionCollection))
	defer func() {
		require.NoError(db.ClearCollections(model.VersionCollection))
	}()

	v := model.Version{
		Id: "version_1",
	}
	versions := map[string]model.Version{"version_1": v}
	require.NoError(v.Insert(t.Context()))
	v = model.Version{
		Id: "version_2",
	}
	versions["version_2"] = v
	require.NoError(v.Insert(t.Context()))

	tasks := []task.Task{
		{
			Id:             "task_1",
			BuildId:        "build_1",
			DisplayName:    "later_task",
			Version:        "version_1",
			Requester:      evergreen.PatchVersionRequester,
			TaskGroupOrder: 2,
			TaskGroup:      "example_task_group",
			Priority:       4,
		},
		{
			Id:          "task_3",
			BuildId:     "build_1",
			DisplayName: "third_task",
			Version:     "version_1",
			Requester:   evergreen.PatchVersionRequester,
			Priority:    1,
		},
		{
			Id:             "task_2",
			BuildId:        "build_1",
			DisplayName:    "earlier_task",
			Version:        "version_1",
			Requester:      evergreen.PatchVersionRequester,
			TaskGroup:      "example_task_group",
			TaskGroupOrder: 1,
			Priority:       0,
		},
	}

	prioritizer := &CmpBasedTaskPrioritizer{}
	sorted, _, err := prioritizer.PrioritizeTasks(ctx, "distro", tasks, versions)
	assert.NoError(err)
	list := []string{}
	for _, t := range sorted {
		list = append(list, t.DisplayName)
	}
	for _, d := range list {
		if d == "earlier_task" {
			break
		}
		if d == "later_task" {
			assert.Fail("later_task appeared before earlier_task")
		}
	}
}

func TestByGenerateTasks(t *testing.T) {
	assert := assert.New(t)
	tasks := []task.Task{
		{
			Id:           "task_1",
			GenerateTask: true,
		},
		{
			Id:           "task_2",
			GenerateTask: false,
		},
		{
			Id:           "task_3",
			GenerateTask: true,
		},
		{
			Id:           "task_4",
			GenerateTask: false,
		},
	}
	comparator := &CmpBasedTaskComparator{}
	c, _, err := byGenerateTasksCmp.compare(tasks[0], tasks[2], comparator)
	assert.NoError(err)
	assert.Equal(0, c)
	c, _, err = byGenerateTasksCmp.compare(tasks[1], tasks[3], comparator)
	assert.NoError(err)
	assert.Equal(0, c)
	c, _, err = byGenerateTasksCmp.compare(tasks[0], tasks[1], comparator)
	assert.NoError(err)
	assert.Equal(1, c)
	c, _, err = byGenerateTasksCmp.compare(tasks[2], tasks[3], comparator)
	assert.NoError(err)
	assert.Equal(1, c)
	c, _, err = byGenerateTasksCmp.compare(tasks[1], tasks[0], comparator)
	assert.NoError(err)
	assert.Equal(-1, c)
	c, _, err = byGenerateTasksCmp.compare(tasks[3], tasks[2], comparator)
	assert.NoError(err)
	assert.Equal(-1, c)
}

func TestByCommitQueue(t *testing.T) {
	tasks := []task.Task{
		{Id: "t0", Version: "v0"},
		{Id: "t1", Version: "v0"},
		{Id: "t2", Version: "v1"},
		{Id: "t3", Version: "v1"},
	}
	comparator := &CmpBasedTaskComparator{versions: map[string]model.Version{
		"v0": {Requester: evergreen.GithubMergeRequester},
		"v1": {Requester: evergreen.PatchVersionRequester},
	}}

	c, _, err := byCommitQueueCmp.compare(tasks[0], tasks[1], comparator)
	assert.NoError(t, err)
	assert.Equal(t, 0, c)

	c, _, err = byCommitQueueCmp.compare(tasks[2], tasks[3], comparator)
	assert.NoError(t, err)
	assert.Equal(t, 0, c)

	c, _, err = byCommitQueueCmp.compare(tasks[1], tasks[2], comparator)
	assert.NoError(t, err)
	assert.Equal(t, 1, c)

	c, _, err = byCommitQueueCmp.compare(tasks[2], tasks[1], comparator)
	assert.NoError(t, err)
	assert.Equal(t, -1, c)
}
