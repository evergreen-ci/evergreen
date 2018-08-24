package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestGroupByTaskGroup(t *testing.T) {
	assert := assert.New(t)

	// no task groups
	hosts := []host.Host{
		{Id: "h1"},
		{Id: "h2"},
	}
	tasks := []model.TaskQueueItem{
		{Id: "t1"},
		{Id: "t2"},
	}
	groups := groupByTaskGroup(hosts, tasks)
	assert.Len(groups, 1)
	assert.Len(groups[""].hosts, 2)
	assert.Len(groups[""].tasks, 2)

	// some running task groups
	hosts = []host.Host{
		{Id: "h1", RunningTaskGroup: "g1", RunningTask: "foo"},
		{Id: "h2", RunningTaskGroup: "g1", RunningTask: "bar"},
	}
	tasks = []model.TaskQueueItem{
		{Id: "t1", Group: "g2"},
		{Id: "t2"},
	}
	groups = groupByTaskGroup(hosts, tasks)
	assert.Len(groups, 3)
	assert.Len(groups[makeTaskGroupString("g1", "", "", "")].hosts, 2)
	assert.Len(groups[makeTaskGroupString("g1", "", "", "")].tasks, 0)
	assert.Len(groups[makeTaskGroupString("g2", "", "", "")].hosts, 0)
	assert.Len(groups[makeTaskGroupString("g2", "", "", "")].tasks, 1)
	assert.Equal("h1", groups[makeTaskGroupString("g1", "", "", "")].hosts[0].Id)
	assert.Equal("h2", groups[makeTaskGroupString("g1", "", "", "")].hosts[1].Id)
	assert.Len(groups[""].hosts, 0)
	assert.Len(groups[""].tasks, 1)

	// some finished task groups
	hosts = []host.Host{
		{Id: "h1", RunningTaskGroup: "g1"},
		{Id: "h2", RunningTaskGroup: "g1"},
	}
	tasks = []model.TaskQueueItem{
		{Id: "t1", Group: "g2"},
		{Id: "t2"},
	}
	groups = groupByTaskGroup(hosts, tasks)
	assert.Len(groups, 2)
	assert.Len(groups[makeTaskGroupString("g2", "", "", "")].hosts, 2)
	assert.Len(groups[makeTaskGroupString("g2", "", "", "")].tasks, 1)
	assert.Len(groups[""].hosts, 2)
	assert.Len(groups[""].tasks, 1)
}

type UtilizationAllocatorSuite struct {
	ctx              context.Context
	distroName       string
	distro           distro.Distro
	projectName      string
	freeHostFraction float64
	suite.Suite
}

func TestUtilizationAllocatorSuite(t *testing.T) {
	s := &UtilizationAllocatorSuite{}
	suite.Run(t, s)
}

func (s *UtilizationAllocatorSuite) SetupSuite() {
	s.distroName = "testDistro"
	s.distro = distro.Distro{
		Id:       s.distroName,
		PoolSize: 50,
		Provider: evergreen.ProviderNameEc2Auto,
	}
	s.projectName = "testProject"
	s.freeHostFraction = 0.5
}

func (s *UtilizationAllocatorSuite) SetupTest() {
	s.ctx = context.Background()
	s.NoError(db.ClearCollections(task.Collection, host.Collection, distro.Collection))
}

// unit tests for calcuation functions
func (s *UtilizationAllocatorSuite) TestCalcScheduledTasksDuration() {
	queue := []model.TaskQueueItem{
		{
			ExpectedDuration: 5 * time.Minute,
		},
		{
			ExpectedDuration: 15 * time.Minute,
		},
		{
			ExpectedDuration: 2 * time.Hour,
		},
	}
	s.Equal(140*time.Minute, calcScheduledTasksDuration(queue))
}

func (s *UtilizationAllocatorSuite) TestCalcNewHostsNeeded() {
	s.Equal(0, calcNewHostsNeeded(0*time.Second, 30*time.Minute, 0, 0))
	s.Equal(0, calcNewHostsNeeded(0*time.Second, 30*time.Minute, 1, 0))
	s.Equal(1, calcNewHostsNeeded(1*time.Second, 30*time.Minute, 0, 0))
	s.Equal(0, calcNewHostsNeeded(0*time.Second, 30*time.Minute, 1, 0))
	s.Equal(3, calcNewHostsNeeded(3*time.Minute, 1*time.Minute, 0, 0))
	s.Equal(11, calcNewHostsNeeded(6*time.Hour, 30*time.Minute, 1, 0))
	s.Equal(10, calcNewHostsNeeded(80*time.Hour, 30*time.Minute, 150, 0))
	s.Equal(11, calcNewHostsNeeded(80*time.Hour, 30*time.Minute, 150, 1))
}

func (s *UtilizationAllocatorSuite) TestCalcExistingFreeHosts() {
	h1 := host.Host{
		Id:          "h1",
		RunningTask: "t1",
	}
	h2 := host.Host{
		Id:          "h2",
		RunningTask: "t2",
	}
	h3 := host.Host{
		Id:          "h3",
		RunningTask: "t3",
	}
	h4 := host.Host{
		Id:          "h4",
		RunningTask: "",
	}
	h5 := host.Host{
		Id:          "h5",
		RunningTask: "",
	}
	t1 := task.Task{
		Id:               "t1",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-11 * time.Minute),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 10 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert())
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Hour,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t3.Insert())

	freeHosts, err := calcExistingFreeHosts([]host.Host{h1, h2, h3, h4, h5}, 1, 30*time.Minute)
	s.NoError(err)
	s.Equal(3, freeHosts)
}

func (s *UtilizationAllocatorSuite) TestCalcHostsForLongTasks() {
	queue := []model.TaskQueueItem{
		{
			ExpectedDuration: 5 * time.Minute,
		},
		{
			ExpectedDuration: 6 * time.Hour,
		},
		{
			ExpectedDuration: 15 * time.Minute,
		},
		{
			ExpectedDuration: 30 * time.Minute,
		},
	}
	newQueue, numRemoved := calcHostsForLongTasks(queue, 30*time.Minute)
	s.Equal(2, numRemoved)
	s.Len(newQueue, 2)
	for _, item := range newQueue {
		s.True(item.ExpectedDuration < 30*time.Minute)
	}
}

func (s *UtilizationAllocatorSuite) TestNoExistingHosts() {
	data := HostAllocatorData{
		distro:           s.distro,
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 20 * time.Minute,
			},
			{
				ExpectedDuration: 3 * time.Minute,
			},
			{
				ExpectedDuration: 45 * time.Second,
			},
			{
				ExpectedDuration: 15 * time.Minute,
			},
			{
				ExpectedDuration: 25 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(2, hosts)
}

func (s *UtilizationAllocatorSuite) TestStaticDistro() {
	data := HostAllocatorData{
		distro: distro.Distro{
			Provider: evergreen.ProviderNameStatic,
		},
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 20 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(0, hosts)
}

func (s *UtilizationAllocatorSuite) TestExistingHostsSufficient() {
	h1 := host.Host{
		Id:          "h1",
		RunningTask: "t1",
	}
	h2 := host.Host{
		Id:          "h2",
		RunningTask: "t2",
	}
	h3 := host.Host{
		Id:          "h3",
		RunningTask: "",
	}
	t1 := task.Task{
		Id:               "t1",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert())
	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2, h3},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 30 * time.Second,
			},
			{
				ExpectedDuration: 3 * time.Minute,
			},
			{
				ExpectedDuration: 5 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(0, hosts)
}

func (s *UtilizationAllocatorSuite) TestLongTasksInQueue1() {
	h1 := host.Host{
		Id:          "h1",
		RunningTask: "t1",
	}
	h2 := host.Host{
		Id:          "h2",
		RunningTask: "t2",
	}
	t1 := task.Task{
		Id:               "t1",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert())
	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(5, hosts)
}

func (s *UtilizationAllocatorSuite) TestLongTasksInQueue2() {
	h1 := host.Host{
		Id:          "h1",
		RunningTask: "t1",
	}
	h2 := host.Host{
		Id:          "h2",
		RunningTask: "t2",
	}
	t1 := task.Task{
		Id:               "t1",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert())
	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 3 * time.Minute,
			},
			{
				ExpectedDuration: 10 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(5, hosts)
}

func (s *UtilizationAllocatorSuite) TestOverMaxHosts() {
	h1 := host.Host{
		Id:          "h1",
		RunningTask: "t1",
	}
	h2 := host.Host{
		Id:          "h2",
		RunningTask: "t2",
	}
	t1 := task.Task{
		Id:               "t1",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert())
	data := HostAllocatorData{
		distro: distro.Distro{
			Provider: evergreen.ProviderNameEc2Auto,
			PoolSize: 10,
		},
		existingHosts:    []host.Host{h1, h2},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(8, hosts)
}

func (s *UtilizationAllocatorSuite) TestExistingLongTask() {
	h1 := host.Host{
		Id:          "h1",
		RunningTask: "t1",
	}
	h2 := host.Host{
		Id:          "h2",
		RunningTask: "t2",
	}
	t1 := task.Task{
		Id:               "t1",
		Project:          s.projectName,
		ExpectedDuration: 4 * time.Hour,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert())
	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 30 * time.Second,
			},
			{
				ExpectedDuration: 5 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(1, hosts)
}

func (s *UtilizationAllocatorSuite) TestOverrunTask() {
	h1 := host.Host{
		Id:          "h1",
		RunningTask: "t1",
	}
	t1 := task.Task{
		Id:               "t1",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-1 * time.Hour),
	}
	s.NoError(t1.Insert())
	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 20 * time.Minute,
			},
			{
				ExpectedDuration: 15 * time.Minute,
			},
			{
				ExpectedDuration: 15 * time.Minute,
			},
			{
				ExpectedDuration: 25 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(2, hosts)
}

func (s *UtilizationAllocatorSuite) TestSoonToBeFree() {
	h1 := host.Host{
		Id:          "h1",
		RunningTask: "t1",
	}
	h2 := host.Host{
		Id:          "h2",
		RunningTask: "t2",
	}
	h3 := host.Host{
		Id:          "h3",
		RunningTask: "t3",
	}
	h4 := host.Host{
		Id:          "h4",
		RunningTask: "t4",
	}
	h5 := host.Host{
		Id:          "h5",
		RunningTask: "t5",
	}
	t1 := task.Task{
		Id:               "t1",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-15 * time.Minute),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 10 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert())
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-30 * time.Minute),
	}
	s.NoError(t3.Insert())
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Hour,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-2 * time.Hour),
	}
	s.NoError(t4.Insert())
	t5 := task.Task{
		Id:               "t5",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Hour,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t5.Insert())
	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2, h3, h4, h5},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(5, hosts)
}

func (s *UtilizationAllocatorSuite) TestExcessHosts() {
	h1 := host.Host{
		Id:          "h1",
		RunningTask: "",
	}
	h2 := host.Host{
		Id:          "h2",
		RunningTask: "",
	}
	h3 := host.Host{
		Id:          "h3",
		RunningTask: "",
	}
	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2, h3},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 29 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(0, hosts)
}

func (s *UtilizationAllocatorSuite) TestRealisticScenario1() {
	h1 := host.Host{
		Id:          "h1",
		RunningTask: "t1",
	}
	h2 := host.Host{
		Id:          "h2",
		RunningTask: "t2",
	}
	h3 := host.Host{
		Id:          "h3",
		RunningTask: "t3",
	}
	h4 := host.Host{
		Id:          "h4",
		RunningTask: "t4",
	}
	h5 := host.Host{
		Id:          "h5",
		RunningTask: "",
	}
	t1 := task.Task{
		Id:               "t1",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 10 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert())
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 10 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t3.Insert())
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Hour,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-30 * time.Minute),
	}
	s.NoError(t4.Insert())
	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2, h3, h4, h5},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: []model.TaskQueueItem{
			// 3 long tasks + 37min of new tasks
			// these should need 4 total hosts, but there is 1 idle host
			// and 2 hosts soon to be idle (1 after scaling by a factor of 0.5)
			// so we only need 2 new hosts
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 5 * time.Minute,
			},
			{
				ExpectedDuration: 45 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Second,
			},
			{
				ExpectedDuration: 10 * time.Minute,
			},
			{
				ExpectedDuration: 1 * time.Hour,
			},
			{
				ExpectedDuration: 1 * time.Minute,
			},
			{
				ExpectedDuration: 20 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(2, hosts)
}

func (s *UtilizationAllocatorSuite) TestRealisticScenario2() {
	h1 := host.Host{
		Id:          "h1",
		RunningTask: "t1",
	}
	h2 := host.Host{
		Id:          "h2",
		RunningTask: "t2",
	}
	h3 := host.Host{
		Id:          "h3",
		RunningTask: "t3",
	}
	h4 := host.Host{
		Id:          "h4",
		RunningTask: "t4",
	}
	h5 := host.Host{
		Id:          "h5",
		RunningTask: "t5",
	}
	t1 := task.Task{
		Id:               "t1",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-40 * time.Minute),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-30 * time.Minute),
	}
	s.NoError(t2.Insert())
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-20 * time.Minute),
	}
	s.NoError(t3.Insert())
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t4.Insert())
	t5 := task.Task{
		Id:               "t5",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t5.Insert())
	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2, h3, h4, h5},
		freeHostFraction: 1,
		taskQueueItems: []model.TaskQueueItem{
			// 1 long task + 68 minutes of tasks should need 3 hosts
			// 3.0 free hosts in the next 30 mins (factor = 1)
			// so we need 0 hosts
			{
				ExpectedDuration: 30 * time.Minute,
			},
			{
				ExpectedDuration: 20 * time.Minute,
			},
			{
				ExpectedDuration: 15 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Second,
			},
			{
				ExpectedDuration: 10 * time.Minute,
			},
			{
				ExpectedDuration: 50 * time.Second,
			},
			{
				ExpectedDuration: 1 * time.Minute,
			},
			{
				ExpectedDuration: 20 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(0, hosts)
}

func (s *UtilizationAllocatorSuite) TestRealisticScenarioWithContainers() {
	parentDistro := distro.Distro{
		Id:       "parent-distro",
		PoolSize: 50,
		Provider: evergreen.ProviderNameEc2Auto,
	}
	s.NoError(parentDistro.Insert())

	h1 := host.Host{
		Id:            "h1",
		HasContainers: true,
	}
	h2 := host.Host{
		Id:            "h2",
		HasContainers: true,
	}
	h3 := host.Host{
		Id:          "h3",
		RunningTask: "t1",
		ParentID:    "h1",
	}
	h4 := host.Host{
		Id:          "h4",
		RunningTask: "t2",
		ParentID:    "h1",
	}
	h5 := host.Host{
		Id:          "h5",
		RunningTask: "t3",
		ParentID:    "h2",
	}
	h6 := host.Host{
		Id:          "h6",
		RunningTask: "t4",
		ParentID:    "h2",
	}
	h7 := host.Host{
		Id:          "h7",
		RunningTask: "t5",
		ParentID:    "h2",
	}
	t1 := task.Task{
		Id:               "t1",
		Project:          s.projectName,
		ExpectedDuration: 3 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-5 * time.Minute),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 10 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-5 * time.Minute),
	}
	s.NoError(t2.Insert())
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 5 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t3.Insert())
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 5 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t4.Insert())
	t5 := task.Task{
		Id:               "t5",
		Project:          s.projectName,
		ExpectedDuration: 15 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t5.Insert())

	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2, h3, h4, h5, h6, h7},
		freeHostFraction: 1,
		taskQueueItems: []model.TaskQueueItem{
			// 2 long tasks + 9 minutes of tasks should need 3 hosts
			// there are 2 idle tasks and 2 free hosts in the next 5 mins (factor = 1)
			// so we need 0 hosts
			{
				ExpectedDuration: 5 * time.Minute,
			},
			{
				ExpectedDuration: 2 * time.Minute,
			},
			{
				ExpectedDuration: 15 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Second,
			},
			{
				ExpectedDuration: 10 * time.Minute,
			},
			{
				ExpectedDuration: 50 * time.Second,
			},
		},
		usesContainers: true,
		containerPool: &evergreen.ContainerPool{
			Id:            "test-pool",
			MaxContainers: 10,
			Distro:        "parent-distro",
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(0, hosts)
}

func (s *UtilizationAllocatorSuite) TestRealisticScenarioWithContainers2() {
	parentDistro := distro.Distro{
		Id:       "parent-distro",
		PoolSize: 50,
		Provider: evergreen.ProviderNameEc2Auto,
	}
	s.NoError(parentDistro.Insert())

	h1 := host.Host{
		Id:            "h1",
		HasContainers: true,
	}
	h2 := host.Host{
		Id:            "h2",
		HasContainers: true,
	}
	h3 := host.Host{
		Id:          "h3",
		RunningTask: "t1",
		ParentID:    "h1",
	}
	h4 := host.Host{
		Id:          "h4",
		RunningTask: "t2",
		ParentID:    "h1",
	}
	h5 := host.Host{
		Id:          "h5",
		RunningTask: "t3",
		ParentID:    "h2",
	}
	h6 := host.Host{
		Id:          "h6",
		RunningTask: "t4",
		ParentID:    "h2",
	}
	h7 := host.Host{
		Id:          "h7",
		RunningTask: "t5",
		ParentID:    "h2",
	}
	t1 := task.Task{
		Id:               "t1",
		Project:          s.projectName,
		ExpectedDuration: 10 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-5 * time.Minute),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 12 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-5 * time.Minute),
	}
	s.NoError(t2.Insert())
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 5 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t3.Insert())
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 5 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t4.Insert())
	t5 := task.Task{
		Id:               "t5",
		Project:          s.projectName,
		ExpectedDuration: 13 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t5.Insert())

	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2, h3, h4, h5, h6, h7},
		freeHostFraction: 1,
		taskQueueItems: []model.TaskQueueItem{
			// 3 long tasks + 10 minutes of tasks should need 5 hosts
			// there is 1 idle task and 3 free hosts in the next 5 mins (factor = 1)
			// so we need 1 host
			{
				ExpectedDuration: 5 * time.Minute,
			},
			{
				ExpectedDuration: 2 * time.Minute,
			},
			{
				ExpectedDuration: 15 * time.Minute,
			},
			{
				ExpectedDuration: 30 * time.Second,
			},
			{
				ExpectedDuration: 10 * time.Minute,
			},
			{
				ExpectedDuration: 50 * time.Second,
			},
			{
				ExpectedDuration: 50 * time.Second,
			},
			{
				ExpectedDuration: 7 * time.Minute,
			},
		},
		usesContainers: true,
		containerPool: &evergreen.ContainerPool{
			Id:            "test-pool",
			MaxContainers: 10,
			Distro:        "parent-distro",
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(3, hosts)
}

func (s *UtilizationAllocatorSuite) TestOnlyTaskGroupsOnlyScheduled() {
	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{},
		freeHostFraction: 1,
		taskQueueItems: []model.TaskQueueItem{
			// a long queue of task group tasks with max hosts=2 should request 2
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "tg1",
				GroupMaxHosts:    2,
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "tg1",
				GroupMaxHosts:    2,
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "tg1",
				GroupMaxHosts:    2,
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "tg1",
				GroupMaxHosts:    2,
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "tg1",
				GroupMaxHosts:    2,
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "tg1",
				GroupMaxHosts:    2,
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "tg1",
				GroupMaxHosts:    2,
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "tg1",
				GroupMaxHosts:    2,
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "tg1",
				GroupMaxHosts:    2,
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "tg1",
				GroupMaxHosts:    2,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(2, hosts)
}

func (s *UtilizationAllocatorSuite) TestOnlyTaskGroupsSomeRunning() {
	h1 := host.Host{
		Id:                      "h1",
		RunningTask:             "t1",
		RunningTaskGroup:        "g1",
		RunningTaskProject:      s.projectName,
		RunningTaskVersion:      "v1",
		RunningTaskBuildVariant: "bv1",
	}
	h2 := host.Host{
		Id:                      "h2",
		RunningTask:             "t2",
		RunningTaskGroup:        "g1",
		RunningTaskProject:      s.projectName,
		RunningTaskVersion:      "v1",
		RunningTaskBuildVariant: "bv1",
	}
	h3 := host.Host{
		Id:                      "h3",
		RunningTask:             "t3",
		RunningTaskGroup:        "g2",
		RunningTaskProject:      s.projectName,
		RunningTaskVersion:      "v1",
		RunningTaskBuildVariant: "bv1",
	}
	t1 := task.Task{
		Id:                "t1",
		Project:           s.projectName,
		ExpectedDuration:  15 * time.Minute,
		BuildVariant:      "bv1",
		TaskGroup:         "g1",
		TaskGroupMaxHosts: 3,
		StartTime:         time.Now(),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:                "t2",
		Project:           s.projectName,
		ExpectedDuration:  15 * time.Minute,
		BuildVariant:      "bv1",
		TaskGroup:         "g1",
		TaskGroupMaxHosts: 3,
		StartTime:         time.Now(),
	}
	s.NoError(t2.Insert())
	t3 := task.Task{
		Id:                "t3",
		Project:           s.projectName,
		ExpectedDuration:  15 * time.Minute,
		BuildVariant:      "bv1",
		TaskGroup:         "g2",
		TaskGroupMaxHosts: 1,
		StartTime:         time.Now(),
	}
	s.NoError(t3.Insert())

	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2, h3},
		freeHostFraction: 1,
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 15 * time.Minute,
				Group:            "g1",
				GroupMaxHosts:    3,
				BuildVariant:     "bv1",
				Project:          s.projectName,
				Version:          "v1",
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "g2",
				GroupMaxHosts:    1,
				BuildVariant:     "bv1",
				Project:          s.projectName,
				Version:          "v1",
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "g2",
				GroupMaxHosts:    1,
				BuildVariant:     "bv1",
				Project:          s.projectName,
				Version:          "v1",
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "g2",
				GroupMaxHosts:    1,
				BuildVariant:     "bv1",
				Project:          s.projectName,
				Version:          "v1",
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "g2",
				GroupMaxHosts:    1,
				BuildVariant:     "bv1",
				Project:          s.projectName,
				Version:          "v1",
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(0, hosts)
}

func (s *UtilizationAllocatorSuite) TestRealisticScenarioWithTaskGroups() {
	h1 := host.Host{
		Id:                      "h1",
		RunningTask:             "t1",
		RunningTaskGroup:        "g1",
		RunningTaskProject:      s.projectName,
		RunningTaskVersion:      "v1",
		RunningTaskBuildVariant: "bv1",
	}
	h2 := host.Host{
		Id:                      "h2",
		RunningTask:             "t2",
		RunningTaskGroup:        "g1",
		RunningTaskProject:      s.projectName,
		RunningTaskVersion:      "v1",
		RunningTaskBuildVariant: "bv1",
	}
	h3 := host.Host{
		Id:                      "h3",
		RunningTask:             "t3",
		RunningTaskGroup:        "g2",
		RunningTaskProject:      s.projectName,
		RunningTaskVersion:      "v1",
		RunningTaskBuildVariant: "bv1",
	}
	h4 := host.Host{
		Id:          "h4",
		RunningTask: "t4",
	}
	h5 := host.Host{
		Id:          "h5",
		RunningTask: "t5",
	}
	h6 := host.Host{
		Id:          "h6",
		RunningTask: "t6",
	}
	h7 := host.Host{
		Id:                      "h7",
		RunningTask:             "t7",
		RunningTaskGroup:        "g3",
		RunningTaskProject:      s.projectName,
		RunningTaskVersion:      "v1",
		RunningTaskBuildVariant: "bv1",
	}
	t1 := task.Task{
		Id:                "t1",
		Project:           s.projectName,
		ExpectedDuration:  15 * time.Minute,
		BuildVariant:      "bv1",
		TaskGroup:         "g1",
		TaskGroupMaxHosts: 3,
		StartTime:         time.Now(),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:                "t2",
		Project:           s.projectName,
		ExpectedDuration:  15 * time.Minute,
		BuildVariant:      "bv1",
		TaskGroup:         "g1",
		TaskGroupMaxHosts: 3,
		StartTime:         time.Now(),
	}
	s.NoError(t2.Insert())
	t3 := task.Task{
		Id:                "t3",
		Project:           s.projectName,
		ExpectedDuration:  15 * time.Minute,
		BuildVariant:      "bv1",
		TaskGroup:         "g2",
		TaskGroupMaxHosts: 1,
		StartTime:         time.Now(),
	}
	s.NoError(t3.Insert())
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 5 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-5 * time.Minute),
	}
	s.NoError(t4.Insert())
	t5 := task.Task{
		Id:               "t5",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t5.Insert())
	t6 := task.Task{
		Id:               "t6",
		Project:          s.projectName,
		ExpectedDuration: 2 * time.Hour,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t6.Insert())
	t7 := task.Task{
		Id:                "t7",
		Project:           s.projectName,
		ExpectedDuration:  15 * time.Minute,
		BuildVariant:      "bv1",
		TaskGroup:         "g3",
		TaskGroupMaxHosts: 1,
		StartTime:         time.Now(),
	}
	s.NoError(t7.Insert())

	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2, h3, h4, h5, h6, h7},
		freeHostFraction: 1,
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "g1",
				GroupMaxHosts:    3,
				BuildVariant:     "bv1",
				Project:          s.projectName,
				Version:          "v1",
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "g1",
				GroupMaxHosts:    3,
				BuildVariant:     "bv1",
				Project:          s.projectName,
				Version:          "v1",
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "g2",
				GroupMaxHosts:    1,
				BuildVariant:     "bv1",
				Project:          s.projectName,
				Version:          "v1",
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "g2",
				GroupMaxHosts:    1,
				BuildVariant:     "bv1",
				Project:          s.projectName,
				Version:          "v1",
			},
			{
				ExpectedDuration: 15 * time.Minute,
			},
			{
				ExpectedDuration: 5 * time.Minute,
			},
			{
				ExpectedDuration: 20 * time.Minute,
			},
			{
				ExpectedDuration: 15 * time.Minute,
			},
			{
				ExpectedDuration: 15 * time.Minute,
			},
			{
				ExpectedDuration: 5 * time.Minute,
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	// robust handling of task groups would request 2 hosts rather than 1 here
	s.Equal(1, hosts)
}

func (s *UtilizationAllocatorSuite) TestTaskGroupsWithExcessFreeHosts() {
	h1 := host.Host{
		Id: "h1",
	}
	h2 := host.Host{
		Id: "h2",
	}
	h3 := host.Host{
		Id: "h3",
	}

	data := HostAllocatorData{
		distro:           s.distro,
		existingHosts:    []host.Host{h1, h2, h3},
		freeHostFraction: 1,
		taskQueueItems: []model.TaskQueueItem{
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "g1",
				GroupMaxHosts:    3,
				BuildVariant:     "bv1",
				Project:          s.projectName,
				Version:          "v1",
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "g1",
				GroupMaxHosts:    3,
				BuildVariant:     "bv1",
				Project:          s.projectName,
				Version:          "v1",
			},
			{
				ExpectedDuration: 30 * time.Minute,
				Group:            "g1",
				GroupMaxHosts:    3,
				BuildVariant:     "bv1",
				Project:          s.projectName,
				Version:          "v1",
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(0, hosts)
}
