package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestGroupByTaskGroup(t *testing.T) {
	assert := assert.New(t)

	// No task groups except ""

	hosts := []host.Host{
		{
			Id: "host1",
		},
		{
			Id: "host2",
		},
	}

	taskGroupInfo := model.TaskGroupInfo{
		Name:                  "",
		Count:                 2,
		MaxHosts:              1,
		ExpectedDuration:      2 * time.Minute,
		CountOverThreshold:    0,
		DurationOverThreshold: 0,
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:             2,
		ExpectedDuration:   2 * time.Minute,
		CountOverThreshold: 0,
		TaskGroupInfos:     []model.TaskGroupInfo{taskGroupInfo},
	}

	taskGroupDatas := groupByTaskGroup(hosts, distroQueueInfo)
	assert.Len(taskGroupDatas, 1)
	assert.Len(taskGroupDatas[""].Hosts, 2)
	assert.Equal(taskGroupDatas[""].Info.Count, 2)

	// Some running task groups

	hosts = []host.Host{
		{
			Id:               "h1",
			RunningTaskGroup: "g1",
			RunningTask:      "foo",
		},
		{
			Id:               "h2",
			RunningTaskGroup: "g1",
			RunningTask:      "bar",
		},
	}
	taskGroupInfo1 := model.TaskGroupInfo{
		Name:  fmt.Sprintf("%s_%s_%s_%s", "g2", "", "", ""),
		Count: 1,
	}

	taskGroupInfo2 := model.TaskGroupInfo{
		Name:  "",
		Count: 1,
	}

	distroQueueInfo = model.DistroQueueInfo{
		Length: 2,
		TaskGroupInfos: []model.TaskGroupInfo{
			taskGroupInfo1,
			taskGroupInfo2,
		},
	}

	taskGroupDatas = groupByTaskGroup(hosts, distroQueueInfo)
	assert.Len(taskGroupDatas, 3)
	assert.Len(taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g1", "", "", "")].Hosts, 2)
	assert.Equal(taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g1", "", "", "")].Info.Count, 0)
	assert.Len(taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g2", "", "", "")].Hosts, 0)
	assert.Equal(taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g2", "", "", "")].Info.Count, 1)
	assert.Equal("h1", taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g1", "", "", "")].Hosts[0].Id)
	assert.Equal("h2", taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g1", "", "", "")].Hosts[1].Id)
	assert.Len(taskGroupDatas[""].Hosts, 0)
	assert.Equal(taskGroupDatas[""].Info.Count, 1)

	// Some finished task groups

	hosts = []host.Host{
		{
			Id:               "h1",
			RunningTaskGroup: "g1",
		},
		{
			Id:               "h2",
			RunningTaskGroup: "g1",
		},
	}

	distroQueueInfo = model.DistroQueueInfo{
		Length: 2,
		TaskGroupInfos: []model.TaskGroupInfo{
			taskGroupInfo1,
			taskGroupInfo2,
		},
	}

	taskGroupDatas = groupByTaskGroup(hosts, distroQueueInfo)
	assert.Len(taskGroupDatas, 2)
	assert.Len(taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g2", "", "", "")].Hosts, 2)
	assert.Equal(taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g2", "", "", "")].Info.Count, 1)
	assert.Len(taskGroupDatas[""].Hosts, 2)
	assert.Equal(taskGroupDatas[""].Info.Count, 1)
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
		Provider: evergreen.ProviderNameEc2Auto,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MinimumHosts: 0,
			MaximumHosts: 50,
		},
	}
	s.projectName = "testProject"
	s.freeHostFraction = 0.5
}

func (s *UtilizationAllocatorSuite) SetupTest() {
	s.ctx = context.Background()
	s.NoError(db.ClearCollections(task.Collection, host.Collection, distro.Collection))
	s.distro.HostAllocatorSettings.MinimumHosts = 0
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

	freeHosts, err := calcExistingFreeHosts([]host.Host{h1, h2, h3, h4, h5}, 1, evergreen.MaxDurationPerDistroHost)
	s.NoError(err)
	s.Equal(3, freeHosts)
}

func (s *UtilizationAllocatorSuite) TestNoExistingHosts() {
	taskGroupInfo := model.TaskGroupInfo{
		Name:             "",
		Count:            5,
		ExpectedDuration: (20 * time.Minute) + (3 * time.Minute) + (45 * time.Second) + (15 * time.Minute) + (25 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:               5,
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		ExpectedDuration:     (20 * time.Minute) + (3 * time.Minute) + (45 * time.Second) + (15 * time.Minute) + (25 * time.Minute),
		TaskGroupInfos: []model.TaskGroupInfo{
			taskGroupInfo,
		},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{},
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
	s.NoError(err)
	s.Equal(2, hosts)
}

func (s *UtilizationAllocatorSuite) TestStaticDistro() {
	distro := distro.Distro{
		Provider: evergreen.ProviderNameStatic,
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:               2,
		ExpectedDuration:     (20 * time.Minute) + (30 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   1,
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          distro,
		ExistingHosts:   []host.Host{},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
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

	distroQueueInfo := model.DistroQueueInfo{
		Length:               3,
		ExpectedDuration:     (30 * time.Second) + (3 * time.Minute) + (5 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   0,
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2, h3},
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
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

	taskGroupInfo := model.TaskGroupInfo{
		Name:                  "",
		Count:                 5,
		ExpectedDuration:      5 * (30 * time.Minute),
		CountOverThreshold:    5,
		DurationOverThreshold: 5 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:               5,
		ExpectedDuration:     5 * (30 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   5,
		TaskGroupInfos:       []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2},
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
	s.NoError(err)
	s.Equal(5, hosts)
}

func (s *UtilizationAllocatorSuite) TestMinimumHostsThreshold() {
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

	taskGroupInfo := model.TaskGroupInfo{
		Name:                  "",
		Count:                 5,
		ExpectedDuration:      5 * (30 * time.Minute),
		CountOverThreshold:    5,
		DurationOverThreshold: 5 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:               5,
		ExpectedDuration:     5 * (30 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   5,
		TaskGroupInfos:       []model.TaskGroupInfo{taskGroupInfo},
	}

	minimumHostsThreshold := 10
	s.distro.HostAllocatorSettings.MinimumHosts = minimumHostsThreshold
	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2},
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
	s.NoError(err)
	s.Equal(8, hosts)
	s.Equal(minimumHostsThreshold, len(hostAllocatorData.ExistingHosts)+hosts)
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

	taskGroupInfo := model.TaskGroupInfo{
		Name:                  "",
		Count:                 7,
		ExpectedDuration:      (5 * (30 * time.Minute)) + (3 * time.Minute) + (10 * time.Minute),
		CountOverThreshold:    5,
		DurationOverThreshold: 5 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		ExpectedDuration:     (5 * (30 * time.Minute)) + (3 * time.Minute) + (10 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   5,
		TaskGroupInfos: []model.TaskGroupInfo{
			taskGroupInfo,
		},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2},
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
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

	distro := distro.Distro{
		Provider: evergreen.ProviderNameEc2Auto,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 10,
		},
	}

	taskGroupInfo := model.TaskGroupInfo{
		Name:                  "",
		Count:                 9,
		ExpectedDuration:      9 * (30 * time.Minute),
		CountOverThreshold:    9,
		DurationOverThreshold: 9 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:               9,
		ExpectedDuration:     9 * (30 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   9,
		TaskGroupInfos:       []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           distro,
		ExistingHosts:    []host.Host{h1, h2},
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
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

	taskGroupInfo := model.TaskGroupInfo{
		Name:             "",
		Count:            2,
		ExpectedDuration: (30 * time.Second) + (5 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:               2,
		ExpectedDuration:     (30 * time.Second) + (5 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   0,
		TaskGroupInfos:       []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2},
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
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

	taskGroupInfo := model.TaskGroupInfo{
		Name:             "",
		Count:            4,
		ExpectedDuration: (20 * time.Minute) + (15 * time.Minute) + (15 * time.Minute) + (25 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:               4,
		ExpectedDuration:     (20 * time.Minute) + (15 * time.Minute) + (15 * time.Minute) + (25 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		TaskGroupInfos:       []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1},
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
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

	taskGroupInfo := model.TaskGroupInfo{
		Name:                  "",
		Count:                 6,
		ExpectedDuration:      6 * (30 * time.Minute),
		CountOverThreshold:    6,
		DurationOverThreshold: 6 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:               6,
		ExpectedDuration:     6 * (30 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   6,
		TaskGroupInfos:       []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2, h3, h4, h5},
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
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

	distroQueueInfo := model.DistroQueueInfo{
		Length:               1,
		ExpectedDuration:     29 * time.Minute,
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   0,
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2, h3},
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
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

	taskGroupInfo := model.TaskGroupInfo{
		Name:                  "",
		Count:                 8,
		ExpectedDuration:      (30 * time.Minute) + (5 * time.Minute) + (45 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (1 * time.Hour) + (1 * time.Minute) + (20 * time.Minute),
		CountOverThreshold:    3,
		DurationOverThreshold: (30 * time.Minute) + (45 * time.Minute) + (1 * time.Hour),
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length: 8,
		// 3 long tasks + 37min of new tasks
		// these should need 4 total hosts, but there is 1 idle host
		// and 2 hosts soon to be idle (1 after scaling by a factor of 0.5)
		// so we only need 2 new hosts
		ExpectedDuration:     (30 * time.Minute) + (5 * time.Minute) + (45 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (1 * time.Hour) + (1 * time.Minute) + (20 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   3,
		TaskGroupInfos:       []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2, h3, h4, h5},
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
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

	distroQueueInfo := model.DistroQueueInfo{
		Length: 8,
		// 1 long task + 68 minutes of tasks should need 3 hosts
		// 3.0 free hosts in the next 30 mins (factor = 1)
		// so we need 0 hosts
		ExpectedDuration:     (30 * time.Minute) + (20 * time.Minute) + (15 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (50 * time.Second) + (1 * time.Minute) + (20 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   1,
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2, h3, h4, h5},
		FreeHostFraction: 1,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
	s.NoError(err)
	s.Equal(0, hosts)
}

func (s *UtilizationAllocatorSuite) TestRealisticScenarioWithContainers() {
	parentDistro := distro.Distro{
		Id:       "parent-distro",
		Provider: evergreen.ProviderNameEc2Auto,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 50,
		},
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

	distroQueueInfo := model.DistroQueueInfo{
		Length: 6,
		// 2 long tasks + 9 minutes of tasks should need 3 hosts
		// there are 2 idle tasks and 2 free hosts in the next 5 mins (factor = 1)
		// so we need 0 hosts
		ExpectedDuration:     (5 * time.Minute) + (2 * time.Minute) + (15 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (50 * time.Second),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHostWithContainers,
		CountOverThreshold:   4,
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2, h3, h4, h5, h6, h7},
		FreeHostFraction: 1,
		UsesContainers:   true,
		DistroQueueInfo:  distroQueueInfo,
		ContainerPool: &evergreen.ContainerPool{
			Id:            "test-pool",
			MaxContainers: 10,
			Distro:        "parent-distro",
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
	s.NoError(err)
	s.Equal(0, hosts)
}

func (s *UtilizationAllocatorSuite) TestRealisticScenarioWithContainers2() {
	parentDistro := distro.Distro{
		Id:       "parent-distro",
		Provider: evergreen.ProviderNameEc2Auto,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 50,
		},
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

	taskGroupInfo := model.TaskGroupInfo{
		Name:                  "",
		Count:                 8,
		ExpectedDuration:      (5 * time.Minute) + (2 * time.Minute) + (15 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (50 * time.Second) + (50 * time.Second) + (7 * time.Minute),
		CountOverThreshold:    5,
		DurationOverThreshold: (5 * time.Minute) + (2 * time.Minute) + (15 * time.Minute) + (10 * time.Minute) + (7 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length: 8,
		// 3 long tasks + 10 minutes of tasks should need 5 hosts
		// there is 1 idle task and 3 free hosts in the next 5 mins (factor = 1)
		// so we need 1 host
		ExpectedDuration:     (5 * time.Minute) + (2 * time.Minute) + (15 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (50 * time.Second) + (50 * time.Second) + (7 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHostWithContainers,
		CountOverThreshold:   5,
		TaskGroupInfos: []model.TaskGroupInfo{
			taskGroupInfo,
		},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2, h3, h4, h5, h6, h7},
		FreeHostFraction: 1,
		UsesContainers:   true,
		DistroQueueInfo:  distroQueueInfo,
		ContainerPool: &evergreen.ContainerPool{
			Id:            "test-pool",
			MaxContainers: 10,
			Distro:        "parent-distro",
		},
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
	s.NoError(err)
	s.Equal(3, hosts)
}

func (s *UtilizationAllocatorSuite) TestOnlyTaskGroupsOnlyScheduled() {
	name := fmt.Sprintf("%s_%s_%s_%s", "tg1", "", "", "")

	taskGroupInfo := model.TaskGroupInfo{
		Name:                  name,
		Count:                 10,
		MaxHosts:              2,
		ExpectedDuration:      10 * (30 * time.Minute),
		CountOverThreshold:    10,
		DurationOverThreshold: 10 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length: 10,
		// a long queue of task group tasks with max hosts=2 should request 2
		ExpectedDuration:     10 * (30 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   10,
		TaskGroupInfos: []model.TaskGroupInfo{
			taskGroupInfo,
		},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{},
		FreeHostFraction: 1,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
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

	group1 := fmt.Sprintf("%s_%s_%s_%s", "g1", "bv1", s.projectName, "v1")
	taskGroupInfo1 := model.TaskGroupInfo{
		Name:                  group1,
		Count:                 1,
		MaxHosts:              3,
		ExpectedDuration:      15 * time.Minute,
		CountOverThreshold:    0,
		DurationOverThreshold: 0,
	}

	group2 := fmt.Sprintf("%s_%s_%s_%s", "g2", "bv1", s.projectName, "v1")
	taskGroupInfo2 := model.TaskGroupInfo{
		Name:                  group2,
		Count:                 4,
		MaxHosts:              1,
		ExpectedDuration:      4 * (30 * time.Minute),
		CountOverThreshold:    4,
		DurationOverThreshold: 4 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:               taskGroupInfo1.Count + taskGroupInfo2.Count,
		ExpectedDuration:     taskGroupInfo1.ExpectedDuration + taskGroupInfo2.ExpectedDuration,
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   taskGroupInfo1.CountOverThreshold + taskGroupInfo2.CountOverThreshold,
		TaskGroupInfos:       []model.TaskGroupInfo{taskGroupInfo1, taskGroupInfo2},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2, h3},
		FreeHostFraction: 1,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
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

	group1 := fmt.Sprintf("%s_%s_%s_%s", "g1", "bv1", s.projectName, "v1")
	taskGroupInfo1 := model.TaskGroupInfo{
		Name:                  group1,
		Count:                 2,
		MaxHosts:              3,
		ExpectedDuration:      2 * (30 * time.Minute),
		CountOverThreshold:    2,
		DurationOverThreshold: 2 * (30 * time.Minute),
	}

	group2 := fmt.Sprintf("%s_%s_%s_%s", "g2", "bv1", s.projectName, "v1")
	taskGroupInfo2 := model.TaskGroupInfo{
		Name:                  group2,
		Count:                 2,
		MaxHosts:              1,
		ExpectedDuration:      2 * (30 * time.Minute),
		CountOverThreshold:    2,
		DurationOverThreshold: 2 * (30 * time.Minute),
	}

	group3 := ""
	taskGroupInfo3 := model.TaskGroupInfo{
		Name:                  group3,
		Count:                 6,
		MaxHosts:              0,
		ExpectedDuration:      (15 * time.Minute) + (5 * time.Minute) + (20 * time.Minute) + (15 * time.Minute) + (15 * time.Minute) + (5 * time.Minute),
		CountOverThreshold:    0,
		DurationOverThreshold: 0,
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:               taskGroupInfo1.Count + taskGroupInfo2.Count + taskGroupInfo3.Count,
		ExpectedDuration:     taskGroupInfo1.ExpectedDuration + taskGroupInfo2.ExpectedDuration + taskGroupInfo3.ExpectedDuration,
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   taskGroupInfo1.CountOverThreshold + taskGroupInfo2.CountOverThreshold + taskGroupInfo3.CountOverThreshold,
		TaskGroupInfos:       []model.TaskGroupInfo{taskGroupInfo1, taskGroupInfo2, taskGroupInfo3},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2, h3, h4, h5, h6, h7},
		FreeHostFraction: 1,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
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

	name := fmt.Sprintf("%s_%s_%s_%s", "g1", "bv1", s.projectName, "v1")
	taskGroupInfo := model.TaskGroupInfo{
		Name:                  name,
		Count:                 3,
		MaxHosts:              3,
		ExpectedDuration:      3 * (30 * time.Minute),
		CountOverThreshold:    3,
		DurationOverThreshold: 3,
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:               taskGroupInfo.Count,
		ExpectedDuration:     taskGroupInfo.ExpectedDuration,
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   taskGroupInfo.CountOverThreshold,
		TaskGroupInfos:       []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2, h3},
		FreeHostFraction: 1,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
	s.NoError(err)
	s.Equal(0, hosts)
}

func (s *UtilizationAllocatorSuite) TestHostsWithLongTasks() {
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
	t1 := task.Task{
		Id:           "t1",
		Project:      s.projectName,
		BuildVariant: "bv1",
		StartTime:    time.Now().Add(-60 * time.Minute),
		DurationPrediction: util.CachedDurationValue{
			Value:       10 * time.Minute,
			StdDev:      1 * time.Minute,
			TTL:         time.Hour,
			CollectedAt: time.Now(),
		},
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:           "t2",
		Project:      s.projectName,
		BuildVariant: "bv1",
		StartTime:    time.Now().Add(-15 * time.Minute),
		DurationPrediction: util.CachedDurationValue{
			Value:       15 * time.Minute,
			StdDev:      1 * time.Minute,
			TTL:         time.Hour,
			CollectedAt: time.Now(),
		},
	}
	s.NoError(t2.Insert())
	t3 := task.Task{
		Id:           "t3",
		Project:      s.projectName,
		BuildVariant: "bv1",
		StartTime:    time.Now().Add(-15 * time.Minute),
		DurationPrediction: util.CachedDurationValue{
			Value:       15 * time.Minute,
			StdDev:      1 * time.Minute,
			TTL:         time.Hour,
			CollectedAt: time.Now(),
		},
	}
	s.NoError(t3.Insert())
	t4 := task.Task{
		Id:           "t4",
		Project:      s.projectName,
		BuildVariant: "bv1",
		StartTime:    time.Now().Add(-60 * time.Minute),
		DurationPrediction: util.CachedDurationValue{
			Value:       10 * time.Minute,
			StdDev:      1 * time.Minute,
			TTL:         time.Hour,
			CollectedAt: time.Now(),
		},
	}
	s.NoError(t4.Insert())

	taskGroupInfo := model.TaskGroupInfo{
		Name:                  "",
		Count:                 5,
		ExpectedDuration:      5 * (30 * time.Minute),
		CountOverThreshold:    2,
		DurationOverThreshold: 60 * time.Minute,
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length: 5,
		// 2 running tasks that will take forever + 2 tasks that each should finish now means
		// we should have 1 expected free host after scaling by the 0.5 fraction
		// if we have 5 more tasks coming in, we should request 4 hosts
		ExpectedDuration:     5 * (30 * time.Minute),
		MaxDurationThreshold: evergreen.MaxDurationPerDistroHost,
		CountOverThreshold:   2,
		TaskGroupInfos:       []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    []host.Host{h1, h2, h3, h4},
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}

	hosts, err := UtilizationBasedHostAllocator(s.ctx, hostAllocatorData)
	s.NoError(err)
	s.Equal(4, hosts)
}
