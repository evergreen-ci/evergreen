package model

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
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
	items := []TaskQueueItem{}
	var group string
	var variant string
	var version string
	var maxHosts int

	for i := 0; i < 100; i++ {
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
			Version:       version,
			GroupMaxHosts: maxHosts,
		})
		t := task.Task{
			Id:                fmt.Sprintf("%d", i),
			TaskGroup:         group,
			BuildVariant:      variant,
			Version:           version,
			TaskGroupMaxHosts: maxHosts,
			StartTime:         util.ZeroTime,
			FinishTime:        util.ZeroTime,
		}
		s.Require().NoError(t.Insert())
	}
	s.items = items

}

func (s *taskDispatchServiceSuite) TestConstructor() {
	service := NewDistroTaskDispatchService("distro_1", s.items, time.Minute).(*taskDistroDispatchService)
	s.Len(service.order, 24, "20 bare tasks + 4 task groups")
	s.Len(service.units, 24, "20 bare tasks + 4 task groups")
	s.Equal(len(service.order), len(service.units), "order and units should have same length")

	s.Contains(service.order, compositeGroupId("group_1", "variant_1", "version_1"))
	s.Contains(service.units, compositeGroupId("group_1", "variant_1", "version_1"))
	s.Len(service.units[compositeGroupId("group_1", "variant_1", "version_1")].tasks, 20)

	s.Contains(service.order, compositeGroupId("group_2", "variant_1", "version_1"))
	s.Contains(service.units, compositeGroupId("group_2", "variant_1", "version_1"))
	s.Len(service.units[compositeGroupId("group_2", "variant_1", "version_1")].tasks, 20)

	s.Contains(service.order, compositeGroupId("group_1", "variant_2", "version_1"))
	s.Contains(service.units, compositeGroupId("group_1", "variant_2", "version_1"))
	s.Len(service.units[compositeGroupId("group_1", "variant_2", "version_1")].tasks, 20)

	s.Contains(service.order, compositeGroupId("group_1", "variant_1", "version_2"))
	s.Contains(service.units, compositeGroupId("group_1", "variant_1", "version_2"))
	s.Len(service.units[compositeGroupId("group_1", "variant_1", "version_2")].tasks, 20)

	for i := 0; i < 100; i = i + 5 {
		s.Contains(service.order, fmt.Sprintf("%d", i))
	}
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
	service := NewDistroTaskDispatchService("distro_1", []TaskQueueItem{
		{
			Id:    "0",
			Group: "",
		},
	}, time.Minute).(*taskDistroDispatchService)
	next := service.FindNextTask(TaskSpec{})
	s.Require().NotNil(next)
	s.Equal("0", next.Id)
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
			TaskGroupMaxHosts: 1,
			StartTime:         startTime,
			FinishTime:        endTime,
			Status:            status,
		}
		s.Require().NoError(t.Insert())
	}
	service := NewDistroTaskDispatchService("distro_1", items, time.Minute).(*taskDistroDispatchService)
	spec := TaskSpec{
		Group:        "group_1",
		BuildVariant: "variant_1",
		Version:      "version_1",
	}
	next := service.FindNextTask(spec)
	s.Nil(next)
	s.Empty(service.units)
}

func (s *taskDispatchServiceSuite) TestFindNextTask() {
	service := NewDistroTaskDispatchService("distro_1", s.items, time.Minute)
	var spec TaskSpec
	var next *TaskQueueItem

	// Dispatch 5 tasks from a group
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_1",
			Version:      "version_1",
		}
		next = service.FindNextTask(spec)

		s.Require().NotNil(next)
		s.Equal(fmt.Sprintf("%d", 5*i+1), next.Id)
	}

	// Dispatch 5 tasks from a different group
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_2",
			BuildVariant: "variant_1",
			Version:      "version_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+2), next.Id)
	}

	// Dispatch 5 tasks from a group in another variant
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_2",
			Version:      "version_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+3), next.Id)
	}

	// Dispatch 5 tasks from a group in another variant
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_1",
			Version:      "version_2",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+4), next.Id)
	}

	// Dispatch 5 more tasks from the first group
	for i := 0; i < 5; i++ {
		spec = TaskSpec{
			Group:        "group_1",
			BuildVariant: "variant_1",
			Version:      "version_1",
		}
		next = service.FindNextTask(spec)
		s.Equal(fmt.Sprintf("%d", 5*i+26), next.Id)
	}

	// Dispatch a task with an empty task group should get a non-task group task, then the rest
	// of the task group tasks, then the non-task group tasks again
	spec = TaskSpec{
		Group:        "",
		BuildVariant: "variant_1",
		Version:      "version_1",
	}
	next = service.FindNextTask(spec)
	s.Equal("0", next.Id)
	currentID := 0
	var nextInt int
	var err error
	for i := 0; i < 10; i++ {
		next = service.FindNextTask(spec)
		nextInt, err = strconv.Atoi(next.Id)
		s.NoError(err)
		s.True(nextInt > currentID)
		currentID = nextInt
		s.Equal("group_1", next.Group)
		s.Equal("variant_1", next.BuildVariant)
		s.Equal("version_1", next.Version)
	}
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
	}
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
	}
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
	}

	// Dispatch the rest of the non-task group tasks
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
	}
}
