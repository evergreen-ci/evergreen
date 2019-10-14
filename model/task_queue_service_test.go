package model

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type taskDAGDispatchServiceSuite struct {
	suite.Suite

	taskQueue TaskQueue
}

func TestTaskDAGDispatchServiceSuite(t *testing.T) {
	suite.Run(t, new(taskDAGDispatchServiceSuite))
}

func (s *taskDAGDispatchServiceSuite) TestOutsideTasksWithTaskGroupDependencies() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(db.ClearCollections(host.Collection))
	distroID := "distro_1"
	items := []TaskQueueItem{}

	t1 := task.Task{
		Id:                "taskgroup_task1",
		BuildId:           "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:         "tg_compile_and_test",
		TaskGroupMaxHosts: 1,
		StartTime:         util.ZeroTime,
		BuildVariant:      "archlinux",
		Version:           "5d8cd23da4cf4747f4210333",
		Project:           "genny",
		Activated:         true,
		ActivatedBy:       "",
		DistroId:          "archlinux-test",
		Requester:         "github_pull_request",
		Status:            evergreen.TaskUndispatched,
		Revision:          "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		TaskGroupOrder:    4,
	}
	item1 := TaskQueueItem{
		Id:                  "taskgroup_task1",
		IsDispatched:        false,
		Group:               "tg_compile_and_test",
		GroupMaxHosts:       1,
		Version:             "5d8cd23da4cf4747f4210333",
		BuildVariant:        "archlinux",
		RevisionOrderNumber: 261,
		Requester:           "github_pull_request",
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		Project:             "genny",
		GroupIndex:          4,
	}

	//////////////////////////////////////////////////////////////////////////////

	t2 := task.Task{
		Id:                  "taskgroup_task2",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           util.ZeroTime,
		BuildVariant:        "archlinux",
		Version:             "5d8cd23da4cf4747f4210333",
		Project:             "genny",
		Activated:           true,
		ActivatedBy:         "",
		DistroId:            "archlinux-test",
		Requester:           "github_pull_request",
		Status:              evergreen.TaskUndispatched,
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		RevisionOrderNumber: 261,
		DependsOn:           []task.Dependency{},
		TaskGroupOrder:      1,
	}
	item2 := TaskQueueItem{
		Id:                  "taskgroup_task2",
		IsDispatched:        false,
		Group:               "tg_compile_and_test",
		GroupMaxHosts:       1,
		Version:             "5d8cd23da4cf4747f4210333",
		BuildVariant:        "archlinux",
		RevisionOrderNumber: 261,
		Requester:           "github_pull_request",
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		Project:             "genny",
		Dependencies:        []string{},
		GroupIndex:          1,
	}

	//////////////////////////////////////////////////////////////////////////////

	t3 := task.Task{
		Id:                  "taskgroup_task3",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           util.ZeroTime,
		BuildVariant:        "archlinux",
		Version:             "5d8cd23da4cf4747f4210333",
		Project:             "genny",
		Activated:           true,
		ActivatedBy:         "",
		DistroId:            "archlinux-test",
		Requester:           "github_pull_request",
		Status:              evergreen.TaskUndispatched,
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		RevisionOrderNumber: 261,
		TaskGroupOrder:      3,
	}
	item3 := TaskQueueItem{
		Id:                  "taskgroup_task3",
		IsDispatched:        false,
		Group:               "tg_compile_and_test",
		GroupMaxHosts:       1,
		Version:             "5d8cd23da4cf4747f4210333",
		BuildVariant:        "archlinux",
		RevisionOrderNumber: 261,
		Requester:           "github_pull_request",
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		Project:             "genny",
		GroupIndex:          3,
	}

	//////////////////////////////////////////////////////////////////////////////

	t4 := task.Task{
		Id:                  "taskgroup_task4",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           util.ZeroTime,
		BuildVariant:        "archlinux",
		Version:             "5d8cd23da4cf4747f4210333",
		Project:             "genny",
		Activated:           true,
		ActivatedBy:         "",
		DistroId:            "archlinux-test",
		Requester:           "github_pull_request",
		Status:              evergreen.TaskUndispatched,
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		RevisionOrderNumber: 261,
		TaskGroupOrder:      2,
	}
	item4 := TaskQueueItem{
		Id:                  "taskgroup_task4",
		IsDispatched:        false,
		Group:               "tg_compile_and_test",
		GroupMaxHosts:       1,
		Version:             "5d8cd23da4cf4747f4210333",
		BuildVariant:        "archlinux",
		RevisionOrderNumber: 261,
		Requester:           "github_pull_request",
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		Project:             "genny",
		GroupIndex:          2,
	}

	//////////////////////////////////////////////////////////////////////////////

	t5 := task.Task{
		Id:                  "external_task5",
		BuildId:             "build_1",
		TaskGroup:           "",
		TaskGroupMaxHosts:   0,
		StartTime:           util.ZeroTime,
		BuildVariant:        "archlinux",
		Version:             "version_1",
		Project:             "project_1",
		Activated:           true,
		ActivatedBy:         "",
		DistroId:            "archlinux-test",
		Requester:           "github_pull_request",
		Status:              evergreen.TaskUndispatched,
		Revision:            "revision_1",
		RevisionOrderNumber: 262,
		DependsOn: []task.Dependency{
			task.Dependency{
				TaskId:       "taskgroup_task3",
				Status:       "success",
				Unattainable: false,
			},
		},
	}
	item5 := TaskQueueItem{
		Id:                  "external_task5",
		IsDispatched:        false,
		Group:               "",
		GroupMaxHosts:       0,
		Version:             "version_1",
		BuildVariant:        "archlinux",
		RevisionOrderNumber: 262,
		Requester:           "github_pull_request",
		Revision:            "revision_1",
		Project:             "project_1",
		Dependencies:        []string{"taskgroup_task3"},
	}

	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())
	s.Require().NoError(t4.Insert())
	s.Require().NoError(t5.Insert())
	// items = append(items, item1, item2, item3, item4, item5)
	items = append(items, item5, item1, item2, item3, item4)

	s.taskQueue = TaskQueue{
		Distro: distroID,
		Queue:  items,
	}

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.Equal("distro_1", service.distroID)
	s.Equal(60*time.Second, service.ttl)
	s.NotEqual(util.ZeroTime, service.lastUpdated)

	spec := TaskSpec{}

	// 3 successive calls (regardless of the TaskSpec passed) will dispatch 3 task group tasks, per TaskGroupOrder.
	next := service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("taskgroup_task2", next.Id) // TaskGroupOrder: 1
	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("taskgroup_task4", next.Id) // TaskGroupOrder: 2
	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("taskgroup_task3", next.Id) // TaskGroupOrder: 3

	// "taskgroup_task3" completes with "status": evergreen.TaskSucceeded.
	err = setTaskStatus("taskgroup_task3", evergreen.TaskSucceeded)
	s.Require().NoError(err)

	// Fake a refresh of the in-memory queue.
	items = []TaskQueueItem{}
	items = append(items, item5, item1)
	s.taskQueue.Queue = items

	err = service.rebuild(s.taskQueue.Queue)
	s.Require().NoError(err)

	// "external_task5" can now be dispatched as its dependency "taskgroup_task3" has completed successfully.
	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("external_task5", next.Id)

	// The final task group task "taskgroup_task1" is dispatched
	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("taskgroup_task1", next.Id)

	// There are no more tasks to dispatch.
	next = service.FindNextTask(spec)
	s.Require().Nil(next)
	next = service.FindNextTask(spec)
	s.Require().Nil(next)
	next = service.FindNextTask(spec)
	s.Require().Nil(next)
}

func (s *taskDAGDispatchServiceSuite) TestIntraTaskGroupDependencies() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(db.ClearCollections(host.Collection))
	distroID := "distro_1"
	items := []TaskQueueItem{}

	// db.tasks.find({"build_id": "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10"}).pretty()

	t1 := task.Task{
		Id:                  "task1",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           util.ZeroTime,
		BuildVariant:        "archlinux",
		Version:             "5d8cd23da4cf4747f4210333",
		Project:             "genny",
		Activated:           true,
		ActivatedBy:         "",
		DistroId:            "archlinux-test",
		Requester:           "github_pull_request",
		Status:              evergreen.TaskUndispatched,
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		RevisionOrderNumber: 261,
		DisplayName:         "t_cmake_test",
		DependsOn: []task.Dependency{
			task.Dependency{
				TaskId:       "task2",
				Status:       "success",
				Unattainable: false,
			},
			task.Dependency{
				TaskId:       "task4",
				Status:       "success",
				Unattainable: false,
			},
			task.Dependency{
				TaskId:       "task3",
				Status:       "success",
				Unattainable: false,
			},
		},
		// TaskGroupOrder:      4,
	}
	item1 := TaskQueueItem{
		Id:                  "task1",
		IsDispatched:        false,
		DisplayName:         "t_cmake_test",
		Group:               "tg_compile_and_test",
		GroupMaxHosts:       1,
		Version:             "5d8cd23da4cf4747f4210333",
		BuildVariant:        "archlinux",
		RevisionOrderNumber: 261,
		Requester:           "github_pull_request",
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		Project:             "genny",
		Dependencies: []string{
			"task2",
			"task4",
			"task3",
		},
		// GroupIndex:          4,
	}

	//////////////////////////////////////////////////////////////////////////////

	t2 := task.Task{
		Id:                  "task2",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           util.ZeroTime,
		BuildVariant:        "archlinux",
		Version:             "5d8cd23da4cf4747f4210333",
		Project:             "genny",
		Activated:           true,
		ActivatedBy:         "",
		DistroId:            "archlinux-test",
		Requester:           "github_pull_request",
		Status:              evergreen.TaskUndispatched,
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		RevisionOrderNumber: 261,
		DisplayName:         "t_compile",
		DependsOn:           []task.Dependency{},
		// TaskGroupOrder:      1,
	}
	item2 := TaskQueueItem{
		Id:                  "task2",
		IsDispatched:        false,
		DisplayName:         "t_compile",
		Group:               "tg_compile_and_test",
		GroupMaxHosts:       1,
		Version:             "5d8cd23da4cf4747f4210333",
		BuildVariant:        "archlinux",
		RevisionOrderNumber: 261,
		Requester:           "github_pull_request",
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		Project:             "genny",
		Dependencies:        []string{},
		// GroupIndex:          1,
	}

	//////////////////////////////////////////////////////////////////////////////

	t3 := task.Task{
		Id:                  "task3",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           util.ZeroTime,
		BuildVariant:        "archlinux",
		Version:             "5d8cd23da4cf4747f4210333",
		Project:             "genny",
		Activated:           true,
		ActivatedBy:         "",
		DistroId:            "archlinux-test",
		Requester:           "github_pull_request",
		Status:              evergreen.TaskUndispatched,
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		RevisionOrderNumber: 261,
		DisplayName:         "t_lint_workloads",
		DependsOn: []task.Dependency{
			task.Dependency{
				TaskId:       "task2",
				Status:       "success",
				Unattainable: false,
			},
			task.Dependency{
				TaskId:       "task4",
				Status:       "success",
				Unattainable: false,
			},
		},
		// TaskGroupOrder:      3,
	}
	item3 := TaskQueueItem{
		Id:                  "task3",
		IsDispatched:        false,
		DisplayName:         "t_lint_workloads",
		Group:               "tg_compile_and_test",
		GroupMaxHosts:       1,
		Version:             "5d8cd23da4cf4747f4210333",
		BuildVariant:        "archlinux",
		RevisionOrderNumber: 261,
		Requester:           "github_pull_request",
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		Project:             "genny",
		Dependencies: []string{
			"task2",
			"task4",
		},
		// GroupIndex:          3,
	}

	//////////////////////////////////////////////////////////////////////////////

	t4 := task.Task{
		Id:                  "task4",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           util.ZeroTime,
		BuildVariant:        "archlinux",
		Version:             "5d8cd23da4cf4747f4210333",
		Project:             "genny",
		Activated:           true,
		ActivatedBy:         "",
		DistroId:            "archlinux-test",
		Requester:           "github_pull_request",
		Status:              evergreen.TaskUndispatched,
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		RevisionOrderNumber: 261,
		DisplayName:         "t_python_test",
		DependsOn: []task.Dependency{
			task.Dependency{
				TaskId:       "task2",
				Status:       "success",
				Unattainable: false,
			},
		},
		// TaskGroupOrder:      2,
	}
	item4 := TaskQueueItem{
		Id:                  "task4",
		IsDispatched:        false,
		DisplayName:         "t_python_test",
		Group:               "tg_compile_and_test",
		GroupMaxHosts:       1,
		Version:             "5d8cd23da4cf4747f4210333",
		BuildVariant:        "archlinux",
		RevisionOrderNumber: 261,
		Requester:           "github_pull_request",
		Revision:            "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		Project:             "genny",
		Dependencies: []string{
			"task2",
		},
		// GroupIndex:          2,
	}

	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())
	s.Require().NoError(t4.Insert())
	items = append(items, item1, item2, item3, item4)

	s.taskQueue = TaskQueue{
		Distro: distroID,
		Queue:  items,
	}

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.Equal("distro_1", service.distroID)
	s.Equal(60*time.Second, service.ttl)
	s.NotEqual(util.ZeroTime, service.lastUpdated)

	spec := TaskSpec{}

	// Only "task2" can be dispatched - the other 3 tasks cannot be dispatched as they all have unmet dependencies.
	next := service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("task2", next.Id)
	next = service.FindNextTask(spec)
	s.Require().Nil(next)
	next = service.FindNextTask(spec)
	s.Require().Nil(next)
	next = service.FindNextTask(spec)
	s.Require().Nil(next)

	// "task2" completes with "status": evergreen.TaskSucceeded.
	err = setTaskStatus("task2", evergreen.TaskSucceeded)
	s.Require().NoError(err)

	// Fake a refresh of the in-memory queue.
	items = []TaskQueueItem{}
	items = append(items, item1, item3, item4)
	s.taskQueue.Queue = items

	err = service.rebuild(s.taskQueue.Queue)
	s.Require().NoError(err)

	// Only "task4" can be dispatched - the other 2 tasks cannot be dispatched as they have unmet dependencies.
	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("task4", next.Id)
	next = service.FindNextTask(spec)
	s.Require().Nil(next)
	next = service.FindNextTask(spec)
	s.Require().Nil(next)

	// "task4" completes with "status": evergreen.TaskSucceeded
	err = setTaskStatus("task4", evergreen.TaskSucceeded)
	s.Require().NoError(err)

	// Fake a refresh of the in-memory queue.
	items = []TaskQueueItem{}
	items = append(items, item1, item3)
	s.taskQueue.Queue = items

	err = service.rebuild(s.taskQueue.Queue)
	s.Require().NoError(err)

	// Only "task3" can be dispatched - the remaining task cannot be dispatched as it has an unmet dependency.
	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("task3", next.Id)
	next = service.FindNextTask(spec)
	s.Require().Nil(next)

	// "task4" completes with "status": evergreen.TaskSucceeded
	err = setTaskStatus("task3", evergreen.TaskSucceeded)
	s.Require().NoError(err)

	// Fake a refresh of the in-memory queue.
	items = []TaskQueueItem{}
	items = append(items, item1)
	s.taskQueue.Queue = items

	err = service.rebuild(s.taskQueue.Queue)
	s.Require().NoError(err)

	// Finally, "task1" can be dispatched - all 3 of its dependencies have been satisfied.
	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("task1", next.Id)
	next = service.FindNextTask(spec)
	s.Require().Nil(next)
}

func (s *taskDAGDispatchServiceSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(db.ClearCollections(host.Collection))
	items := []TaskQueueItem{}
	var group string
	var variant string
	var version string
	var maxHosts int
	project := "project_1"
	distroID := "distro_1"

	for i := 0; i < 100; i++ {
		dependencies := []string{}
		if i%5 == 0 { // no group
			group = ""
			variant = "variant_1"
			version = "version_1"
			maxHosts = 0

			if i > 30 && i < 50 {
				// Adding dependency to task.Id 40 from task.Id 35
				// Adding dependency to task.Id 45 from task.Id 40
				// Adding dependency to task.Id 50 from task.Id 45
				dependencies = append(dependencies, strconv.Itoa(i+5))
			}
			if i > 60 && i < 80 {
				// Adding dependency to task.Id 70 from task.Id 65
				// Adding dependency to task.Id 75 from task.Id 70
				// Adding dependency to task.Id 80 from task.Id 75
				dependencies = append(dependencies, strconv.Itoa(i+5))
			}
		} else if i%5 == 1 { // "group_1_variant_1_project_1_version_1"
			group = "group_1"
			variant = "variant_1"
			version = "version_1"
			maxHosts = 1
		} else if i%5 == 2 { // "group_2_variant_1_project_1_version_1"
			group = "group_2"
			variant = "variant_1"
			version = "version_1"
			maxHosts = 2
		} else if i%5 == 3 { // "group_1_variant_2_project_1_version_1"
			group = "group_1"
			variant = "variant_2"
			version = "version_1"
			maxHosts = 2
		} else if i%5 == 4 { // "group_1_variant_1_project_1_version_2"
			group = "group_1"
			variant = "variant_1"
			version = "version_2"
			maxHosts = 2
		}

		ID := fmt.Sprintf("%d", i)
		items = append(items, TaskQueueItem{
			Id:            ID,
			Group:         group,
			BuildVariant:  variant,
			Version:       version,
			GroupMaxHosts: maxHosts,
			Project:       project,
			Dependencies:  dependencies,
		})

		dependsOn := []task.Dependency{}
		for i := range dependencies {
			d := task.Dependency{
				TaskId:       dependencies[i],
				Status:       "success",
				Unattainable: false,
			}
			dependsOn = append(dependsOn, d)
		}

		t := task.Task{
			Id:                ID,
			DistroId:          distroID,
			StartTime:         util.ZeroTime,
			TaskGroup:         group,
			BuildVariant:      variant,
			Version:           version,
			TaskGroupMaxHosts: maxHosts,
			Project:           project,
			DependsOn:         dependsOn,
		}
		s.Require().NoError(t.Insert())
	}

	s.taskQueue = TaskQueue{
		Distro: distroID,
		Queue:  items,
	}
}

func (s *taskDAGDispatchServiceSuite) TestConstructor() {
	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.Equal("distro_1", service.distroID)
	s.Equal(60*time.Second, service.ttl)
	s.NotEqual(util.ZeroTime, service.lastUpdated)

	s.Len(service.itemNodeMap, 100)
	s.Len(service.nodeItemMap, 100)
	s.Len(service.sorted, 100)

	s.Len(service.taskGroups, 4, "")
	s.Contains(service.taskGroups, compositeGroupID("group_1", "variant_1", "project_1", "version_1"))
	s.Contains(service.taskGroups, compositeGroupID("group_2", "variant_1", "project_1", "version_1"))
	s.Contains(service.taskGroups, compositeGroupID("group_1", "variant_2", "project_1", "version_1"))
	s.Contains(service.taskGroups, compositeGroupID("group_1", "variant_1", "project_1", "version_2"))
	s.Equal(len(service.taskGroups[compositeGroupID("group_1", "variant_1", "project_1", "version_1")].tasks), 20)
	s.Equal(len(service.taskGroups[compositeGroupID("group_2", "variant_1", "project_1", "version_1")].tasks), 20)
	s.Equal(len(service.taskGroups[compositeGroupID("group_1", "variant_2", "project_1", "version_1")].tasks), 20)
	s.Equal(len(service.taskGroups[compositeGroupID("group_1", "variant_1", "project_1", "version_2")].tasks), 20)

	expectedOrder := []string{
		"0",  // ''
		"1",  // 'group_1_variant_1_project_1_version_1'
		"2",  // 'group_2_variant_1_project_1_version_1'
		"3",  // 'group_1_variant_2_project_1_version_1'
		"4",  // 'group_1_variant_1_project_1_version_2'
		"5",  // ''
		"6",  // 'group_1_variant_1_project_1_version_1'
		"7",  // 'group_2_variant_1_project_1_version_1'
		"8",  // 'group_1_variant_2_project_1_version_1'
		"9",  // 'group_1_variant_1_project_1_version_2'
		"10", // ''
		"11", // 'group_1_variant_1_project_1_version_1'
		"12", // 'group_2_variant_1_project_1_version_1'
		"13", // 'group_1_variant_2_project_1_version_1'
		"14", // 'group_1_variant_1_project_1_version_2'
		"15", // ''
		"16", // 'group_1_variant_1_project_1_version_1'
		"17", // 'group_2_variant_1_project_1_version_1'
		"18", // 'group_1_variant_2_project_1_version_1'
		"19", // 'group_1_variant_1_project_1_version_2'
		"20", // ''
		"21", // 'group_1_variant_1_project_1_version_1'
		"22", // 'group_2_variant_1_project_1_version_1'
		"23", // 'group_1_variant_2_project_1_version_1'
		"24", // 'group_1_variant_1_project_1_version_2'
		"25", // ''
		"26", // 'group_1_variant_1_project_1_version_1'
		"27", // 'group_2_variant_1_project_1_version_1'
		"28", // 'group_1_variant_2_project_1_version_1'
		"29", // 'group_1_variant_1_project_1_version_2'
		"30", // ''
		"31", // 'group_1_variant_1_project_1_version_1'
		"32", // 'group_2_variant_1_project_1_version_1'
		"33", // 'group_1_variant_2_project_1_version_1'
		"34", // 'group_1_variant_1_project_1_version_2'
		"36", // 'group_1_variant_1_project_1_version_1'
		"37", // 'group_2_variant_1_project_1_version_1'
		"38", // 'group_1_variant_2_project_1_version_1'
		"39", // 'group_1_variant_1_project_1_version_2'
		"41", // 'group_1_variant_1_project_1_version_1'
		"42", // 'group_2_variant_1_project_1_version_1'
		"43", // 'group_1_variant_2_project_1_version_1'
		"44", // 'group_1_variant_1_project_1_version_2'
		"46", // 'group_1_variant_1_project_1_version_1'
		"47", // 'group_2_variant_1_project_1_version_1'
		"48", // 'group_1_variant_2_project_1_version_1'
		"49", // 'group_1_variant_1_project_1_version_2'
		"50", // ''
		"45", // ''
		"40", // ''
		"35", // ''
		"51", // 'group_1_variant_1_project_1_version_1'
		"52", // 'group_2_variant_1_project_1_version_1'
		"53", // 'group_1_variant_2_project_1_version_1'
		"54", // 'group_1_variant_1_project_1_version_2'
		"55", // ''
		"56", // 'group_1_variant_1_project_1_version_1'
		"57", // 'group_2_variant_1_project_1_version_1'
		"58", // 'group_1_variant_2_project_1_version_1'
		"59", // 'group_1_variant_1_project_1_version_2'
		"60", // ''
		"61", // 'group_1_variant_1_project_1_version_1'
		"62", // 'group_2_variant_1_project_1_version_1'
		"63", // 'group_1_variant_2_project_1_version_1'
		"64", // 'group_1_variant_1_project_1_version_2'
		"66", // 'group_1_variant_1_project_1_version_1'
		"67", // 'group_2_variant_1_project_1_version_1'
		"68", // 'group_1_variant_2_project_1_version_1'
		"69", // 'group_1_variant_1_project_1_version_2'
		"71", // 'group_1_variant_1_project_1_version_1'
		"72", // 'group_2_variant_1_project_1_version_1'
		"73", // 'group_1_variant_2_project_1_version_1'
		"74", // 'group_1_variant_1_project_1_version_2'
		"76", // 'group_1_variant_1_project_1_version_1'
		"77", // 'group_2_variant_1_project_1_version_1'
		"78", // 'group_1_variant_2_project_1_version_1'
		"79", // 'group_1_variant_1_project_1_version_2'
		"80", // ''
		"75", // ''
		"70", // ''
		"65", // ''
		"81", // 'group_1_variant_1_project_1_version_1'
		"82", // 'group_2_variant_1_project_1_version_1'
		"83", // 'group_1_variant_2_project_1_version_1'
		"84", // 'group_1_variant_1_project_1_version_2'
		"85", // ''
		"86", // 'group_1_variant_1_project_1_version_1'
		"87", // 'group_2_variant_1_project_1_version_1'
		"88", // 'group_1_variant_2_project_1_version_1'
		"89", // 'group_1_variant_1_project_1_version_2'
		"90", // ''
		"91", // 'group_1_variant_1_project_1_version_1'
		"92", // 'group_2_variant_1_project_1_version_1'
		"93", // 'group_1_variant_2_project_1_version_1'
		"94", // 'group_1_variant_1_project_1_version_2'
		"95", // ''
		"96", // 'group_1_variant_1_project_1_version_1'
		"97", // 'group_2_variant_1_project_1_version_1'
		"98", // 'group_1_variant_2_project_1_version_1'
		"99", // 'group_1_variant_1_project_1_version_2'
	}

	for i, node := range service.sorted {
		taskQueueItem := service.nodeItemMap[node.ID()]
		s.Equal(taskQueueItem.Id, expectedOrder[i])
	}
}

func (s *taskDAGDispatchServiceSuite) TestAddingSelfEdge() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	items := []TaskQueueItem{}

	t1 := task.Task{
		Id:                  "1",
		BuildId:             "ops_manager_kubernetes_init_test_run_patch_1a53e026e05561c3efbb626185e155a7d1e4865d_5d88953e2a60ed61eefe9561_19_09_23_09_49_51",
		TaskGroup:           "",
		StartTime:           util.ZeroTime,
		BuildVariant:        "init_test_run",
		Version:             "5d88953e2a60ed61eefe9561",
		Project:             "ops-manager-kubernetes",
		Activated:           true,
		ActivatedBy:         "",
		DistroId:            "archlinux-test",
		Requester:           "patch_request",
		Status:              evergreen.TaskUndispatched,
		Revision:            "1a53e026e05561c3efbb626185e155a7d1e4865d",
		RevisionOrderNumber: 1846,
		DependsOn: []task.Dependency{
			task.Dependency{
				TaskId:       "1",
				Status:       "success",
				Unattainable: false,
			},
		},
	}
	item1 := TaskQueueItem{
		Id:            "1",
		Group:         "",
		BuildVariant:  "init_test_run",
		Version:       "5d88953e2a60ed61eefe9561",
		Project:       "ops-manager-kubernetes",
		Requester:     "patch_request",
		GroupMaxHosts: 0,
		IsDispatched:  false,
		Dependencies:  []string{"1"},
	}

	s.Require().NoError(t1.Insert())
	items = append(items, item1)

	s.taskQueue = TaskQueue{
		Distro: "archlinux-test",
		Queue:  items,
	}

	_, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.Error(err)
	s.Contains(err.Error(), "cannot add a self edge to task")
}

func (s *taskDAGDispatchServiceSuite) TestAddingEdgeWithMissingNodes() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	items := []TaskQueueItem{}

	t1 := task.Task{
		Id:                  "1",
		BuildId:             "ops_manager_kubernetes_init_test_run_patch_1a53e026e05561c3efbb626185e155a7d1e4865d_5d88953e2a60ed61eefe9561_19_09_23_09_49_51",
		TaskGroup:           "",
		StartTime:           util.ZeroTime,
		BuildVariant:        "init_test_run",
		Version:             "5d88953e2a60ed61eefe9561",
		Project:             "ops-manager-kubernetes",
		Activated:           true,
		ActivatedBy:         "",
		DistroId:            "archlinux-test",
		Requester:           "patch_request",
		Status:              evergreen.TaskUndispatched,
		Revision:            "1a53e026e05561c3efbb626185e155a7d1e4865d",
		RevisionOrderNumber: 1846,
	}
	item1 := TaskQueueItem{
		Id:            "1",
		Group:         "",
		BuildVariant:  "init_test_run",
		Version:       "5d88953e2a60ed61eefe9561",
		Project:       "ops-manager-kubernetes",
		Requester:     "patch_request",
		GroupMaxHosts: 0,
		IsDispatched:  false,
	}

	t2 := task.Task{
		Id:                  "2",
		BuildId:             "ops_manager_kubernetes_e2e_openshift_cloud_qa_patch_1a53e026e05561c3efbb626185e155a7d1e4865d_5d88953e2a60ed61eefe9561_19_09_23_09_49_51",
		TaskGroup:           "e2e_core_task_group",
		TaskGroupMaxHosts:   5,
		TaskGroupOrder:      2,
		StartTime:           util.ZeroTime,
		BuildVariant:        "e2e_openshift_cloud_qa",
		Version:             "5d88953e2a60ed61eefe9561",
		Project:             "ops-manager-kubernetes",
		Activated:           true,
		ActivatedBy:         "",
		DistroId:            "archlinux-test",
		Requester:           "patch_request",
		Status:              evergreen.TaskUndispatched,
		Revision:            "1a53e026e05561c3efbb626185e155a7d1e4865d",
		RevisionOrderNumber: 1846,
		DependsOn: []task.Dependency{{
			TaskId:       "1",
			Status:       evergreen.TaskSucceeded,
			Unattainable: false,
		}},
	}
	item2 := TaskQueueItem{
		Id:            "2",
		Group:         "e2e_core_task_group",
		GroupIndex:    2,
		BuildVariant:  "e2e_openshift_cloud_qa",
		Version:       "5d88953e2a60ed61eefe9561",
		Project:       "ops-manager-kubernetes",
		GroupMaxHosts: 5,
		Requester:     "patch_request",
		Dependencies:  []string{"1"},
		IsDispatched:  false,
	}

	t3 := task.Task{
		Id:                  "3",
		BuildId:             "ops_manager_kubernetes_e2e_openshift_cloud_qa_patch_1a53e026e05561c3efbb626185e155a7d1e4865d_5d88953e2a60ed61eefe9561_19_09_23_09_49_51",
		TaskGroup:           "e2e_core_task_group",
		TaskGroupMaxHosts:   5,
		TaskGroupOrder:      1,
		StartTime:           util.ZeroTime,
		BuildVariant:        "e2e_openshift_cloud_qa",
		Version:             "5d88953e2a60ed61eefe9561",
		Project:             "ops-manager-kubernetes",
		Activated:           true,
		ActivatedBy:         "",
		DistroId:            "archlinux-test",
		Requester:           "patch_request",
		Status:              evergreen.TaskUndispatched,
		Revision:            "1a53e026e05561c3efbb626185e155a7d1e4865d",
		RevisionOrderNumber: 1846,
		DependsOn: []task.Dependency{{
			TaskId:       "1",
			Status:       evergreen.TaskSucceeded,
			Unattainable: false,
		}},
	}
	item3 := TaskQueueItem{
		Id:            "3",
		Group:         "e2e_core_task_group",
		GroupIndex:    1,
		BuildVariant:  "e2e_openshift_cloud_qa",
		Version:       "5d88953e2a60ed61eefe9561",
		Project:       "ops-manager-kubernetes",
		GroupMaxHosts: 5,
		Requester:     "patch_request",
		Dependencies:  []string{"1"},
		IsDispatched:  false,
	}

	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())

	items = append(items, item1)
	items = append(items, item2)
	items = append(items, item3)

	s.taskQueue = TaskQueue{
		Distro: "archlinux-test",
		Queue:  items,
	}

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)

	spec := TaskSpec{}

	next := service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("1", next.Id)

	next = service.FindNextTask(spec)
	s.Require().Nil(next)
	next = service.FindNextTask(spec)
	s.Require().Nil(next)
	next = service.FindNextTask(spec)
	s.Require().Nil(next)

	t1.Status = evergreen.TaskSucceeded

	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())

	items = []TaskQueueItem{}
	items = append(items, item2)
	items = append(items, item3)

	s.taskQueue = TaskQueue{
		Distro: "archlinux-test",
		Queue:  items,
	}

	service, err = newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)

	err = service.rebuild(s.taskQueue.Queue)
	s.Require().NoError(err)

	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("3", next.Id)

	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("2", next.Id)

	next = service.FindNextTask(spec)
	s.Require().Nil(next)

	//

	t2.DependsOn = []task.Dependency{
		task.Dependency{
			TaskId:       "1", // A Task.Id that will not be in the task_queue.
			Status:       evergreen.TaskSucceeded,
			Unattainable: false,
		},
	}

	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())

	items = []TaskQueueItem{}
	// items = append(items, item1)
	items = append(items, item2)
	items = append(items, item3)

	s.taskQueue = TaskQueue{
		Distro: "archlinux-test",
		Queue:  items,
	}

	service, err = newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	err = service.addEdge("2", "5") // There is no Node for the <to> task.Id: "5" in the task_queue.
	s.Error(err)
	s.Contains(err.Error(), "is not present in the DAG", nil)

	//

	t1.Status = evergreen.TaskFailed
	t3.DependsOn = []task.Dependency{{
		TaskId:       "1",
		Status:       evergreen.TaskFailed,
		Unattainable: false,
	}}

	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())

	items = []TaskQueueItem{}
	items = append(items, item1)
	items = append(items, item2)
	items = append(items, item3)

	s.taskQueue = TaskQueue{
		Distro: "archlinux-test",
		Queue:  items,
	}

	spec = TaskSpec{}

	service, err = newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)

	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("1", next.Id)

	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("3", next.Id)
}

func (s *taskDAGDispatchServiceSuite) TestNextTaskForDefaultTaskSpec() {
	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	spec := TaskSpec{}
	s.NoError(err)
	next := service.FindNextTask(spec)
	s.NotNil(next)
	// First, a standalone task
	s.Equal("0", next.Id)
	// Then all 20 tasks from "group_1_variant_1_project_1_version_1"
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("1", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("6", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("11", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("16", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("21", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("26", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("31", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("36", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("41", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("46", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("51", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("56", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("61", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("66", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("71", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("76", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("81", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("86", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("91", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("96", next.Id)
	// The all the tasks from "group_2_variant_1_project_1_version_1"
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("2", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("7", next.Id)
	next = service.FindNextTask(spec)
	s.NotNil(next)
	s.Equal("12", next.Id)
	// .....
}

func (s *taskDAGDispatchServiceSuite) TestSingleHostTaskGroupsBlock() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	items := []TaskQueueItem{}
	var startTime time.Time
	var endTime time.Time
	var status string
	for i := 0; i < 5; i++ {
		items = append(items, TaskQueueItem{
			Id:            fmt.Sprintf("%d", i),
			Group:         "group_1",
			BuildVariant:  "variant_1",
			Version:       "version_1",
			Project:       "project_1",
			GroupMaxHosts: 1,
		})
		if i == 0 {
			startTime = time.Now().Add(-2 * time.Minute)
			endTime = time.Now().Add(-time.Minute)
			status = evergreen.TaskSucceeded
		} else if i == 1 {
			startTime = time.Now().Add(-time.Minute)
			endTime = time.Now()
			status = evergreen.TaskFailed
		} else {
			startTime = util.ZeroTime
			endTime = util.ZeroTime
			status = ""
		}
		t := task.Task{
			Id:                fmt.Sprintf("%d", i),
			TaskGroup:         "group_1",
			BuildVariant:      "variant_1",
			Version:           "version_1",
			Project:           "project_1",
			TaskGroupMaxHosts: 1,
			StartTime:         startTime,
			FinishTime:        endTime,
			Status:            status,
		}
		s.Require().NoError(t.Insert())
	}

	s.taskQueue.Queue = items
	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	spec := TaskSpec{
		Group:        "group_1",
		BuildVariant: "variant_1",
		Version:      "version_1",
		Project:      "project_1",
	}
	next := service.FindNextTask(spec)
	s.Require().Nil(next)
}

func setTaskStatus(taskID string, status string) error {
	return task.UpdateOne(
		bson.M{
			task.IdKey: taskID,
		},
		bson.M{
			"$set": bson.M{
				// task.DispatchTimeKey: time.Now().Add(-1 * time.Minute),
				task.StatusKey: status,
			},
		},
	)
}

func (s *taskDAGDispatchServiceSuite) TestFindNextTask() {
	service, e := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(e)
	var spec TaskSpec
	var next *TaskQueueItem

	// Dispatch the first 5 tasks for the taskGroupTasks "group_1_variant_1_project_1_version_1", which represents a task group that initially contains 20 tasks.
	// task ids: ["1", "6", "11", "16", "21"]
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_1",
			Version:      "version_1",
			Project:      "project_1",
		}
		next = service.FindNextTask(spec)
		s.Require().NotNil(next)
		s.Equal(fmt.Sprintf("%d", 5*i+1), next.Id)
		s.Require().NoError(setTaskStatus(next.Id, evergreen.TaskSucceeded))
	}

	// Dispatch the first 5 tasks for taskGroupTasks "group_2_variant_1_project_1_version_1", which represents a task group that initially contains 20 tasks.
	// task ids: ["2", "7", "12", "17", "22"]
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_2",
			BuildVariant: "variant_1",
			Version:      "version_1",
			Project:      "project_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+2), next.Id)
		s.Require().NoError(setTaskStatus(next.Id, evergreen.TaskSucceeded))
	}

	// Dispatch the first 5 tasks for taskGroupTasks "group_1_variant_2_project_1_version_1", which represents a task group that initially contains 20 tasks.
	// task ids: ["3", "8", "13", "18", "23"]
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_2",
			Version:      "version_1",
			Project:      "project_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+3), next.Id)
		s.Require().NoError(setTaskStatus(next.Id, evergreen.TaskSucceeded))
	}

	// Dispatch the first 5 tasks for taskGroupTasks "group_1_variant_1_project_1_version_2", which represents a task group that initially contains 20 tasks.
	// task ids: ["4", "9", "14", "19", "24"]
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_1",
			Version:      "version_2",
			Project:      "project_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+4), next.Id)
		s.Require().NoError(setTaskStatus(next.Id, evergreen.TaskSucceeded))
	}

	// The taskGroupTasks "group_1_variant_1_project_1_version_1" now contains 15 tasks; dispatch another 5 of them.
	// task ids: ["26", "31", "36", "41", "46"]
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_1",
			Version:      "version_1",
			Project:      "project_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+26), next.Id)
		s.Require().NoError(setTaskStatus(next.Id, evergreen.TaskSucceeded))
	}

	//////////////////////////////////////////////////////////////////////////////
	// Repeat requests for tasks by a TaskSpec containing an empty Group field dispatch, in order:
	// (1) A single standalone (non-taskGroupTasks) task
	// (2) The rest of the tasks for taskGroupTasks "group_1_variant_1_project_1_version_1"
	// (3) The rest of the tasks for taskGroupTasks "group_2_variant_1_project_1_version_1"
	// (4) The rest of the tasks for taskGroupTasks "group_1_variant_2_project_1_version_1"
	// (5) The rest of the tasks for taskGroupTasks "group_1_variant_1_project_1_version_2"
	// (6) The remaining 19 standalone tasks
	//////////////////////////////////////////////////////////////////////////////

	// Make a request for another task, passing an "empty" TaskSpec{} - the returned task should should be TaskQueueItem.Id 0 and be a standalone task.
	spec = TaskSpec{}
	next = service.FindNextTask(spec)
	s.Equal("0", next.Id)
	s.Equal("", next.Group)

	currentID := 0
	var nextInt int
	var err error

	// Make another 10 requests for a task, passing an "empty" TaskSpec{} - all 10 dispatched tasks should come from the "group_1_variant_1_project_1_version_1" taskGroupTasks.
	// task ids: ["51", "56", "61", "66", "71", "76", "81", "86", "91", "96"]
	// All 20 tasks for taskGroupTasks "group_1_variant_1_project_1_version_1" have been dispatched.
	for i := 0; i < 10; i++ {
		next = service.FindNextTask(spec)
		nextInt, err = strconv.Atoi(next.Id)
		s.NoError(err)
		s.True(nextInt > currentID)
		currentID = nextInt
		s.Equal("group_1", next.Group)
		s.Equal("variant_1", next.BuildVariant)
		s.Equal("project_1", next.Project)
		s.Equal("version_1", next.Version)
		s.Equal("project_1", next.Project)
		s.Require().NoError(setTaskStatus(next.Id, evergreen.TaskSucceeded))
	}

	// Make another 15 requests for a task, passing an "empty" TaskSpec{} - all 15 dispatched tasks should come from the "group_2_variant_1_project_1_version_1" taskGroupTasks.
	// task ids: ["27", "32", "37", "42", "47", "52", "57", "62", "67", "72", "77", "82", "87", "82", "92, "97"]
	// All 20 tasks for taskGroupTasks "group_2_variant_1_project_1_version_1" have been dispatched.
	currentID = 0
	for i := 0; i < 15; i++ {
		next = service.FindNextTask(spec)
		nextInt, err = strconv.Atoi(next.Id)
		s.NoError(err)
		s.True(nextInt > currentID)
		currentID = nextInt
		s.Equal("group_2", next.Group)
		s.Equal("variant_1", next.BuildVariant)
		s.Equal("project_1", next.Project)
		s.Equal("version_1", next.Version)
		s.Equal("project_1", next.Project)
		s.Require().NoError(setTaskStatus(next.Id, evergreen.TaskSucceeded))
	}

	// Make another 15 requests for a task, passing an "empty" TaskSpec{} - all 15 dispatched tasks should come from the "group_1_variant_2_project_1_version_1" taskGroupTasks.
	// task ids: ["28", "33", "38", "43", "48", "53", "58", "63", "68", "73", "78", "83", "88", "93", "98"]
	// All 20 tasks for taskGroupTasks group_1_variant_2_project_1_version_1" have been dispatched.
	currentID = 0
	for i := 0; i < 15; i++ {
		next = service.FindNextTask(spec)
		nextInt, err = strconv.Atoi(next.Id)
		s.NoError(err)
		s.True(nextInt > currentID)
		currentID = nextInt
		s.Equal("group_1", next.Group)
		s.Equal("variant_2", next.BuildVariant)
		s.Equal("project_1", next.Project)
		s.Equal("version_1", next.Version)
		s.Equal("project_1", next.Project)
		s.Require().NoError(setTaskStatus(next.Id, evergreen.TaskSucceeded))
	}

	// Make another 15 requests for a task, passing an "empty" TaskSpec{} - all 15 dispatched tasks should come from the "group_1_variant_1_project_1_version_2" taskGroupTasks.
	// task ids: ["29", "34", "39", "44", "49", "54", "59", "64", "69", "74", "79", "84", "89", "94", "99"]
	// All 20 tasks for taskGroupTasks "group_1_variant_1_project_1_version_2" have been dispatched.
	currentID = 0
	for i := 0; i < 15; i++ {
		next = service.FindNextTask(spec)
		nextInt, err = strconv.Atoi(next.Id)
		s.NoError(err)
		s.True(nextInt > currentID)
		currentID = nextInt
		s.Equal("group_1", next.Group)
		s.Equal("variant_1", next.BuildVariant)
		s.Equal("project_1", next.Project)
		s.Equal("version_2", next.Version)
		s.Equal("project_1", next.Project)
		s.Require().NoError(setTaskStatus(next.Id, evergreen.TaskSucceeded))
	}

	// Make another 19 requests for a task, passing an "empty" TaskSpec{} - all 19 dispatched tasks should be standalone tasks.
	// The dispatch order of the 19 standalone tasks is dependent on the Node order of basicCachedDAGDispatcherImpl.sorted (for our particular set of test tasks and dependencies)
	expectedStandaloneTaskOrder := []string{"5", "10", "15", "20", "25", "30", "50", "45", "40", "35", "55", "60", "80", "75", "70", "65", "85", "90", "95"}
	for i := 0; i < 19; i++ {
		next = service.FindNextTask(spec)
		s.Equal(expectedStandaloneTaskOrder[i], next.Id)
		s.Equal("", next.Group)
		s.Require().NoError(setTaskStatus(next.Id, evergreen.TaskSucceeded))
	}
}

func (s *taskDAGDispatchServiceSuite) TestTaskGroupTasksRunningHostsVersusMaxHosts() {
	// Add a host which that would request a task with a TaskSpec resolving to "group_1_variant_1_project_1_version_1"
	h1 := host.Host{
		Id:   "sir-mixalot",
		Host: "ec2-18-234-180-219.compute-1.amazonaws.com",
		Distro: distro.Distro{
			Id: "distro_1",
		},
		LastTask:         "my_last_task_1",
		LastGroup:        "group_1",
		LastProject:      "project_1",
		LastVersion:      "version_1",
		LastBuildVariant: "variant_1",
		Status:           evergreen.HostRunning,
	}
	s.Require().NoError(h1.Insert())

	service, e := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(e)

	spec := TaskSpec{}
	next := service.FindNextTask(spec)
	s.Equal("0", next.Id)
	next = service.FindNextTask(spec)
	// The next task, according to the order of basicCachedDAGDispatcherImpl.sorted is from task group "group_1_variant_1_version_1".
	// However, runningHosts < maxHosts is false for this task group, so we cannot dispatch this task.
	s.NotEqual("1", next.Id)
	// Instead, return the next task, which is from task group "group_2_variant_1_project_1_version_1".
	s.Equal("2", next.Id)
	s.Equal("group_2", next.Group)
	s.Equal("variant_1", next.BuildVariant)
	s.Equal("version_1", next.Version)
	s.Equal("project_1", next.Project)
	next = service.FindNextTask(spec)
	// Same situation again - so we dispatch the next task from "group_2_variant_1_project_1_version_1".
	s.Equal("7", next.Id)
	s.Equal("group_2", next.Group)
	s.Equal("variant_1", next.BuildVariant)
	s.Equal("version_1", next.Version)
	s.Equal("project_1", next.Project)
}

func (s *taskDAGDispatchServiceSuite) TestTaskGroupWithExternalDependency() {
	dependsOn := []task.Dependency{{TaskId: "95"}}
	err := task.UpdateOne(
		bson.M{
			task.IdKey: "1",
		},
		bson.M{
			"$set": bson.M{
				task.DependsOnKey: dependsOn,
			},
		},
	)
	s.Require().NoError(err)

	service, e := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.Require().NoError(e)
	var spec TaskSpec
	var next *TaskQueueItem

	// task ids: ["1", "6", "11", "16", "21", "26", "31", "36", "41", "46", "51", "56", "61", "66", "71", "76", "81", "86", "91", "96"]
	// Dispatch 5 tasks for the taskGroupTasks "group_1_variant_1_project_1_version_1".
	// task "1" is dependent on task "95" having status evergreen.TaskSucceeded, so we cannot dispatch task "1".
	expectedOrder := []string{
		"6",
		"11",
		"16",
		"21",
		"26",
	}

	spec = TaskSpec{
		Group:        "group_1",
		BuildVariant: "variant_1",
		Version:      "version_1",
		Project:      "project_1",
	}
	taskGroupID := compositeGroupID(spec.Group, spec.BuildVariant, spec.Project, spec.Version)
	taskGroup := service.taskGroups[taskGroupID]

	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal(expectedOrder[0], next.Id)
	s.Equal("1", taskGroup.tasks[0].Id)
	s.Equal(true, taskGroup.tasks[0].IsDispatched) // Even though this task was not actually dispatched, we still set IsDispatched = true.
	s.Equal("6", taskGroup.tasks[1].Id)
	s.Equal(true, taskGroup.tasks[1].IsDispatched)
	s.Equal("11", taskGroup.tasks[2].Id)
	s.Equal(false, taskGroup.tasks[2].IsDispatched)

	for i := 1; i < 5; i++ {
		next = service.FindNextTask(spec)
		s.Require().NotNil(next)
		s.Equal(expectedOrder[i], next.Id)
		s.Equal(expectedOrder[i], taskGroup.tasks[i+1].Id)
		s.Equal(true, taskGroup.tasks[i+1].IsDispatched)
	}

	// Set task "95"'s status to evergreen.TaskSucceeded.
	err = task.UpdateOne(
		bson.M{
			task.IdKey: "95",
		},
		bson.M{
			"$set": bson.M{
				task.StatusKey: evergreen.TaskSucceeded,
			},
		},
	)
	s.Require().NoError(err)

	// Rebuild the dispatcher service's in-memory state.
	err = service.rebuild(s.taskQueue.Queue)
	s.Require().NoError(err)

	// Now task "1" can be dispatched!
	expectedOrder = []string{
		"1",
		"31",
		"36",
		"41",
		"46",
		"51",
		"56",
		"61",
		"66",
		"71",
		"76",
		"81",
		"86",
		"91",
		"96",
	}
	for i := 0; i < 15; i++ {
		next = service.FindNextTask(spec)
		s.Require().NotNil(next)
		s.Equal(expectedOrder[i], next.Id)
	}

	// All the tasks within taskGroup "group_1_variant_1_project_1_version_1" has now been dispatched.
	next = service.FindNextTask(spec)
	s.Require().NotNil(next)
	s.Equal("0", next.Id)
	s.Equal("", next.Group)
}

func (s *taskDAGDispatchServiceSuite) TestSingleHostTaskGroupOrdering() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	items := []TaskQueueItem{}
	groupIndexes := []int{2, 0, 4, 1, 3}

	for i := 0; i < 5; i++ {
		ID := fmt.Sprintf("%d", i)
		items = append(items, TaskQueueItem{
			Id:            ID,
			Group:         "group_1",
			BuildVariant:  "variant_1",
			Version:       "version_1",
			Project:       "project_1",
			GroupMaxHosts: 1,
			GroupIndex:    groupIndexes[i],
		})
		t := task.Task{
			Id:                ID,
			TaskGroup:         "group_1",
			BuildVariant:      "variant_1",
			Version:           "version_1",
			TaskGroupMaxHosts: 1,
			Project:           "project_1",
			StartTime:         util.ZeroTime,
			FinishTime:        util.ZeroTime,
		}
		s.Require().NoError(t.Insert())

		s.taskQueue = TaskQueue{
			Distro: "distro_1",
			Queue:  items,
		}
	}

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.Require().NoError(err)

	spec := TaskSpec{
		Group:        "group_1",
		BuildVariant: "variant_1",
		Version:      "version_1",
		Project:      "project_1",
	}
	expectedOrder := []string{"1", "3", "0", "4", "2"}

	for i := 0; i < 5; i++ {
		next := service.FindNextTask(spec)
		s.Require().NotNil(next)
		s.Equal(expectedOrder[i], next.Id)
	}
}

//////////////////////////////////////////////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////

type taskDispatchServiceSuite struct {
	suite.Suite

	taskQueue TaskQueue
}

func TestTaskDispatchServiceSuite(t *testing.T) {
	suite.Run(t, new(taskDispatchServiceSuite))
}

func (s *taskDispatchServiceSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(db.ClearCollections(host.Collection))

	items := []TaskQueueItem{}
	var group string
	var variant string
	var version string
	var maxHosts int
	project := "project_1"

	for i := 0; i < 100; i++ {
		if i%5 == 0 { // no group
			group = ""
			variant = "variant_1"
			version = "version_1"
			maxHosts = 0
		} else if i%5 == 1 { // "group_1_variant_1_project_1_version_1"
			group = "group_1"
			variant = "variant_1"
			version = "version_1"
			maxHosts = 1
		} else if i%5 == 2 { // "group_2_variant_1_project_1_version_1"
			group = "group_2"
			variant = "variant_1"
			version = "version_1"
			maxHosts = 2
		} else if i%5 == 3 { // "group_1_variant_2_project_1_version_1"
			group = "group_1"
			variant = "variant_2"
			version = "version_1"
			maxHosts = 2
		} else if i%5 == 4 { // "group_1_variant_1_project_1_version_2"
			group = "group_1"
			variant = "variant_1"
			version = "version_2"
			maxHosts = 2
		}
		items = append(items, TaskQueueItem{
			Id:            fmt.Sprintf("%d", i),
			Group:         group,
			BuildVariant:  variant,
			Project:       project,
			Version:       version,
			GroupMaxHosts: maxHosts,
		})
		t := task.Task{
			Id:                fmt.Sprintf("%d", i),
			TaskGroup:         group,
			BuildVariant:      variant,
			Version:           version,
			Project:           project,
			TaskGroupMaxHosts: maxHosts,
			StartTime:         util.ZeroTime,
			FinishTime:        util.ZeroTime,
		}
		s.Require().NoError(t.Insert())
	}

	s.taskQueue = TaskQueue{
		Distro: "distro_1",
		Queue:  items,
	}
}

func (s *taskDispatchServiceSuite) TestConstructor() {
	service := newDistroTaskDispatchService(s.taskQueue, "", time.Minute)
	//////////////////////////////////////////////////////////////////////////////
	// basicCachedDispatcherImpl.order[0] = "0"
	// basicCachedDispatcherImpl.order[1] = "group_1_variant_1_project_1_version_1"
	// basicCachedDispatcherImpl.order[2] = "group_2_variant_1_project_1_version_1"
	// basicCachedDispatcherImpl.order[3] = "group_1_variant_2_project_1_version_1"
	// basicCachedDispatcherImpl.order[4] = "group_1_variant_1_project_1_version_2"
	// basicCachedDispatcherImpl.order[5] = "5"
	// basicCachedDispatcherImpl.order[6] = "10"
	// basicCachedDispatcherImpl.order[7] = "15"
	// basicCachedDispatcherImpl.order[8] = "20"
	// basicCachedDispatcherImpl.order[9] = "25"
	// basicCachedDispatcherImpl.order[10] = "30"
	// basicCachedDispatcherImpl.order[11] = "35"
	// basicCachedDispatcherImpl.order[12] = "40"
	// basicCachedDispatcherImpl.order[13] = "45"
	// basicCachedDispatcherImpl.order[14] = "50"
	// basicCachedDispatcherImpl.order[15] = "55"
	// basicCachedDispatcherImpl.order[16] = "60"
	// basicCachedDispatcherImpl.order[17] = "65"
	// basicCachedDispatcherImpl.order[18] = "70"
	// basicCachedDispatcherImpl.order[19] = "75"
	// basicCachedDispatcherImpl.order[20] = "80"
	// basicCachedDispatcherImpl.order[21] = "85"
	// basicCachedDispatcherImpl.order[22] = "90"
	// basicCachedDispatcherImpl.order[23] = "95"
	//////////////////////////////////////////////////////////////////////////////
	// basicCachedDispatcherImpl.units["0"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["group_1_variant_1_project_1_version_1"].len(tasks) = 20
	//	[1, 6, 11, 16, 21, 26, 31, 36, 41, 46, 51, 56, 61, 66, 71, 76, 81, 86, 91, 96]
	// basicCachedDispatcherImpl.units["group_1_variant_1_project_1_version_1"].maxHosts = 1
	// basicCachedDispatcherImpl.units["group_2_variant_1_project_1_version_1"].len(tasks) = 20
	//	[2, 7, 12, 17, 22, 27, 32, 37, 42, 47, 52, 57, 62, 67, 72, 77, 82, 87, 92, 97]
	// basicCachedDispatcherImpl.units["group_2_variant_1_project_1_version_1"].maxHosts = 2
	// basicCachedDispatcherImpl.units["group_1_variant_1_project_1_version_2"].len(tasks) = 20
	// basicCachedDispatcherImpl.units["group_1_variant_1_project_1_version_2"].maxHosts = 2
	// basicCachedDispatcherImpl.units["group_1_variant_2_project_1_version_1"].len(tasks) = 20
	// basicCachedDispatcherImpl.units[""group_1_variant_2_project_1_version_1"].maxHosts = 2
	// basicCachedDispatcherImpl.units["15"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["20"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["35"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["45"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["60"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["65"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["75"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["95"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["5"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["10"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["55"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["90"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["25"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["40"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["70"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["85"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["30"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["50"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["80"].len(tasks) = 1
	//////////////////////////////////////////////////////////////////////////////

	s.Len(service.order, 24, "20 bare tasks + 4 task groups")
	s.Len(service.units, 24, "20 bare tasks + 4 task groups")
	s.Equal(len(service.order), len(service.units), "order and units should have same length")
	s.Equal("distro_1", service.distroID)
	s.Equal(60*time.Second, service.ttl)
	s.NotEqual(util.ZeroTime, service.lastUpdated)

	s.Contains(service.order, compositeGroupID("group_1", "variant_1", "project_1", "version_1"))
	s.Contains(service.units, compositeGroupID("group_1", "variant_1", "project_1", "version_1"))
	s.Len(service.units[compositeGroupID("group_1", "variant_1", "project_1", "version_1")].tasks, 20)
	s.Equal(1, service.units[compositeGroupID("group_1", "variant_1", "project_1", "version_1")].maxHosts)

	s.Contains(service.order, compositeGroupID("group_2", "variant_1", "project_1", "version_1"))
	s.Contains(service.units, compositeGroupID("group_2", "variant_1", "project_1", "version_1"))
	s.Len(service.units[compositeGroupID("group_2", "variant_1", "project_1", "version_1")].tasks, 20)
	s.Equal(2, service.units[compositeGroupID("group_2", "variant_1", "project_1", "version_1")].maxHosts)

	s.Contains(service.order, compositeGroupID("group_1", "variant_2", "project_1", "version_1"))
	s.Contains(service.units, compositeGroupID("group_1", "variant_2", "project_1", "version_1"))
	s.Len(service.units[compositeGroupID("group_1", "variant_2", "project_1", "version_1")].tasks, 20)
	s.Equal(2, service.units[compositeGroupID("group_1", "variant_2", "project_1", "version_1")].maxHosts)

	s.Contains(service.order, compositeGroupID("group_1", "variant_1", "project_1", "version_2"))
	s.Contains(service.units, compositeGroupID("group_1", "variant_1", "project_1", "version_2"))
	s.Len(service.units[compositeGroupID("group_1", "variant_1", "project_1", "version_2")].tasks, 20)
	s.Equal(2, service.units[compositeGroupID("group_1", "variant_1", "project_1", "version_2")].maxHosts)

	for i := 0; i < 100; i = i + 5 {
		s.Contains(service.order, fmt.Sprintf("%d", i))
	}
	for i := 0; i < 100; i++ {
		if i%5 != 0 {
			s.NotContains(service.order, fmt.Sprintf("%d", i))
		}
	}
}

func (s *taskDispatchServiceSuite) TestEmptyService() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	t := task.Task{
		Id:        "a-standalone-task",
		TaskGroup: "",
		StartTime: util.ZeroTime,
	}
	s.Require().NoError(t.Insert())

	s.taskQueue.Queue = []TaskQueueItem{
		{
			Id:    "a-standalone-task",
			Group: "",
		},
	}
	service := newDistroTaskDispatchService(s.taskQueue, "", time.Minute)
	next := service.FindNextTask(TaskSpec{})
	s.Require().NotNil(next)
	s.Equal("a-standalone-task", next.Id)
	next = service.FindNextTask(TaskSpec{})
	s.Nil(next)
	s.Empty(service.order) // slice is emptied when map is emptied
}

func (s *taskDispatchServiceSuite) TestSingleHostTaskGroupsBlock() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(db.ClearCollections(host.Collection))
	items := []TaskQueueItem{}
	var startTime time.Time
	var endTime time.Time
	var status string
	for i := 0; i < 5; i++ {
		items = append(items, TaskQueueItem{
			Id:            fmt.Sprintf("%d", i),
			Group:         "group_1",
			BuildVariant:  "variant_1",
			Version:       "version_1",
			Project:       "project_1",
			GroupMaxHosts: 1,
		})
		if i == 0 {
			startTime = time.Now().Add(-2 * time.Minute)
			endTime = time.Now().Add(-time.Minute)
			status = evergreen.TaskSucceeded
		} else if i == 1 {
			startTime = time.Now().Add(-time.Minute)
			endTime = time.Now()
			status = evergreen.TaskFailed
		} else {
			startTime = util.ZeroTime
			endTime = util.ZeroTime
			status = ""
		}
		t := task.Task{
			Id:                fmt.Sprintf("%d", i),
			TaskGroup:         "group_1",
			BuildVariant:      "variant_1",
			Version:           "version_1",
			Project:           "project_1",
			TaskGroupMaxHosts: 1,
			StartTime:         startTime,
			FinishTime:        endTime,
			Status:            status,
		}
		s.Require().NoError(t.Insert())
	}

	s.taskQueue.Queue = items
	service := newDistroTaskDispatchService(s.taskQueue, "", time.Minute)
	spec := TaskSpec{
		Group:        "group_1",
		BuildVariant: "variant_1",
		Project:      "project_1",
		Version:      "version_1",
	}
	next := service.FindNextTask(spec)
	s.Nil(next)
	s.Empty(service.units)
}

func (s *taskDispatchServiceSuite) TestFindNextTask() {
	service := newDistroTaskDispatchService(s.taskQueue, "", time.Minute)
	var spec TaskSpec
	var next *TaskQueueItem
	//////////////////////////////////////////////////////////////////////////////
	// basicCachedDispatcherImpl.order[0] = "0"
	// basicCachedDispatcherImpl.order[1] = "group_1_variant_1_project_1_version_1"
	// basicCachedDispatcherImpl.order[2] = "group_2_variant_2_project_1_version_1"
	// basicCachedDispatcherImpl.order[3] = "group_1_variant_2_project_1_version_1"
	// basicCachedDispatcherImpl.order[4] = "group_1_variant_1_project_1_version_2"
	// basicCachedDispatcherImpl.order[5] = "5"
	// basicCachedDispatcherImpl.order[6] = "10"
	// basicCachedDispatcherImpl.order[7] = "15"
	// basicCachedDispatcherImpl.order[8] = "20"
	// basicCachedDispatcherImpl.order[9] = "25"
	// basicCachedDispatcherImpl.order[10] = "30"
	// basicCachedDispatcherImpl.order[11] = "35"
	// basicCachedDispatcherImpl.order[12] = "40"
	// basicCachedDispatcherImpl.order[13] = "45"
	// basicCachedDispatcherImpl.order[14] = "50"
	// basicCachedDispatcherImpl.order[15] = "55"
	// basicCachedDispatcherImpl.order[16] = "60"
	// basicCachedDispatcherImpl.order[17] = "65"
	// basicCachedDispatcherImpl.order[18] = "70"
	// basicCachedDispatcherImpl.order[19] = "75"
	// basicCachedDispatcherImpl.order[20] = "80"
	// basicCachedDispatcherImpl.order[21] = "85"
	// basicCachedDispatcherImpl.order[22] = "90"
	// basicCachedDispatcherImpl.order[23] = "95"
	//////////////////////////////////////////////////////////////////////////////
	// basicCachedDispatcherImpl.units["0"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["group_1_variant_1_project_1_version_1"].len(tasks) = 20
	// basicCachedDispatcherImpl.units["group_2_variant_1_project_1_version_1"].len(tasks) = 20
	// basicCachedDispatcherImpl.units["group_1_variant_1_project_1_version_2"].len(tasks) = 20
	// basicCachedDispatcherImpl.units["group_1_variant_2_project_1_version_1"].len(tasks) = 20
	// basicCachedDispatcherImpl.units["15"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["20"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["35"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["45"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["60"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["65"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["75"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["95"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["5"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["10"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["55"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["90"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["25"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["40"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["70"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["85"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["30"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["50"].len(tasks) = 1
	// basicCachedDispatcherImpl.units["80"].len(tasks) = 1
	//////////////////////////////////////////////////////////////////////////////

	// Dispatch the first 5 tasks for the schedulableUnit "group_1_variant_1_project_1_version_1", which represents a task group that initially contains 20 tasks.
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_1",
			Version:      "version_1",
			Project:      "project_1",
		}
		next = service.FindNextTask(spec)
		s.Require().NotNil(next)
		s.Equal(fmt.Sprintf("%d", 5*i+1), next.Id)
	}

	// Dispatch the first 5 tasks for schedulableUnit "group_2_variant_1_project_1_version_1", which represents a task group that initially contains 20 tasks.
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_2",
			BuildVariant: "variant_1",
			Version:      "version_1",
			Project:      "project_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+2), next.Id)
	}

	// Dispatch the first 5 tasks for schedulableUnit "group_1_variant_2_project_1_version_1", which represents a task group that initially contains 20 tasks.
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_2",
			Version:      "version_1",
			Project:      "project_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+3), next.Id)
	}

	// Dispatch the first 5 tasks for schedulableUnit "group_1_variant_1_project_1_version_2", which represents a task group that initially contains 20 tasks.
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_1",
			Version:      "version_2",
			Project:      "project_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+4), next.Id)
	}

	// The task group schedulableUnit "group_1_variant_1_project_1_version_1" now contains 15 tasks; dispatch another 5 of them.
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_1",
			Version:      "version_1",
			Project:      "project_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+26), next.Id)
	}

	//////////////////////////////////////////////////////////////////////////////
	// Repeat requests for tasks by a TaskSpec containing an empty Group field dispatch, in order:
	// (1) A single standalone (non-task group) task
	// (2) The rest of the task group tasks for schedulableUnit "group_1_variant_1_project_1_version_1"
	// (3) The rest of the task group tasks for schedulableUnit "group_2_variant_1_project_1_version_1"
	// (4) The rest of the task group tasks for schedulableUnit "group_1_variant_2_project_1_version_1"
	// (5) The rest of the task group tasks for schedulableUnit "group_1_variant_1_project_1_version_2"
	// (6) The remaining 19 schedulableUnits, which are all individual, standalone tasks
	//////////////////////////////////////////////////////////////////////////////

	spec = TaskSpec{
		Group:        "",
		BuildVariant: "variant_1",
		Version:      "version_1",
		Project:      "project_2",
	}
	next = service.FindNextTask(spec)
	s.Equal("0", next.Id)
	currentID := 0
	var nextInt int
	var err error

	// Dispatch the remaining 10 tasks from the task group schedulableUnit "group_1_variant_1_project_1_version_1".
	for i := 0; i < 10; i++ {
		next = service.FindNextTask(spec)
		nextInt, err = strconv.Atoi(next.Id)
		s.NoError(err)
		s.True(nextInt > currentID)
		currentID = nextInt
		s.Equal("group_1", next.Group)
		s.Equal("variant_1", next.BuildVariant)
		s.Equal("version_1", next.Version)
		s.Equal("project_1", next.Project)
	}

	// All 20 tasks for schedulableUnit "group_1_variant_1_project_1_version_1" have been dispatched.
	// basicCachedDispatcherImpl.order's ([]string) next value is "group_2_variant_1_project_1_version_1".
	// The corresponding schedulableUnit represents another task group; dispatch its remaining 15 tasks.
	currentID = 0
	for i := 0; i < 15; i++ {
		next = service.FindNextTask(spec)
		nextInt, err = strconv.Atoi(next.Id)
		s.NoError(err)
		s.True(nextInt > currentID)
		currentID = nextInt
		s.Equal("group_2", next.Group)
		s.Equal("variant_1", next.BuildVariant)
		s.Equal("version_1", next.Version)
		s.Equal("project_1", next.Project)
	}

	// All 20 tasks for schedulableUnit "group_2_variant_1_project_1_version_1" have been dispatched.
	// basicCachedDispatcherImpl.order's ([]string) next value is "group_1_variant_2_project_1_version_1".
	// The corresponding schedulableUnit represents another task group; dispatch its remaining 15 tasks.
	currentID = 0
	for i := 0; i < 15; i++ {
		next = service.FindNextTask(spec)
		nextInt, err = strconv.Atoi(next.Id)
		s.NoError(err)
		s.True(nextInt > currentID)
		currentID = nextInt
		s.Equal("group_1", next.Group)
		s.Equal("variant_2", next.BuildVariant)
		s.Equal("version_1", next.Version)
		s.Equal("project_1", next.Project)
	}

	// All 20 tasks for schedulableUnit ""group_1_variant_2_project_1_version_1"" have been dispatched.
	// basicCachedDispatcherImpl.order's ([]string) next value is "group_1_variant_1_project_1_version_2".
	// The corresponding schedulableUnit represents another task group; dispatch its remaining 15 tasks.
	currentID = 0
	for i := 0; i < 15; i++ {
		next = service.FindNextTask(spec)
		nextInt, err = strconv.Atoi(next.Id)
		s.NoError(err)
		s.True(nextInt > currentID)
		currentID = nextInt
		s.Equal("group_1", next.Group)
		s.Equal("variant_1", next.BuildVariant)
		s.Equal("version_2", next.Version)
		s.Equal("project_1", next.Project)
	}

	// The remaining 19 schedulableUnits represent individual, standalone tasks. Dispatch the rest of these non-task group tasks.
	currentID = 0
	for i := 0; i < 19; i++ {
		next = service.FindNextTask(spec)
		nextInt, err = strconv.Atoi(next.Id)
		s.NoError(err)
		s.True(nextInt > currentID)
		currentID = nextInt
		s.Equal("", next.Group)
		s.Equal("variant_1", next.BuildVariant)
		s.Equal("version_1", next.Version)
		s.Equal("project_1", next.Project)
	}
}

func (s *taskDispatchServiceSuite) TestSchedulableUnitsRunningHostsVersusMaxHosts() {
	s.Require().NoError(db.ClearCollections(host.Collection))

	// Add a host which that would request a task with a TaskSpec resolving to "group_1_variant_1_project_1_version_1"
	h1 := host.Host{
		Id:   "sir-mixalot",
		Host: "ec2-18-234-180-219.compute-1.amazonaws.com",
		Distro: distro.Distro{
			Id: "distro_1",
		},
		LastTask:         "my_last_task_1",
		LastGroup:        "group_1",
		LastProject:      "project_1",
		LastVersion:      "version_1",
		LastBuildVariant: "variant_1",
		Status:           evergreen.HostRunning,
	}
	s.Require().NoError(h1.Insert())

	service := newDistroTaskDispatchService(s.taskQueue, "", time.Minute)
	//////////////////////////////////////////////////////////////////////////////
	// basicCachedDispatcherImpl.order[0] = "0"
	// basicCachedDispatcherImpl.order[1] = "group_1_variant_1_project_1_version_1"
	// basicCachedDispatcherImpl.order[2] = "group_2_variant_1_project_1_version_1"
	//////////////////////////////////////////////////////////////////////////////
	// basicCachedDispatcherImpl.units["0"].len(tasks) = 1
	//	[0]
	// basicCachedDispatcherImpl.units["group_1_variant_1_project_1_version_1"].len(tasks) = 20
	//	[1, 6, 11, 16, 21, 26, 31, 36, 41, 46, 51, 56, 61, 66, 71, 76, 81, 86, 91, 96]
	// basicCachedDispatcherImpl.units[group_1_variant_1_project_1_version_1].maxHosts = 1
	// basicCachedDispatcherImpl.units["group_2_variant_1_project_1_version_1"].len(tasks) = 20
	//	[2, 7, 12, 17, 22, 27, 32, 37, 42, 47, 52, 57, 62, 67, 72, 77, 82, 87, 92, 97]
	// basicCachedDispatcherImpl.units["group_2_variant_1_project_1_version_1"].maxHosts = 2
	//////////////////////////////////////////////////////////////////////////////
	spec := TaskSpec{}
	next := service.FindNextTask(spec)
	s.Equal("0", next.Id)
	s.Equal("", next.Group)

	// basicCachedDispatcherImpl.order's ([]string) next value is "group_1_variant_1_project_1_version_1".
	// However, runningHosts < maxHosts is false for its corresponding schedulableUnit, so we cannot dispatch one of its tasks
	// On to basicCachedDispatcherImpl.order's next value: "group_2_variant_1_project_1_version_1" - we can dispatch a task from its corresponding schedulableUnit
	next = service.FindNextTask(spec)
	s.Equal("2", next.Id)
	s.Equal("group_2", next.Group)
	s.Equal("variant_1", next.BuildVariant)
	s.Equal("version_1", next.Version)
	s.Equal("project_1", next.Project)
	s.Equal(2, next.GroupMaxHosts)

	// Same situation again - so we dispatch the next task for the schedulableUnit "group_2_variant_1_project_1_version_1"
	next = service.FindNextTask(spec)
	s.Equal("7", next.Id)
	s.Equal("group_2", next.Group)
	s.Equal("variant_1", next.BuildVariant)
	s.Equal("version_1", next.Version)
	s.Equal("project_1", next.Project)
	s.Equal(2, next.GroupMaxHosts)
}
