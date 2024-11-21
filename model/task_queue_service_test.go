package model

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

type taskDAGDispatchServiceSuite struct {
	suite.Suite
	ctx       context.Context
	cancel    context.CancelFunc
	taskQueue TaskQueue
}

func (s *taskDAGDispatchServiceSuite) TearDownTest() {
	s.cancel()
}

func TestTaskDAGDispatchServiceSuite(t *testing.T) {
	suite.Run(t, new(taskDAGDispatchServiceSuite))
}

func (s *taskDAGDispatchServiceSuite) TestOutsideTasksWithTaskGroupDependencies() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(db.ClearCollections(host.Collection))
	distroID := "distro_1"

	t1 := task.Task{
		Id:                "taskgroup_task1",
		BuildId:           "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:         "tg_compile_and_test",
		TaskGroupMaxHosts: 1,
		StartTime:         utility.ZeroTime,
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
	t2 := task.Task{
		Id:                  "taskgroup_task2",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           utility.ZeroTime,
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
	t3 := task.Task{
		Id:                  "taskgroup_task3",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           utility.ZeroTime,
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
	t4 := task.Task{
		Id:                  "taskgroup_task4",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           utility.ZeroTime,
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
	t5 := task.Task{
		Id:                  "external_task5",
		BuildId:             "build_1",
		TaskGroup:           "",
		TaskGroupMaxHosts:   0,
		StartTime:           utility.ZeroTime,
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
			{
				TaskId:       "taskgroup_task3",
				Status:       "success",
				Unattainable: false,
			},
		},
	}
	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())
	s.Require().NoError(t4.Insert())
	s.Require().NoError(t5.Insert())

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.taskQueue = TaskQueue{
		Distro: distroID,
		Queue:  s.refreshTaskQueue(service),
	}
	s.Equal("distro_1", service.distroID)
	s.Equal(60*time.Second, service.ttl)
	s.NotEqual(utility.ZeroTime, service.lastUpdated)

	spec := TaskSpec{}

	// 3 successive calls (regardless of the TaskSpec passed) will dispatch 3 task group tasks, per TaskGroupOrder.
	next := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("taskgroup_task2", next.Id) // TaskGroupOrder: 1
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("taskgroup_task4", next.Id) // TaskGroupOrder: 2
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("taskgroup_task3", next.Id) // TaskGroupOrder: 3

	// Fake a refresh of the in-memory queue.
	s.Require().NoError(db.Clear(task.Collection))
	s.Require().NoError(t1.Insert())
	s.Require().NoError(t3.Insert())
	s.Require().NoError(t5.Insert())
	err = setTaskStatus("taskgroup_task3", evergreen.TaskSucceeded)
	s.Require().NoError(err)
	s.taskQueue.Queue = s.refreshTaskQueue(service)

	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("taskgroup_task1", next.Id)

	// "external_task5" can now be dispatched as its dependency "taskgroup_task3" has completed successfully.
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("external_task5", next.Id)

	// There are no more tasks to dispatch.
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)
}

func (s *taskDAGDispatchServiceSuite) TestIntraTaskGroupDependencies() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(db.ClearCollections(host.Collection))
	distroID := "distro_1"
	t1 := task.Task{
		Id:                  "task1",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           utility.ZeroTime,
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
			{
				TaskId:       "task2",
				Status:       "success",
				Unattainable: false,
			},
			{
				TaskId:       "task4",
				Status:       "success",
				Unattainable: false,
			},
			{
				TaskId:       "task3",
				Status:       "success",
				Unattainable: false,
			},
		},
	}
	t2 := task.Task{
		Id:                  "task2",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           utility.ZeroTime,
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
	t3 := task.Task{
		Id:                  "task3",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           utility.ZeroTime,
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
			{
				TaskId:       "task2",
				Status:       "success",
				Unattainable: false,
			},
			{
				TaskId:       "task4",
				Status:       "success",
				Unattainable: false,
			},
		},
	}
	t4 := task.Task{
		Id:                  "task4",
		BuildId:             "genny_archlinux_patch_6273aa2072f8325b8d1ceae2dfff74a775b018fc_5d8cd23da4cf4747f4210333_19_09_26_14_59_10",
		TaskGroup:           "tg_compile_and_test",
		TaskGroupMaxHosts:   1,
		StartTime:           utility.ZeroTime,
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
			{
				TaskId:       "task2",
				Status:       "success",
				Unattainable: false,
			},
		},
	}

	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())
	s.Require().NoError(t4.Insert())

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.taskQueue = TaskQueue{
		Distro: distroID,
		Queue:  s.refreshTaskQueue(service),
	}

	s.Equal("distro_1", service.distroID)
	s.Equal(60*time.Second, service.ttl)
	s.NotEqual(utility.ZeroTime, service.lastUpdated)

	spec := TaskSpec{}

	// Only "task2" can be dispatched - the other 3 tasks cannot be dispatched as they all have unmet dependencies.
	next := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("task2", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)

	// Fake a refresh of the in-memory queue.
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())
	s.Require().NoError(t4.Insert())
	err = setTaskStatus("task2", evergreen.TaskSucceeded)
	s.Require().NoError(err)

	s.taskQueue.Queue = s.refreshTaskQueue(service)

	err = service.rebuild(s.taskQueue.Queue)
	s.Require().NoError(err)

	// Only "task4" can be dispatched - the other 2 tasks cannot be dispatched as they have unmet dependencies.
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("task4", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)

	// Fake a refresh of the in-memory queue.
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())
	s.Require().NoError(t4.Insert())
	err = setTaskStatus("task2", evergreen.TaskSucceeded)
	s.Require().NoError(err)
	err = setTaskStatus("task4", evergreen.TaskSucceeded)
	s.Require().NoError(err)

	s.taskQueue.Queue = s.refreshTaskQueue(service)

	err = service.rebuild(s.taskQueue.Queue)
	s.Require().NoError(err)

	// Only "task3" can be dispatched - the remaining task cannot be dispatched as it has an unmet dependency.
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("task3", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)

	// Fake a refresh of the in-memory queue.
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())
	s.Require().NoError(t4.Insert())
	err = setTaskStatus("task2", evergreen.TaskSucceeded)
	s.Require().NoError(err)
	err = setTaskStatus("task3", evergreen.TaskSucceeded)
	s.Require().NoError(err)
	err = setTaskStatus("task4", evergreen.TaskSucceeded)
	s.Require().NoError(err)
	s.taskQueue.Queue = s.refreshTaskQueue(service)

	err = service.rebuild(s.taskQueue.Queue)
	s.Require().NoError(err)

	// Finally, "task1" can be dispatched - all 3 of its dependencies have been satisfied.
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("task1", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)
}

func (s *taskDAGDispatchServiceSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.Require().NoError(db.ClearCollections(task.Collection, host.Collection, VersionCollection))
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

		id := fmt.Sprintf("%d", i)
		items = append(items, TaskQueueItem{
			Id:            id,
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
			Id:                id,
			DistroId:          distroID,
			StartTime:         utility.ZeroTime,
			TaskGroup:         group,
			BuildVariant:      variant,
			Version:           version,
			TaskGroupMaxHosts: maxHosts,
			Project:           project,
			DependsOn:         dependsOn,
			CreateTime:        time.Now(),
		}
		s.Require().NoError(t.Insert())
	}
	taskVersion1 := &Version{
		Id:                   "version_1",
		ProjectStorageMethod: evergreen.ProjectStorageMethodS3,
	}
	taskVersion2 := &Version{
		Id: "version_2",
	}
	taskVersion3 := &Version{
		Id: "5d8cd23da4cf4747f4210333",
	}
	taskVersion4 := &Version{
		Id: "5d88953e2a60ed61eefe9561",
	}
	taskVersion5 := &Version{
		Id: "version",
	}
	taskVersion6 := &Version{
		Id: "",
	}
	s.Require().NoError(taskVersion1.Insert())
	s.Require().NoError(taskVersion2.Insert())
	s.Require().NoError(taskVersion3.Insert())
	s.Require().NoError(taskVersion4.Insert())
	s.Require().NoError(taskVersion5.Insert())
	s.Require().NoError(taskVersion6.Insert())

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
	s.NotEqual(utility.ZeroTime, service.lastUpdated)

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

func (s *taskDAGDispatchServiceSuite) TestSelfEdge() {
	s.Require().NoError(db.ClearCollections(task.Collection))

	t0 := task.Task{
		Id: "t0",
		DependsOn: []task.Dependency{
			{TaskId: "t0"},
		},
	}
	s.Require().NoError(t0.Insert())

	s.taskQueue = TaskQueue{
		Queue: []TaskQueueItem{
			{
				Id:           "t0",
				Dependencies: []string{"t0"},
			},
		},
	}

	dispatcher, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)

	nextTask := dispatcher.FindNextTask(s.ctx, TaskSpec{}, time.Time{})
	s.Nil(nextTask)
}

func (s *taskDAGDispatchServiceSuite) TestDependencyCycle() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	for _, t := range []task.Task{
		{
			Id:        "t0",
			DependsOn: []task.Dependency{{TaskId: "t1"}},
		},
		{
			Id:        "t1",
			DependsOn: []task.Dependency{{TaskId: "t0"}},
		},
		{
			Id: "t2",
		},
	} {
		s.Require().NoError(t.Insert())
	}

	dispatcher, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)

	s.taskQueue = TaskQueue{
		Queue: s.refreshTaskQueue(dispatcher),
	}

	nextTask := dispatcher.FindNextTask(s.ctx, TaskSpec{}, time.Time{})
	s.Require().NotNil(nextTask)
	s.Equal("t2", nextTask.Id)
}

func (s *taskDAGDispatchServiceSuite) TestAddingEdgeWithMissingNodes() {
	s.Require().NoError(db.ClearCollections(task.Collection))

	t1 := task.Task{
		Id:                  "1",
		BuildId:             "ops_manager_kubernetes_init_test_run_patch_1a53e026e05561c3efbb626185e155a7d1e4865d_5d88953e2a60ed61eefe9561_19_09_23_09_49_51",
		TaskGroup:           "",
		StartTime:           utility.ZeroTime,
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

	t2 := task.Task{
		Id:                  "2",
		BuildId:             "ops_manager_kubernetes_e2e_openshift_cloud_qa_patch_1a53e026e05561c3efbb626185e155a7d1e4865d_5d88953e2a60ed61eefe9561_19_09_23_09_49_51",
		TaskGroup:           "e2e_core_task_group",
		TaskGroupMaxHosts:   5,
		TaskGroupOrder:      2,
		StartTime:           utility.ZeroTime,
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

	t3 := task.Task{
		Id:                  "3",
		BuildId:             "ops_manager_kubernetes_e2e_openshift_cloud_qa_patch_1a53e026e05561c3efbb626185e155a7d1e4865d_5d88953e2a60ed61eefe9561_19_09_23_09_49_51",
		TaskGroup:           "e2e_core_task_group",
		TaskGroupMaxHosts:   5,
		TaskGroupOrder:      1,
		StartTime:           utility.ZeroTime,
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

	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.taskQueue = TaskQueue{
		Distro: "archlinux-test",
		Queue:  s.refreshTaskQueue(service),
	}

	spec := TaskSpec{}

	next := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("1", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)

	t1.Status = evergreen.TaskSucceeded

	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())

	service, err = newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.taskQueue = TaskQueue{
		Distro: "archlinux-test",
		Queue:  s.refreshTaskQueue(service),
	}

	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("3", next.Id)

	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("2", next.Id)

	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)

	t2.DependsOn = []task.Dependency{
		{
			TaskId:       "1", // A Task.Id that will not be in the task_queue.
			Status:       evergreen.TaskSucceeded,
			Unattainable: false,
		},
	}

	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())

	service, err = newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.taskQueue = TaskQueue{
		Distro: "archlinux-test",
		Queue:  s.refreshTaskQueue(service),
	}

	// There is no Node for the <to> task.Id: "5" in the task_queue.
	err = service.addEdge("2", "5")
	s.Error(err)
	s.Contains(err.Error(), "is not present in the DAG", nil)

	// There is no Node for the <from> task.Id: "5" in the task_queue.
	err = service.addEdge("5", "2")
	s.NoError(err)

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

	spec = TaskSpec{}

	service, err = newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.taskQueue = TaskQueue{
		Distro: "archlinux-test",
		Queue:  s.refreshTaskQueue(service),
	}

	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("3", next.Id)
}

func (s *taskDAGDispatchServiceSuite) TestNextTaskForDefaultTaskSpec() {
	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	spec := TaskSpec{}
	s.NoError(err)
	s.refreshTaskQueue(service)
	next := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	// First, a standalone task
	s.Equal("0", next.Id)
	// Then all 20 tasks from "group_1_variant_1_project_1_version_1"
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("1", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("6", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("11", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("16", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("21", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("26", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("31", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("36", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("41", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("46", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("51", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("56", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("61", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("66", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("71", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("76", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("81", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("86", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("91", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("96", next.Id)
	// The all the tasks from "group_2_variant_1_project_1_version_1"
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("2", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("7", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.NotNil(next)
	s.Equal("12", next.Id)
	// .....
}

func (s *taskDAGDispatchServiceSuite) TestIsRefreshFindNextTaskThreadSafe() {
	s.Require().NoError(db.ClearCollections(task.Collection, distro.Collection, TaskQueuesCollection))
	d := distro.Distro{
		Id: "distro_1",
		DispatcherSettings: distro.DispatcherSettings{
			Version: evergreen.DispatcherVersionRevisedWithDependencies,
		},
	}
	s.Require().NoError(d.Insert(s.ctx))

	items := []TaskQueueItem{}
	for i := 0; i < 50; i++ {
		items = append(items, TaskQueueItem{
			Id:            fmt.Sprintf("%d", i),
			BuildVariant:  "variant_1",
			Version:       "version_1",
			Project:       "project_1",
			GroupMaxHosts: 0,
		})
		t := task.Task{
			Id:                fmt.Sprintf("%d", i),
			BuildVariant:      "variant_1",
			Version:           "version_1",
			Project:           "project_1",
			DistroId:          "distro_1",
			TaskGroupMaxHosts: 0,
		}
		s.Require().NoError(t.Insert())
	}
	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Nanosecond)
	s.NoError(err)
	s.taskQueue.Queue = s.refreshTaskQueue(service)
	s.Require().NoError(s.taskQueue.Save())
	service.lastUpdated = time.Now().Add(-1 * time.Second)
	dispatcher := &taskDispatchService{
		cachedDispatchers: map[string]CachedDispatcher{
			"distro_1": service,
		},
	}
	spec := TaskSpec{
		BuildVariant: "variant_1",
		Version:      "version_1",
		Project:      "project_1",
	}

	var wg sync.WaitGroup
	wait := make(chan struct{})
	numGoroutines := 50
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			<-wait
			s.NotPanics(func() {
				item, err := dispatcher.RefreshFindNextTask(s.ctx, "distro_1", spec, utility.ZeroTime)
				s.Require().NoError(err)
				s.Require().NotNil(item)
			})
		}()
	}
	close(wait)
	wg.Wait()
}

func (s *taskDAGDispatchServiceSuite) TestFindNextTaskThreadSafe() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	items := []TaskQueueItem{}
	for i := 0; i < 100; i++ {
		items = append(items, TaskQueueItem{
			Id:            fmt.Sprintf("%d", i),
			BuildVariant:  "variant_1",
			Version:       "version_1",
			Project:       "project_1",
			GroupMaxHosts: 0,
		})
		t := task.Task{
			Id:                fmt.Sprintf("%d", i),
			BuildVariant:      "variant_1",
			Version:           "version_1",
			Project:           "project_1",
			TaskGroupMaxHosts: 0,
		}
		s.Require().NoError(t.Insert())
	}

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.taskQueue.Queue = s.refreshTaskQueue(service)
	s.NoError(err)
	spec := TaskSpec{
		BuildVariant: "variant_1",
		Version:      "version_1",
		Project:      "project_1",
	}

	numGoroutines := 100
	dispatchedTasks := map[string]bool{}
	var wg sync.WaitGroup
	var mu sync.RWMutex
	wait := make(chan struct{})
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-wait
			item := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
			s.Require().NotNil(item)
			mu.Lock()
			dispatchedTasks[item.Id] = true
			mu.Unlock()
		}()
	}
	close(wait)
	wg.Wait()

	dispatchedCount := 0
	for _, item := range service.nodeItemMap {
		if item.IsDispatched {
			dispatchedCount++
		}
	}
	s.Equal(len(dispatchedTasks), numGoroutines)
	s.Equal(dispatchedCount, numGoroutines)
}

func (s *taskDAGDispatchServiceSuite) TestFindNextTaskGroupTaskThreadSafe() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	items := []TaskQueueItem{}
	for i := 0; i < 20; i++ {
		groupNum := i / 5
		id := fmt.Sprintf("%d", i)
		items = append(items, TaskQueueItem{
			Id:            id,
			Group:         fmt.Sprintf("group_%d", groupNum),
			BuildVariant:  "variant_1",
			Version:       "version_1",
			Project:       "project_1",
			GroupMaxHosts: 1,
		})
		t := task.Task{
			Id:                id,
			TaskGroup:         fmt.Sprintf("group_%d", groupNum),
			BuildVariant:      "variant_1",
			Version:           "version_1",
			TaskGroupMaxHosts: 1,
			Project:           "project_1",
			StartTime:         utility.ZeroTime,
			FinishTime:        utility.ZeroTime,
		}
		s.Require().NoError(t.Insert())
	}

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.taskQueue.Queue = s.refreshTaskQueue(service)

	// Populating the Group field on the task spec indicates the host just ran a task group.
	spec := TaskSpec{
		Group:        "group_1",
		BuildVariant: "variant_1",
		Version:      "version_1",
		Project:      "project_1",
	}

	numGoroutines := 20
	var wg sync.WaitGroup
	var mu sync.RWMutex
	wait := make(chan struct{})
	wg.Add(numGoroutines)
	dispatchedTasks := map[string]bool{}
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			<-wait
			item := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
			s.Require().NotNil(item)
			mu.Lock()
			dispatchedTasks[item.Id] = true
			mu.Unlock()
		}()
	}
	close(wait)
	wg.Wait()

	dispatchedCount := 0
	for _, item := range service.nodeItemMap {
		if item.IsDispatched {
			dispatchedCount++
		}
	}
	s.Equal(len(dispatchedTasks), numGoroutines)
	s.Equal(dispatchedCount, numGoroutines)

	s.Require().NoError(db.ClearCollections(task.Collection))
	items = []TaskQueueItem{}
	for i := 0; i < 20; i++ {
		groupNum := i / 5
		id := fmt.Sprintf("%d", i)
		items = append(items, TaskQueueItem{
			Id:            id,
			Group:         fmt.Sprintf("group_%d", groupNum),
			BuildVariant:  "variant_1",
			Version:       "version_1",
			Project:       "project_1",
			GroupMaxHosts: 1,
		})
		t := task.Task{
			Id:                id,
			TaskGroup:         fmt.Sprintf("group_%d", groupNum),
			BuildVariant:      "variant_1",
			Version:           "version_1",
			TaskGroupMaxHosts: 1,
			Project:           "project_1",
			StartTime:         utility.ZeroTime,
			FinishTime:        utility.ZeroTime,
		}
		s.Require().NoError(t.Insert())
	}

	service, err = newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.taskQueue.Queue = s.refreshTaskQueue(service)

	// Set the Group field on the task spec to empty, indicating the host is running the task group for the first time.
	spec = TaskSpec{
		Group:        "",
		BuildVariant: "variant_1",
		Version:      "version_1",
		Project:      "project_1",
	}
	wait = make(chan struct{})
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			<-wait
			item := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
			s.Require().NotNil(item)
			mu.Lock()
			dispatchedTasks[item.Id] = true
			mu.Unlock()
		}()
	}
	close(wait)
	wg.Wait()

	dispatchedCount = 0
	for _, item := range service.nodeItemMap {
		if item.IsDispatched {
			dispatchedCount++
		}
	}
	s.Equal(len(dispatchedTasks), numGoroutines)
	s.Equal(dispatchedCount, numGoroutines)
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
			startTime = utility.ZeroTime
			endTime = utility.ZeroTime
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
	next := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)
}

func setTaskStatus(taskID string, status string) error {
	return task.UpdateOne(
		bson.M{
			task.IdKey: taskID,
		},
		bson.M{
			"$set": bson.M{
				task.StatusKey: status,
			},
		},
	)
}

func (s *taskDAGDispatchServiceSuite) TestFindNextTask() {
	service, e := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(e)
	s.taskQueue.Queue = s.refreshTaskQueue(service)
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
		next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
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
		next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
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
		next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
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
		next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
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
		next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
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
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Equal("0", next.Id)
	s.Equal("", next.Group)
	s.Require().NoError(setTaskStatus(next.Id, evergreen.TaskSucceeded))

	currentID := 0
	var nextInt int
	var err error

	// Make another 10 requests for a task, passing an "empty" TaskSpec{} - all 10 dispatched tasks should come from the "group_1_variant_1_project_1_version_1" taskGroupTasks.
	// task ids: ["51", "56", "61", "66", "71", "76", "81", "86", "91", "96"]
	// All 20 tasks for taskGroupTasks "group_1_variant_1_project_1_version_1" have been dispatched.
	for i := 0; i < 10; i++ {
		next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
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
		next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
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
		next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
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
		next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
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
		next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
		s.Equal(expectedStandaloneTaskOrder[i], next.Id)
		s.Equal("", next.Group)
		s.Require().NoError(setTaskStatus(next.Id, evergreen.TaskSucceeded))
		s.taskQueue.Queue = s.refreshTaskQueue(service)
	}
}

func (s *taskDAGDispatchServiceSuite) TestFindNextTaskForOutdatedHostAMI() {
	s.Require().NoError(db.ClearCollections(task.Collection))

	amiUpdateTime := time.Now()
	t1 := task.Task{
		Id:                  "1",
		BuildId:             "ops_manager_kubernetes_init_test_run_patch_1a53e026e05561c3efbb626185e155a7d1e4865d_5d88953e2a60ed61eefe9561_19_09_23_09_49_51",
		TaskGroup:           "",
		IngestTime:          amiUpdateTime.Add(time.Minute), // created after the AMI was updated so we should skip
		StartTime:           utility.ZeroTime,
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

	t2 := task.Task{
		Id:                  "2",
		BuildId:             "ops_manager_kubernetes_e2e_openshift_cloud_qa_patch_1a53e026e05561c3efbb626185e155a7d1e4865d_5d88953e2a60ed61eefe9561_19_09_23_09_49_51",
		TaskGroup:           "e2e_core_task_group",
		TaskGroupMaxHosts:   5,
		TaskGroupOrder:      2,
		StartTime:           utility.ZeroTime,
		IngestTime:          amiUpdateTime.Add(-time.Minute), // created before the AMI was updated so we should not skip
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
	}

	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.taskQueue = TaskQueue{
		Distro: "archlinux-test",
		Queue:  s.refreshTaskQueue(service),
	}
	item := service.FindNextTask(s.ctx, TaskSpec{}, amiUpdateTime)
	s.Equal(item.Id, t2.Id)

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
	s.Require().NoError(h1.Insert(s.ctx))

	service, e := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(e)

	s.taskQueue.Queue = s.refreshTaskQueue(service)
	spec := TaskSpec{}
	next := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Equal("0", next.Id)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	// The next task, according to the order of basicCachedDAGDispatcherImpl.sorted is from task group "group_1_variant_1_version_1".
	// However, runningHosts < maxHosts is false for this task group, so we cannot dispatch this task.
	s.NotEqual("1", next.Id)
	// Instead, return the next task, which is from task group "group_2_variant_1_project_1_version_1".
	s.Equal("2", next.Id)
	s.Equal("group_2", next.Group)
	s.Equal("variant_1", next.BuildVariant)
	s.Equal("version_1", next.Version)
	s.Equal("project_1", next.Project)
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
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
	s.taskQueue.Queue = s.refreshTaskQueue(service)
	taskGroup := service.taskGroups[taskGroupID]
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal(expectedOrder[0], next.Id)
	s.Equal("1", taskGroup.tasks[0].Id)
	s.Equal(false, taskGroup.tasks[0].IsDispatched)
	s.Equal("6", taskGroup.tasks[1].Id)
	s.Equal(true, taskGroup.tasks[1].IsDispatched)
	s.Equal("11", taskGroup.tasks[2].Id)
	s.Equal(false, taskGroup.tasks[2].IsDispatched)

	for i := 1; i < 5; i++ {
		next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
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
		next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
		s.Require().NotNil(next)
		s.Equal(expectedOrder[i], next.Id)
	}

	// All the tasks within taskGroup "group_1_variant_1_project_1_version_1" has now been dispatched.
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal("0", next.Id)
	s.Equal("", next.Group)
}

func (s *taskDAGDispatchServiceSuite) TestSingleHostTaskGroupOrdering() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	items := []TaskQueueItem{}
	groupIndexes := []int{2, 0, 4, 1, 3}

	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("%d", i)
		items = append(items, TaskQueueItem{
			Id:            id,
			Group:         "group_1",
			BuildVariant:  "variant_1",
			Version:       "version_1",
			Project:       "project_1",
			GroupMaxHosts: 1,
			GroupIndex:    groupIndexes[i],
		})
		t := task.Task{
			Id:                id,
			TaskGroup:         "group_1",
			BuildVariant:      "variant_1",
			Version:           "version_1",
			TaskGroupMaxHosts: 1,
			Project:           "project_1",
			StartTime:         utility.ZeroTime,
			FinishTime:        utility.ZeroTime,
		}
		s.Require().NoError(t.Insert())

		s.taskQueue = TaskQueue{
			Distro: "distro_1",
			Queue:  items,
		}
	}

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.Require().NoError(err)
	s.taskQueue.Queue = s.refreshTaskQueue(service)
	for i := 0; i < 5; i++ {
		s.taskQueue.Queue[i].GroupIndex = groupIndexes[i]
	}
	err = service.rebuild(s.taskQueue.Queue)
	s.Require().NoError(err)

	spec := TaskSpec{
		Group:        "group_1",
		BuildVariant: "variant_1",
		Version:      "version_1",
		Project:      "project_1",
	}
	expectedOrder := []string{"1", "3", "0", "4", "2"}

	for i := 0; i < 5; i++ {
		next := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
		s.Require().NotNil(next)
		s.Equal(expectedOrder[i], next.Id)
	}
}

func (s *taskDAGDispatchServiceSuite) TestInProgressSingleHostTaskGroupLimits() {
	s.Require().NoError(db.ClearCollections(task.Collection, evergreen.ConfigCollection))

	settings := evergreen.TaskLimitsConfig{
		MaxDegradedModeConcurrentLargeParserProjectTasks: 1,
	}
	s.Require().NoError(settings.Set(s.ctx))

	sampleS3Task := task.Task{
		Id:                         "sample_s3_task",
		Version:                    "version_1",
		Project:                    "project_1",
		Status:                     evergreen.TaskStarted,
		CachedProjectStorageMethod: evergreen.ProjectStorageMethodS3,
	}
	s.Require().NoError(sampleS3Task.Insert())

	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("%d", i)
		t := task.Task{
			Id:                         id,
			TaskGroup:                  "group_1",
			BuildVariant:               "variant_1",
			Version:                    "version_1",
			TaskGroupMaxHosts:          1,
			Project:                    "project_1",
			StartTime:                  utility.ZeroTime,
			FinishTime:                 utility.ZeroTime,
			CachedProjectStorageMethod: evergreen.ProjectStorageMethodS3,
		}
		s.Require().NoError(t.Insert())
	}

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.Require().NoError(err)
	s.taskQueue = TaskQueue{
		Distro: "distro_1",
		Queue:  s.refreshTaskQueue(service),
	}
	spec := TaskSpec{
		Group:        "group_1",
		BuildVariant: "variant_1",
		Version:      "version_1",
		Project:      "project_1",
	}

	for i := 0; i < 5; i++ {
		next := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
		s.Require().NotNil(next)
	}
}

func (s *taskDAGDispatchServiceSuite) TestNewSingleHostTaskGroupLimits() {
	defer evergreen.SetEnvironment(evergreen.GetEnvironment())

	s.Require().NoError(db.ClearCollections(task.Collection, evergreen.ConfigCollection))

	mockEnv := &mock.Environment{}
	s.Require().NoError(mockEnv.Configure(s.ctx))
	mockEnv.EvergreenSettings.TaskLimits = evergreen.TaskLimitsConfig{
		MaxDegradedModeConcurrentLargeParserProjectTasks: 1,
	}
	evergreen.SetEnvironment(mockEnv)

	sampleS3Task := task.Task{
		Id:                         "sample_s3_task",
		Version:                    "version_1",
		Project:                    "project_1",
		Status:                     evergreen.TaskStarted,
		CachedProjectStorageMethod: evergreen.ProjectStorageMethodS3,
	}
	s.Require().NoError(sampleS3Task.Insert())

	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("%d", i)
		t := task.Task{
			Id:                         id,
			TaskGroup:                  "group_1",
			BuildVariant:               "variant_1",
			Version:                    "version_1",
			TaskGroupMaxHosts:          1,
			Project:                    "project_1",
			StartTime:                  utility.ZeroTime,
			FinishTime:                 utility.ZeroTime,
			CachedProjectStorageMethod: evergreen.ProjectStorageMethodS3,
		}
		s.Require().NoError(t.Insert())
	}

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.Require().NoError(err)
	s.taskQueue = TaskQueue{
		Distro: "distro_1",
		Queue:  s.refreshTaskQueue(service),
	}
	spec := TaskSpec{}
	for i := 0; i < 5; i++ {
		next := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
		s.Require().Nil(next)
	}
}

func (s *taskDAGDispatchServiceSuite) TestGenerateTaskLimits() {
	defer evergreen.SetEnvironment(evergreen.GetEnvironment())

	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(db.ClearCollections(host.Collection))
	s.Require().NoError(db.ClearCollections(evergreen.ConfigCollection))

	distroID := "distro_1"

	mockEnv := &mock.Environment{}
	s.Require().NoError(mockEnv.Configure(s.ctx))
	mockEnv.EvergreenSettings.TaskLimits = evergreen.TaskLimitsConfig{
		MaxPendingGeneratedTasks: 6,
	}
	evergreen.SetEnvironment(mockEnv)

	running := task.Task{
		Id:                         "running",
		BuildId:                    "build_id",
		StartTime:                  time.Now(),
		BuildVariant:               "bv",
		Version:                    "version",
		Project:                    "proj",
		Activated:                  true,
		ActivatedBy:                "",
		DistroId:                   distroID,
		Requester:                  "github_pull_request",
		Status:                     evergreen.TaskStarted,
		Revision:                   "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		EstimatedNumGeneratedTasks: utility.ToIntPtr(3),
	}

	t1 := task.Task{
		Id:                         "t1",
		BuildId:                    "build_id",
		StartTime:                  utility.ZeroTime,
		BuildVariant:               "bv",
		Version:                    "version",
		Project:                    "proj",
		Activated:                  true,
		ActivatedBy:                "",
		DistroId:                   distroID,
		Requester:                  "github_pull_request",
		Status:                     evergreen.TaskUndispatched,
		Revision:                   "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		EstimatedNumGeneratedTasks: utility.ToIntPtr(4),
	}
	t2 := task.Task{
		Id:                         "t2",
		BuildId:                    "build_id",
		StartTime:                  utility.ZeroTime,
		BuildVariant:               "bv",
		Version:                    "version",
		Project:                    "proj",
		Activated:                  true,
		ActivatedBy:                "",
		DistroId:                   distroID,
		Requester:                  "github_pull_request",
		Status:                     evergreen.TaskUndispatched,
		Revision:                   "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
		EstimatedNumGeneratedTasks: utility.ToIntPtr(2),
	}
	t3 := task.Task{
		Id:           "t3",
		BuildId:      "build_id",
		StartTime:    utility.ZeroTime,
		BuildVariant: "bv",
		Version:      "version",
		Project:      "proj",
		Activated:    true,
		ActivatedBy:  "",
		DistroId:     distroID,
		Requester:    "github_pull_request",
		Status:       evergreen.TaskUndispatched,
		Revision:     "6273aa2072f8325b8d1ceae2dfff74a775b018fc",
	}
	s.Require().NoError(running.Insert())
	s.Require().NoError(t1.Insert())
	s.Require().NoError(t2.Insert())
	s.Require().NoError(t3.Insert())

	service, err := newDistroTaskDAGDispatchService(s.taskQueue, time.Minute)
	s.NoError(err)
	s.taskQueue = TaskQueue{
		Distro: distroID,
		Queue:  s.refreshTaskQueue(service),
	}
	spec := TaskSpec{}

	// Next task should be t2 since t1 exceeds the generate tasks limit,
	next := service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal(t2.Id, next.Id)

	// Mark running task as complete so that t1 can be dispatched.
	s.Require().NoError(running.MarkEnd(time.Now(), nil))

	// Fake a refresh of the in-memory queue.
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(t1.Insert())
	s.Require().NoError(t3.Insert())

	s.taskQueue.Queue = s.refreshTaskQueue(service)
	s.Require().NoError(service.rebuild(s.taskQueue.Queue))

	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal(t1.Id, next.Id)

	// Tasks without estimated generated tasks should not be limited.
	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().NotNil(next)
	s.Equal(t3.Id, next.Id)

	next = service.FindNextTask(s.ctx, spec, utility.ZeroTime)
	s.Require().Nil(next)
}

func (s *taskDAGDispatchServiceSuite) refreshTaskQueue(service *basicCachedDAGDispatcherImpl) []TaskQueueItem {
	tasks, err := task.FindAll(db.Query(bson.M{task.StatusKey: bson.M{"$nin": evergreen.TaskCompletedStatuses}}))
	s.Require().NoError(err)
	taskQueue := make([]TaskQueueItem, 0, len(tasks))
	for _, t := range tasks {
		_, err = t.DependenciesMet(map[string]task.Task{})
		s.Require().NoError(err)
		dependencies := make([]string, 0, len(t.DependsOn))
		for _, d := range t.DependsOn {
			dependencies = append(dependencies, d.TaskId)
		}
		taskQueue = append(taskQueue, TaskQueueItem{
			Id:                    t.Id,
			DisplayName:           t.DisplayName,
			BuildVariant:          t.BuildVariant,
			RevisionOrderNumber:   t.RevisionOrderNumber,
			Requester:             t.Requester,
			Revision:              t.Revision,
			Project:               t.Project,
			ExpectedDuration:      t.ExpectedDuration,
			Priority:              t.Priority,
			SortingValueBreakdown: t.SortingValueBreakdown,
			Group:                 t.TaskGroup,
			GroupMaxHosts:         t.TaskGroupMaxHosts,
			GroupIndex:            t.TaskGroupOrder,
			Version:               t.Version,
			ActivatedBy:           t.ActivatedBy,
			Dependencies:          dependencies,
			DependenciesMet:       t.HasDependenciesMet(),
		})
	}
	err = service.rebuild(taskQueue)
	s.Require().NoError(err)

	return taskQueue
}
