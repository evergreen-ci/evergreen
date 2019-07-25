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
)

type taskDispatchServiceSuite struct {
	suite.Suite

	items []TaskQueueItem
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

	for i := 0; i < 100; i++ {
		project := "project_1"
		if i%5 == 0 { // no group
			group = ""
			variant = "variant_1"
			version = "version_1"
			maxHosts = 0
		} else if i%5 == 1 { // group 1
			group = "group_1"
			variant = "variant_1"
			version = "version_1"
			maxHosts = 1
		} else if i%5 == 2 { // group 2
			group = "group_2"
			variant = "variant_1"
			version = "version_1"
			maxHosts = 2
		} else if i%5 == 3 { // different variant
			group = "group_1"
			variant = "variant_2"
			version = "version_1"
			maxHosts = 2
		} else if i%5 == 4 { // different version
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

	s.items = items
}

func (s *taskDispatchServiceSuite) TestConstructor() {
	service := newDistroTaskDispatchService("distro_1", s.items, time.Minute)

	//////////////////////////////////////////////////////////////////////////////
	// basicCachedDispatcherImpl.order[0] = "0"
	// basicCachedDispatcherImpl.order[1] = "group_1-variant_1-version_1"
	// basicCachedDispatcherImpl.order[2] = "group_2-variant_1-version_1"
	// basicCachedDispatcherImpl.order[3] = "group_1-variant_2-version_1"
	// basicCachedDispatcherImpl.order[4] = "group_1-variant_1-version_2"
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
	// basicCachedDispatcherImpl.units["group_1-variant_1-version_1"].len(tasks) = 20
	//	[1, 6, 11, 16, 21, 26, 31, 36, 41, 46, 51, 56, 61, 66, 71, 76, 81, 86, 91, 96]
	// basicCachedDispatcherImpl.units["group_1-variant_1-version_1"].maxHosts = 1
	// basicCachedDispatcherImpl.units["group_2-variant_1-version_1"].len(tasks) = 20
	//	[2, 7, 12, 17, 22, 27, 32, 37, 42, 47, 52, 57, 62, 67, 72, 77, 82, 87, 92, 97]
	// basicCachedDispatcherImpl.units["group_2-variant_1-version_1"].maxHosts = 2
	// basicCachedDispatcherImpl.units["group_1-variant_1-version_2"].len(tasks) = 20
	// basicCachedDispatcherImpl.units["group_1-variant_1-version_2"].maxHosts = 2
	// basicCachedDispatcherImpl.units["group_1-variant_2-version_1"].len(tasks) = 20
	// basicCachedDispatcherImpl.units[""group_1-variant_2-version_1"].maxHosts = 2
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
	s.Len(service.order, 24, "20 bare tasks + 4 task groups")
	s.Equal(len(service.order), len(service.units), "order and units should have same length")
	s.Equal("distro_1", service.distroID)
	s.Equal(60*time.Second, service.ttl)
	s.NotEqual(util.ZeroTime, service.lastUpdated)

	s.Contains(service.order, compositeGroupId("group_1", "variant_1", "project_1", "version_1"))
	s.Contains(service.units, compositeGroupId("group_1", "variant_1", "project_1", "version_1"))
	s.Len(service.units[compositeGroupId("group_1", "variant_1", "project_1", "version_1")].tasks, 20)
	s.Equal(1, service.units[compositeGroupId("group_1", "variant_1", "project_1", "version_1")].maxHosts)

	s.Contains(service.order, compositeGroupId("group_2", "variant_1", "project_1", "version_1"))
	s.Contains(service.units, compositeGroupId("group_2", "variant_1", "project_1", "version_1"))
	s.Len(service.units[compositeGroupId("group_2", "variant_1", "project_1", "version_1")].tasks, 20)
	s.Equal(2, service.units[compositeGroupId("group_2", "variant_1", "project_1", "version_1")].maxHosts)

	s.Contains(service.order, compositeGroupId("group_1", "variant_2", "project_1", "version_1"))
	s.Contains(service.units, compositeGroupId("group_1", "variant_2", "project_1", "version_1"))
	s.Len(service.units[compositeGroupId("group_1", "variant_2", "project_1", "version_1")].tasks, 20)
	s.Equal(2, service.units[compositeGroupId("group_1", "variant_2", "project_1", "version_1")].maxHosts)

	s.Contains(service.order, compositeGroupId("group_1", "variant_1", "project_1", "version_2"))
	s.Contains(service.units, compositeGroupId("group_1", "variant_1", "project_1", "version_2"))
	s.Len(service.units[compositeGroupId("group_1", "variant_1", "project_1", "version_2")].tasks, 20)
	s.Equal(2, service.units[compositeGroupId("group_1", "variant_1", "project_1", "version_2")].maxHosts)

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
	service := newDistroTaskDispatchService("distro_1", []TaskQueueItem{
		{
			Id:    "a-standalone-task",
			Group: "",
		},
	}, time.Minute)
	next := service.FindNextTask(TaskSpec{})
	s.Require().NotNil(next)
	s.Equal("a-standalone-task", next.Id)
	next = service.FindNextTask(TaskSpec{})
	s.Nil(next)
	s.Empty(service.order) // slice is emptied when map is emptied
}

func (s *taskDispatchServiceSuite) TestSingleHostTaskGroupsBlock() {
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
	service := newDistroTaskDispatchService("distro_1", items, time.Minute)
	spec := TaskSpec{
		Group:        "group_1",
		BuildVariant: "variant_1",
		ProjectID:    "project_1",
		Version:      "version_1",
	}
	next := service.FindNextTask(spec)
	s.Nil(next)
	s.Empty(service.units)
}

func (s *taskDispatchServiceSuite) TestFindNextTask() {
	service := newDistroTaskDispatchService("distro_1", s.items, time.Minute)
	var spec TaskSpec
	var next *TaskQueueItem
	//////////////////////////////////////////////////////////////////////////////
	// basicCachedDispatcherImpl.order[0] = "0"
	// basicCachedDispatcherImpl.order[1] = "group_1-variant_1-version_1"
	// basicCachedDispatcherImpl.order[2] = "group_2-variant_1-version_1"
	// basicCachedDispatcherImpl.order[3] = "group_1-variant_2-version_1"
	// basicCachedDispatcherImpl.order[4] = "group_1-variant_1-version_2"
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
	// basicCachedDispatcherImpl.units["group_1-variant_1-version_1"].len(tasks) = 20
	// basicCachedDispatcherImpl.units["group_2-variant_1-version_1"].len(tasks) = 20
	// basicCachedDispatcherImpl.units["group_1-variant_1-version_2"].len(tasks) = 20
	// basicCachedDispatcherImpl.units["group_1-variant_2-version_1"].len(tasks) = 20
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

	// Dispatch the first 5 tasks for the schedulableUnit "group_1-variant_1-version_1", which represents a task group that initially contains 20 tasks.
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_1",
			Version:      "version_1",
			ProjectID:    "project_1",
		}
		next = service.FindNextTask(spec)
		s.Require().NotNil(next)
		s.Equal(fmt.Sprintf("%d", 5*i+1), next.Id)
	}

	// Dispatch the first 5 tasks for schedulableUnit "group_2-variant_1-version_1", which represents a task group that initially contains 20 tasks.
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_2",
			BuildVariant: "variant_1",
			Version:      "version_1",
			ProjectID:    "project_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+2), next.Id)
	}

	// Dispatch the first 5 tasks for schedulableUnit "group_1-variant_2-version_1", which represents a task group that initially contains 20 tasks.
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_2",
			Version:      "version_1",
			ProjectID:    "project_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+3), next.Id)
	}

	// Dispatch the first 5 tasks for schedulableUnit "group_1-variant_1-version_2", which represents a task group that initially contains 20 tasks.
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_1",
			Version:      "version_2",
			ProjectID:    "project_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+4), next.Id)
	}

	// The task group schedulableUnit "group_1-variant_1-version_1" now contains 15 tasks; dispatch another 5 of them.
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_1",
			Version:      "version_1",
			ProjectID:    "project_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+26), next.Id)
	}

	//////////////////////////////////////////////////////////////////////////////
	// Repeat requests for tasks by a TaskSpec containing an empty Group field dispatch, in order:
	// (1) A single standalone (non-task group) task
	// (2) The rest of the task group tasks for schedulableUnit "group_1-variant_1-version_1"
	// (3) The rest of the task group tasks for schedulableUnit "group_2-variant_1-version_1"
	// (4) The rest of the task group tasks for schedulableUnit "group_1-variant_1-version_2"
	// (4) The rest of the task group tasks for schedulableUnit "group_1-variant_2-version_1"
	// (5) All the remaining schedulableUnits representing individual, standalone tasks

	spec = TaskSpec{
		Group:        "",
		BuildVariant: "variant_1",
		Version:      "version_1",
		ProjectID:    "project_2",
	}
	next = service.FindNextTask(spec)
	s.Equal("0", next.Id)
	currentID := 0
	var nextInt int
	var err error

	// Dispatch the remaining 10 tasks from the task group schedulableUnit "group_1-variant_1-version_1".
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

	// All 20 tasks for schedulableUnit "group_1-variant_1-version_1" have been dispatched.
	// basicCachedDispatcherImpl.order's ([]string) next value is "group_2-variant_1-version_1".
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

	// All 20 tasks for schedulableUnit "group_2-variant_1-version_1" have been dispatched.
	// basicCachedDispatcherImpl.order's ([]string) next value is "group_1-variant_2-version_1".
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

	// All 20 tasks for schedulableUnit ""group_1-variant_2-version_1"" have been dispatched.
	// basicCachedDispatcherImpl.order's ([]string) next value is "group_1-variant_1-version_2".
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

	// The remaining schedulableUnits represent individual, standalone tasks. Dispatch the rest of these non-task group tasks.
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

	// Add a host which that would request a task with a TaskSpec resolving to "group_1-variant_1-version_1"
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

	spec := TaskSpec{
		Group:         "",
		BuildVariant:  "",
		ProjectID:     "",
		Version:       "",
		GroupMaxHosts: 0,
	}

	service := newDistroTaskDispatchService("distro_1", s.items, time.Minute)
	//////////////////////////////////////////////////////////////////////////////
	// basicCachedDispatcherImpl.order[0] = "0"
	// basicCachedDispatcherImpl.order[1] = "group_1-variant_1-version_1"
	// basicCachedDispatcherImpl.order[2] = "group_2-variant_1-version_1"
	//////////////////////////////////////////////////////////////////////////////
	// basicCachedDispatcherImpl.units["0"].len(tasks) = 1
	//	[0]
	// basicCachedDispatcherImpl.units["group_1-variant_1-version_1"].len(tasks) = 20
	// 	[1, 6, 11, 16, 21, 26, 31, 36, 41, 46, 51, 56, 61, 66, 71, 76, 81, 86, 91, 96]
	// basicCachedDispatcherImpl.units[group_1-variant_1-version_1].maxHosts = 1
	// basicCachedDispatcherImpl.units["group_2-variant_1-version_1"].len(tasks) = 20
	//	[2, 7, 12, 17, 22, 27, 32, 37, 42, 47, 52, 57, 62, 67, 72, 77, 82, 87, 92, 97]
	// basicCachedDispatcherImpl.units["group_2-variant_1-version_1"].maxHosts = 2
	//////////////////////////////////////////////////////////////////////////////
	next := service.FindNextTask(spec)
	s.Equal("0", next.Id)

	// basicCachedDispatcherImpl.order's ([]string) next value is "group_1-variant_1-version_1".
	// However, runningHosts < maxHosts is false for its corresponding schedulableUnit, so we cannot dispatch one of its tasks
	// On to basicCachedDispatcherImpl.order's next value: "group_2-variant_1-version_1" - we can dispatch a task from its corresponding schedulableUnit
	next = service.FindNextTask(spec)
	s.Equal("2", next.Id)
	s.Equal("group_2", next.Group)
	s.Equal("variant_1", next.BuildVariant)
	s.Equal("version_1", next.Version)
	s.Equal("project_1", next.Project)
	s.Equal(2, next.GroupMaxHosts)

	// Same situation again - so we dispatch the next task for the schedulableUnit "group_2-variant_1-version_1"
	next = service.FindNextTask(spec)
	s.Equal("7", next.Id)
	s.Equal("group_2", next.Group)
	s.Equal("variant_1", next.BuildVariant)
	s.Equal("version_1", next.Version)
	s.Equal("project_1", next.Project)
	s.Equal(2, next.GroupMaxHosts)
}
