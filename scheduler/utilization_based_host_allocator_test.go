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
	"github.com/stretchr/testify/suite"
)

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
	s.NoError(db.ClearCollections(task.Collection, host.Collection))
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

// unit tests for the host allocator
func (s *UtilizationAllocatorSuite) TestErrorIfMultipleDistros() {
	data := HostAllocatorData{
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
			"otherDistro": distro.Distro{
				PoolSize: 50,
			},
		},
	}

	_, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.EqualError(err, "more than 1 distro sent to UtilizationBasedHostAllocator")
}

func (s *UtilizationAllocatorSuite) TestNoExistingHosts() {
	data := HostAllocatorData{
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
		},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
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
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(2, hosts[s.distroName])
}

func (s *UtilizationAllocatorSuite) TestStaticDistro() {
	data := HostAllocatorData{
		distros: map[string]distro.Distro{
			s.distroName: distro.Distro{
				Provider: evergreen.ProviderNameStatic,
			},
		},
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
				{
					ExpectedDuration: 20 * time.Minute,
				},
				{
					ExpectedDuration: 30 * time.Minute,
				},
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(0, hosts[s.distroName])
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
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
		},
		existingDistroHosts: map[string][]host.Host{
			s.distroName: []host.Host{h1, h2, h3},
		},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
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
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(0, hosts[s.distroName])
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
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
		},
		existingDistroHosts: map[string][]host.Host{
			s.distroName: []host.Host{h1, h2},
		},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
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
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(5, hosts[s.distroName])
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
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
		},
		existingDistroHosts: map[string][]host.Host{
			s.distroName: []host.Host{h1, h2},
		},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
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
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(5, hosts[s.distroName])
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
		distros: map[string]distro.Distro{
			s.distroName: distro.Distro{
				Provider: evergreen.ProviderNameEc2Auto,
				PoolSize: 10,
			},
		},
		existingDistroHosts: map[string][]host.Host{
			s.distroName: []host.Host{h1, h2},
		},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
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
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(8, hosts[s.distroName])
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
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
		},
		existingDistroHosts: map[string][]host.Host{
			s.distroName: []host.Host{h1, h2},
		},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
				{
					ExpectedDuration: 30 * time.Second,
				},
				{
					ExpectedDuration: 5 * time.Minute,
				},
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(1, hosts[s.distroName])
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
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
		},
		existingDistroHosts: map[string][]host.Host{
			s.distroName: []host.Host{h1},
		},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
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
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(2, hosts[s.distroName])
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
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
		},
		existingDistroHosts: map[string][]host.Host{
			s.distroName: []host.Host{h1, h2, h3, h4, h5},
		},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
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
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(5, hosts[s.distroName])
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
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
		},
		existingDistroHosts: map[string][]host.Host{
			s.distroName: []host.Host{h1, h2, h3},
		},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
				{
					ExpectedDuration: 29 * time.Minute,
				},
			},
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(0, hosts[s.distroName])
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
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
		},
		existingDistroHosts: map[string][]host.Host{
			s.distroName: []host.Host{h1, h2, h3, h4, h5},
		},
		freeHostFraction: s.freeHostFraction,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
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
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(2, hosts[s.distroName])
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
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
		},
		existingDistroHosts: map[string][]host.Host{
			s.distroName: []host.Host{h1, h2, h3, h4, h5},
		},
		freeHostFraction: 1,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
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
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(0, hosts[s.distroName])
}

func (s *UtilizationAllocatorSuite) TestRealisticScenarioWithContainers() {
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
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
		},
		existingDistroHosts: map[string][]host.Host{
			s.distroName: []host.Host{h1, h2, h3, h4, h5, h6, h7},
		},
		freeHostFraction: 1,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
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
		},
		usesContainers: true,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(0, hosts[s.distroName])
}

func (s *UtilizationAllocatorSuite) TestRealisticScenarioWithContainers2() {
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
		distros: map[string]distro.Distro{
			s.distroName: s.distro,
		},
		existingDistroHosts: map[string][]host.Host{
			s.distroName: []host.Host{h1, h2, h3, h4, h5, h6, h7},
		},
		freeHostFraction: 1,
		taskQueueItems: map[string][]model.TaskQueueItem{
			s.distroName: []model.TaskQueueItem{
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
		},
		usesContainers: true,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, data)
	s.NoError(err)
	s.Equal(1, hosts[s.distroName])
}
