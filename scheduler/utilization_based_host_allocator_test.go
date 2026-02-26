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
		Name:                       "",
		Count:                      2,
		MaxHosts:                   1,
		ExpectedDuration:           2 * time.Minute,
		CountDurationOverThreshold: 0,
		DurationOverThreshold:      0,
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet:  2,
		ExpectedDuration:           2 * time.Minute,
		CountDurationOverThreshold: 0,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo},
	}

	taskGroupDatas := groupByTaskGroup(hosts, distroQueueInfo)
	assert.Len(taskGroupDatas, 1)
	assert.Len(taskGroupDatas[""].Hosts, 2)
	assert.Equal(2, taskGroupDatas[""].Info.Count)

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
		LengthWithDependenciesMet: 2,
		TaskGroupInfos: []model.TaskGroupInfo{
			taskGroupInfo1,
			taskGroupInfo2,
		},
	}

	taskGroupDatas = groupByTaskGroup(hosts, distroQueueInfo)
	assert.Len(taskGroupDatas, 3)
	assert.Len(taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g1", "", "", "")].Hosts, 2)
	assert.Equal(0, taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g1", "", "", "")].Info.Count)
	assert.Empty(taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g2", "", "", "")].Hosts)
	assert.Equal(1, taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g2", "", "", "")].Info.Count)
	assert.Equal("h1", taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g1", "", "", "")].Hosts[0].Id)
	assert.Equal("h2", taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g1", "", "", "")].Hosts[1].Id)
	assert.Empty(taskGroupDatas[""].Hosts)
	assert.Equal(1, taskGroupDatas[""].Info.Count)

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
		LengthWithDependenciesMet: 2,
		TaskGroupInfos: []model.TaskGroupInfo{
			taskGroupInfo1,
			taskGroupInfo2,
		},
	}

	taskGroupDatas = groupByTaskGroup(hosts, distroQueueInfo)
	assert.Len(taskGroupDatas, 2)
	assert.Equal(1, taskGroupDatas[fmt.Sprintf("%s_%s_%s_%s", "g2", "", "", "")].Info.Count)
	assert.Len(taskGroupDatas[""].Hosts, 2)
	assert.Equal(1, taskGroupDatas[""].Info.Count)
}

type UtilizationAllocatorSuite struct {
	ctx         context.Context
	distroName  string
	distro      distro.Distro
	projectName string
	suite.Suite
}

func TestUtilizationAllocatorSuite(t *testing.T) {
	s := &UtilizationAllocatorSuite{}
	suite.Run(t, s)
}

func (s *UtilizationAllocatorSuite) SetupSuite() {
	s.distroName = "testDistro"
	s.projectName = "testProject"
}

func (s *UtilizationAllocatorSuite) SetupTest() {
	s.ctx = context.Background()
	s.NoError(db.ClearCollections(task.Collection, host.Collection, distro.Collection))
	s.distro = distro.Distro{
		Id:       s.distroName,
		Provider: evergreen.ProviderNameEc2Fleet,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MinimumHosts:       0,
			MaximumHosts:       50,
			RoundingRule:       evergreen.HostAllocatorRoundDown,
			FeedbackRule:       evergreen.HostAllocatorNoFeedback,
			FutureHostFraction: .5,
		},
	}
}

func (s *UtilizationAllocatorSuite) TestCalcNewHostsNeeded() {
	s.Equal(0, calcNewHostsNeeded(0*time.Second, 30*time.Minute, 0, 0, 0, 0, true))
	s.Equal(0, calcNewHostsNeeded(0*time.Second, 30*time.Minute, 1, 0, 0, 0, true))
	s.Equal(1, calcNewHostsNeeded(1*time.Second, 30*time.Minute, 0, 0, 0, 0, true))
	s.Equal(0, calcNewHostsNeeded(0*time.Second, 30*time.Minute, 1, 0, 0, 0, true))
	s.Equal(3, calcNewHostsNeeded(3*time.Minute, 1*time.Minute, 0, 0, 0, 0, true))
	s.Equal(11, calcNewHostsNeeded(6*time.Hour, 30*time.Minute, 1, 0, 0, 0, true))
	s.Equal(10, calcNewHostsNeeded(80*time.Hour, 30*time.Minute, 150, 0, 0, 0, true))
	s.Equal(11, calcNewHostsNeeded(80*time.Hour, 30*time.Minute, 150, 0, 1, 0, true))
	s.Equal(12, calcNewHostsNeeded(80*time.Hour, 30*time.Minute, 150, 0, 1, 1, true))
}

func (s *UtilizationAllocatorSuite) TestCalcExistingFreeHosts() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 10 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert(s.T().Context()))
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Hour,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t3.Insert(s.T().Context()))

	freeHosts, err := calcExistingFreeHosts(ctx, []host.Host{h1, h2, h3, h4, h5}, 1, evergreen.MaxDurationPerDistroHost)
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
		LengthWithDependenciesMet: 5,
		MaxDurationThreshold:      evergreen.MaxDurationPerDistroHost,
		ExpectedDuration:          (20 * time.Minute) + (3 * time.Minute) + (45 * time.Second) + (15 * time.Minute) + (25 * time.Minute),
		TaskGroupInfos: []model.TaskGroupInfo{
			taskGroupInfo,
		},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(2, hosts)
	s.Equal(0, free)
}

func (s *UtilizationAllocatorSuite) TestStaticDistro() {
	distro := distro.Distro{
		Provider: evergreen.ProviderNameStatic,
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet:  2,
		ExpectedDuration:           (20 * time.Minute) + (30 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 1,
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          distro,
		ExistingHosts:   []host.Host{},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(0, hosts)
	s.Equal(0, free)
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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert(s.T().Context()))

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet:  3,
		ExpectedDuration:           (30 * time.Second) + (3 * time.Minute) + (5 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 0,
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(0, hosts)
	s.Equal(1, free)
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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert(s.T().Context()))

	taskGroupInfo := model.TaskGroupInfo{
		Name:                       "",
		Count:                      5,
		ExpectedDuration:           5 * (30 * time.Minute),
		CountDurationOverThreshold: 5,
		DurationOverThreshold:      5 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet:  5,
		ExpectedDuration:           5 * (30 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 5,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(5, hosts)
	s.Equal(0, free)
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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert(s.T().Context()))

	taskGroupInfo := model.TaskGroupInfo{
		Name:                       "",
		Count:                      5,
		ExpectedDuration:           5 * (30 * time.Minute),
		CountDurationOverThreshold: 5,
		DurationOverThreshold:      5 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet:  5,
		ExpectedDuration:           5 * (30 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 5,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo},
	}

	minimumHostsThreshold := 10
	s.distro.HostAllocatorSettings.MinimumHosts = minimumHostsThreshold
	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(8, hosts)
	s.Equal(0, free)
	s.Equal(minimumHostsThreshold, len(hostAllocatorData.ExistingHosts)+hosts)
}

func (s *UtilizationAllocatorSuite) TestMinimumHostsThresholdForDisabled() {
	h1 := host.Host{
		Id:          "h1",
		RunningTask: "t1",
	}
	h2 := host.Host{
		Id:          "h2",
		RunningTask: "t2",
	}

	minimumHostsThreshold := 10
	s.distro.HostAllocatorSettings.MinimumHosts = minimumHostsThreshold
	s.distro.Disabled = true
	hostAllocatorData := HostAllocatorData{
		Distro:        s.distro,
		ExistingHosts: []host.Host{h1, h2},
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(8, hosts)
	s.Equal(0, free)
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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert(s.T().Context()))

	taskGroupInfo := model.TaskGroupInfo{
		Name:                       "",
		Count:                      7,
		ExpectedDuration:           (5 * (30 * time.Minute)) + (3 * time.Minute) + (10 * time.Minute),
		CountDurationOverThreshold: 5,
		DurationOverThreshold:      5 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		ExpectedDuration:           (5 * (30 * time.Minute)) + (3 * time.Minute) + (10 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 5,
		TaskGroupInfos: []model.TaskGroupInfo{
			taskGroupInfo,
		},
		LengthWithDependenciesMet: 7,
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(5, hosts)
	s.Equal(0, free)
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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert(s.T().Context()))

	distro := distro.Distro{
		Provider: evergreen.ProviderNameEc2Fleet,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 10,
		},
	}

	taskGroupInfo := model.TaskGroupInfo{
		Name:                       "",
		Count:                      9,
		ExpectedDuration:           9 * (30 * time.Minute),
		CountDurationOverThreshold: 9,
		DurationOverThreshold:      9 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet:  9,
		ExpectedDuration:           9 * (30 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 9,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          distro,
		ExistingHosts:   []host.Host{h1, h2},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(8, hosts)
	s.Equal(0, free)
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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert(s.T().Context()))

	taskGroupInfo := model.TaskGroupInfo{
		Name:             "",
		Count:            2,
		ExpectedDuration: (30 * time.Second) + (5 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet:  2,
		ExpectedDuration:           (30 * time.Second) + (5 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 0,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(1, hosts)
	s.Equal(0, free)
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
	s.NoError(t1.Insert(s.T().Context()))

	taskGroupInfo := model.TaskGroupInfo{
		Name:             "",
		Count:            4,
		ExpectedDuration: (20 * time.Minute) + (15 * time.Minute) + (15 * time.Minute) + (25 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet: 4,
		ExpectedDuration:          (20 * time.Minute) + (15 * time.Minute) + (15 * time.Minute) + (25 * time.Minute),
		MaxDurationThreshold:      evergreen.MaxDurationPerDistroHost,
		TaskGroupInfos:            []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(2, hosts)
	s.Equal(0, free)
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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 10 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert(s.T().Context()))
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-30 * time.Minute),
	}
	s.NoError(t3.Insert(s.T().Context()))
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Hour,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-2 * time.Hour),
	}
	s.NoError(t4.Insert(s.T().Context()))
	t5 := task.Task{
		Id:               "t5",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Hour,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t5.Insert(s.T().Context()))

	taskGroupInfo := model.TaskGroupInfo{
		Name:                       "",
		Count:                      6,
		ExpectedDuration:           6 * (30 * time.Minute),
		CountDurationOverThreshold: 6,
		DurationOverThreshold:      6 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet:  6,
		ExpectedDuration:           6 * (30 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 6,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3, h4, h5},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(5, hosts)
	s.Equal(1, free)
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
		LengthWithDependenciesMet:  1,
		ExpectedDuration:           29 * time.Minute,
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 0,
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(0, hosts)
	s.Equal(3, free)
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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 10 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t2.Insert(s.T().Context()))
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 10 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t3.Insert(s.T().Context()))
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 1 * time.Hour,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-30 * time.Minute),
	}
	s.NoError(t4.Insert(s.T().Context()))

	taskGroupInfo := model.TaskGroupInfo{
		Name:                       "",
		Count:                      8,
		ExpectedDuration:           (30 * time.Minute) + (5 * time.Minute) + (45 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (1 * time.Hour) + (1 * time.Minute) + (20 * time.Minute),
		CountDurationOverThreshold: 3,
		DurationOverThreshold:      (30 * time.Minute) + (45 * time.Minute) + (1 * time.Hour),
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet: 8,
		// 3 long tasks + 37min of new tasks
		// these should need 4 total hosts, but there is 1 idle host
		// and 2 hosts soon to be idle (1 after scaling by a factor of 0.5)
		// so we only need 2 new hosts
		ExpectedDuration:           (30 * time.Minute) + (5 * time.Minute) + (45 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (1 * time.Hour) + (1 * time.Minute) + (20 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 3,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3, h4, h5},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(2, hosts)
	s.Equal(2, free)
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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-30 * time.Minute),
	}
	s.NoError(t2.Insert(s.T().Context()))
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-20 * time.Minute),
	}
	s.NoError(t3.Insert(s.T().Context()))
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t4.Insert(s.T().Context()))
	t5 := task.Task{
		Id:               "t5",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t5.Insert(s.T().Context()))

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet: 8,
		// 1 long task + 68 minutes of tasks should need 3 hosts
		// 3.0 free hosts in the next 30 mins (factor = 1)
		// so we need 0 hosts
		ExpectedDuration:           (30 * time.Minute) + (20 * time.Minute) + (15 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (50 * time.Second) + (1 * time.Minute) + (20 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 1,
	}
	defaultPct := s.distro.HostAllocatorSettings.FutureHostFraction
	s.distro.HostAllocatorSettings.FutureHostFraction = 1

	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3, h4, h5},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.distro.HostAllocatorSettings.FutureHostFraction = defaultPct
	s.NoError(err)
	s.Equal(0, hosts)
	s.Equal(3, free)

}

func (s *UtilizationAllocatorSuite) TestRoundingUp() {
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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-30 * time.Minute),
	}
	s.NoError(t2.Insert(s.T().Context()))
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-20 * time.Minute),
	}
	s.NoError(t3.Insert(s.T().Context()))
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t4.Insert(s.T().Context()))
	t5 := task.Task{
		Id:               "t5",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t5.Insert(s.T().Context()))
	taskGroupInfo := model.TaskGroupInfo{
		Name:                       "",
		Count:                      8,
		ExpectedDuration:           (30 * time.Minute) + (5 * time.Minute) + (45 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (1 * time.Hour) + (1 * time.Minute) + (20 * time.Minute),
		CountDurationOverThreshold: 3,
		DurationOverThreshold:      (30 * time.Minute) + (45 * time.Minute) + (1 * time.Hour),
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet: 8,
		// 3 long tasks + 37min of new tasks
		// these should need 4 total hosts, but there is 1 idle host
		// and 2 hosts soon to be idle (1 after scaling by a factor of 0.5)
		// so we only need 4 new hosts
		ExpectedDuration:           (30 * time.Minute) + (5 * time.Minute) + (45 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (1 * time.Hour) + (1 * time.Minute) + (20 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 3,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo},
	}

	defaultRound := s.distro.HostAllocatorSettings.RoundingRule
	s.distro.HostAllocatorSettings.RoundingRule = evergreen.HostAllocatorRoundUp
	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3, h4, h5},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.distro.HostAllocatorSettings.RoundingRule = defaultRound
	s.Equal(4, hosts)
	s.Equal(1, free)
}

func (s *UtilizationAllocatorSuite) TestRealisticScenarioWithContainers() {
	parentDistro := distro.Distro{
		Id:       "parent-distro",
		Provider: evergreen.ProviderNameEc2Fleet,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 50,
		},
	}
	s.NoError(parentDistro.Insert(s.ctx))

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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 10 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-5 * time.Minute),
	}
	s.NoError(t2.Insert(s.T().Context()))
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 5 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t3.Insert(s.T().Context()))
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 5 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t4.Insert(s.T().Context()))
	t5 := task.Task{
		Id:               "t5",
		Project:          s.projectName,
		ExpectedDuration: 15 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t5.Insert(s.T().Context()))

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet: 6,
		// 2 long tasks + 9 minutes of tasks should need 3 hosts
		// there are 2 idle tasks and 2 free hosts in the next 5 mins (factor = 1)
		// so we need 0 hosts
		ExpectedDuration:           (5 * time.Minute) + (2 * time.Minute) + (15 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (50 * time.Second),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHostWithContainers,
		CountDurationOverThreshold: 4,
	}

	defaultPct := s.distro.HostAllocatorSettings.FutureHostFraction
	s.distro.HostAllocatorSettings.FutureHostFraction = 1
	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3, h4, h5, h6, h7},
		UsesContainers:  true,
		DistroQueueInfo: distroQueueInfo,
		ContainerPool: &evergreen.ContainerPool{
			Id:            "test-pool",
			MaxContainers: 10,
			Distro:        "parent-distro",
		},
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.distro.HostAllocatorSettings.FutureHostFraction = defaultPct
	s.NoError(err)
	s.Equal(0, hosts)
	s.Equal(4, free)
}

func (s *UtilizationAllocatorSuite) TestRealisticScenarioWithContainers2() {
	parentDistro := distro.Distro{
		Id:       "parent-distro",
		Provider: evergreen.ProviderNameEc2Fleet,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 50,
		},
	}
	s.NoError(parentDistro.Insert(s.ctx))

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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:               "t2",
		Project:          s.projectName,
		ExpectedDuration: 12 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-5 * time.Minute),
	}
	s.NoError(t2.Insert(s.T().Context()))
	t3 := task.Task{
		Id:               "t3",
		Project:          s.projectName,
		ExpectedDuration: 5 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t3.Insert(s.T().Context()))
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 5 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t4.Insert(s.T().Context()))
	t5 := task.Task{
		Id:               "t5",
		Project:          s.projectName,
		ExpectedDuration: 13 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now(),
	}
	s.NoError(t5.Insert(s.T().Context()))

	taskGroupInfo := model.TaskGroupInfo{
		Name:                       "",
		Count:                      8,
		ExpectedDuration:           (5 * time.Minute) + (2 * time.Minute) + (15 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (50 * time.Second) + (50 * time.Second) + (7 * time.Minute),
		CountDurationOverThreshold: 5,
		DurationOverThreshold:      (5 * time.Minute) + (2 * time.Minute) + (15 * time.Minute) + (10 * time.Minute) + (7 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet: 8,
		// 3 long tasks + 10 minutes of tasks should need 5 hosts
		// there is 1 idle task and 3 free hosts in the next 5 mins (factor = 1)
		// so we need 1 host
		ExpectedDuration:           (5 * time.Minute) + (2 * time.Minute) + (15 * time.Minute) + (30 * time.Second) + (10 * time.Minute) + (50 * time.Second) + (50 * time.Second) + (7 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHostWithContainers,
		CountDurationOverThreshold: 5,
		TaskGroupInfos: []model.TaskGroupInfo{
			taskGroupInfo,
		},
	}

	defaultPct := s.distro.HostAllocatorSettings.FutureHostFraction
	s.distro.HostAllocatorSettings.FutureHostFraction = 1
	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3, h4, h5, h6, h7},
		UsesContainers:  true,
		DistroQueueInfo: distroQueueInfo,
		ContainerPool: &evergreen.ContainerPool{
			Id:            "test-pool",
			MaxContainers: 10,
			Distro:        "parent-distro",
		},
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.distro.HostAllocatorSettings.FutureHostFraction = defaultPct
	s.NoError(err)
	s.Equal(3, hosts)
	s.Equal(3, free)
}

func (s *UtilizationAllocatorSuite) TestOnlyTaskGroupsOnlyScheduled() {
	name := fmt.Sprintf("%s_%s_%s_%s", "tg1", "", "", "")

	taskGroupInfo := model.TaskGroupInfo{
		Name:                       name,
		Count:                      10,
		MaxHosts:                   2,
		ExpectedDuration:           10 * (30 * time.Minute),
		CountDurationOverThreshold: 10,
		DurationOverThreshold:      10 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet: 10,
		// a long queue of task group tasks with max hosts=2 should request 2
		ExpectedDuration:           10 * (30 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 10,
		TaskGroupInfos: []model.TaskGroupInfo{
			taskGroupInfo,
		},
	}

	defaultPct := s.distro.HostAllocatorSettings.FutureHostFraction
	s.distro.HostAllocatorSettings.FutureHostFraction = 1
	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.distro.HostAllocatorSettings.FutureHostFraction = defaultPct
	s.NoError(err)
	s.Equal(2, hosts)
	s.Equal(0, free)
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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:                "t2",
		Project:           s.projectName,
		ExpectedDuration:  15 * time.Minute,
		BuildVariant:      "bv1",
		TaskGroup:         "g1",
		TaskGroupMaxHosts: 3,
		StartTime:         time.Now(),
	}
	s.NoError(t2.Insert(s.T().Context()))
	t3 := task.Task{
		Id:                "t3",
		Project:           s.projectName,
		ExpectedDuration:  15 * time.Minute,
		BuildVariant:      "bv1",
		TaskGroup:         "g2",
		TaskGroupMaxHosts: 1,
		StartTime:         time.Now(),
	}
	s.NoError(t3.Insert(s.T().Context()))

	group1 := fmt.Sprintf("%s_%s_%s_%s", "g1", "bv1", s.projectName, "v1")
	taskGroupInfo1 := model.TaskGroupInfo{
		Name:                       group1,
		Count:                      1,
		MaxHosts:                   3,
		ExpectedDuration:           15 * time.Minute,
		CountDurationOverThreshold: 0,
		DurationOverThreshold:      0,
	}

	group2 := fmt.Sprintf("%s_%s_%s_%s", "g2", "bv1", s.projectName, "v1")
	taskGroupInfo2 := model.TaskGroupInfo{
		Name:                       group2,
		Count:                      4,
		MaxHosts:                   1,
		ExpectedDuration:           4 * (30 * time.Minute),
		CountDurationOverThreshold: 4,
		DurationOverThreshold:      4 * (30 * time.Minute),
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet:  taskGroupInfo1.Count + taskGroupInfo2.Count,
		ExpectedDuration:           taskGroupInfo1.ExpectedDuration + taskGroupInfo2.ExpectedDuration,
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: taskGroupInfo1.CountDurationOverThreshold + taskGroupInfo2.CountDurationOverThreshold,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo1, taskGroupInfo2},
	}

	defaultPct := s.distro.HostAllocatorSettings.FutureHostFraction
	s.distro.HostAllocatorSettings.FutureHostFraction = 1
	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.distro.HostAllocatorSettings.FutureHostFraction = defaultPct
	s.NoError(err)
	s.Equal(0, hosts)
	s.Equal(1, free)
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
	s.NoError(t1.Insert(s.T().Context()))
	t2 := task.Task{
		Id:                "t2",
		Project:           s.projectName,
		ExpectedDuration:  15 * time.Minute,
		BuildVariant:      "bv1",
		TaskGroup:         "g1",
		TaskGroupMaxHosts: 3,
		StartTime:         time.Now(),
	}
	s.NoError(t2.Insert(s.T().Context()))
	t3 := task.Task{
		Id:                "t3",
		Project:           s.projectName,
		ExpectedDuration:  15 * time.Minute,
		BuildVariant:      "bv1",
		TaskGroup:         "g2",
		TaskGroupMaxHosts: 1,
		StartTime:         time.Now(),
	}
	s.NoError(t3.Insert(s.T().Context()))
	t4 := task.Task{
		Id:               "t4",
		Project:          s.projectName,
		ExpectedDuration: 5 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-5 * time.Minute),
	}
	s.NoError(t4.Insert(s.T().Context()))
	t5 := task.Task{
		Id:               "t5",
		Project:          s.projectName,
		ExpectedDuration: 30 * time.Minute,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t5.Insert(s.T().Context()))
	t6 := task.Task{
		Id:               "t6",
		Project:          s.projectName,
		ExpectedDuration: 2 * time.Hour,
		BuildVariant:     "bv1",
		StartTime:        time.Now().Add(-10 * time.Minute),
	}
	s.NoError(t6.Insert(s.T().Context()))
	t7 := task.Task{
		Id:                "t7",
		Project:           s.projectName,
		ExpectedDuration:  15 * time.Minute,
		BuildVariant:      "bv1",
		TaskGroup:         "g3",
		TaskGroupMaxHosts: 1,
		StartTime:         time.Now(),
	}
	s.NoError(t7.Insert(s.T().Context()))

	group1 := fmt.Sprintf("%s_%s_%s_%s", "g1", "bv1", s.projectName, "v1")
	taskGroupInfo1 := model.TaskGroupInfo{
		Name:                       group1,
		Count:                      2,
		MaxHosts:                   3,
		ExpectedDuration:           2 * (30 * time.Minute),
		CountDurationOverThreshold: 2,
		DurationOverThreshold:      2 * (30 * time.Minute),
	}

	group2 := fmt.Sprintf("%s_%s_%s_%s", "g2", "bv1", s.projectName, "v1")
	taskGroupInfo2 := model.TaskGroupInfo{
		Name:                       group2,
		Count:                      2,
		MaxHosts:                   1,
		ExpectedDuration:           2 * (30 * time.Minute),
		CountDurationOverThreshold: 2,
		DurationOverThreshold:      2 * (30 * time.Minute),
	}

	group3 := ""
	taskGroupInfo3 := model.TaskGroupInfo{
		Name:                       group3,
		Count:                      6,
		MaxHosts:                   0,
		ExpectedDuration:           (15 * time.Minute) + (5 * time.Minute) + (20 * time.Minute) + (15 * time.Minute) + (15 * time.Minute) + (5 * time.Minute),
		CountDurationOverThreshold: 0,
		DurationOverThreshold:      0,
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet:  taskGroupInfo1.Count + taskGroupInfo2.Count + taskGroupInfo3.Count,
		ExpectedDuration:           taskGroupInfo1.ExpectedDuration + taskGroupInfo2.ExpectedDuration + taskGroupInfo3.ExpectedDuration,
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: taskGroupInfo1.CountDurationOverThreshold + taskGroupInfo2.CountDurationOverThreshold + taskGroupInfo3.CountDurationOverThreshold,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo1, taskGroupInfo2, taskGroupInfo3},
	}

	defaultPct := s.distro.HostAllocatorSettings.FutureHostFraction
	s.distro.HostAllocatorSettings.FutureHostFraction = 1
	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3, h4, h5, h6, h7},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.distro.HostAllocatorSettings.FutureHostFraction = defaultPct
	s.NoError(err)
	s.Equal(2, hosts)
	s.Equal(2, free)
}

func (s *UtilizationAllocatorSuite) TestTaskGroupsCanReuseFreeHosts() {
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
		Name:                       name,
		Count:                      3,
		MaxHosts:                   3,
		ExpectedDuration:           3 * (30 * time.Minute),
		CountDurationOverThreshold: 3,
		DurationOverThreshold:      3,
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet:  taskGroupInfo.Count,
		ExpectedDuration:           taskGroupInfo.ExpectedDuration,
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: taskGroupInfo.CountDurationOverThreshold,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo},
	}

	defaultPct := s.distro.HostAllocatorSettings.FutureHostFraction
	s.distro.HostAllocatorSettings.FutureHostFraction = 1
	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.distro.HostAllocatorSettings.FutureHostFraction = defaultPct
	s.NoError(err)
	s.Equal(0, hosts)
	s.Equal(3, free)
}

func (s *UtilizationAllocatorSuite) TestTaskGroupsDontReuseFreeHostsWhenOtherTasksInQueue() {
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
		Name:                       name,
		Count:                      3,
		MaxHosts:                   3,
		ExpectedDuration:           3 * (30 * time.Minute),
		CountDurationOverThreshold: 3,
		DurationOverThreshold:      3,
	}

	standaloneTasks := model.TaskGroupInfo{
		Name:                       "",
		Count:                      3,
		ExpectedDuration:           5 * (30 * time.Minute),
		CountDurationOverThreshold: 2,
		DurationOverThreshold:      60 * time.Minute,
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet:  taskGroupInfo.Count + standaloneTasks.Count,
		ExpectedDuration:           taskGroupInfo.ExpectedDuration,
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: taskGroupInfo.CountDurationOverThreshold,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo, standaloneTasks},
	}

	defaultPct := s.distro.HostAllocatorSettings.FutureHostFraction
	s.distro.HostAllocatorSettings.FutureHostFraction = 1
	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.distro.HostAllocatorSettings.FutureHostFraction = defaultPct
	s.NoError(err)
	s.Equal(3, hosts)
	s.Equal(3, free)
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
	s.NoError(t1.Insert(s.T().Context()))
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
	s.NoError(t2.Insert(s.T().Context()))
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
	s.NoError(t3.Insert(s.T().Context()))
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
	s.NoError(t4.Insert(s.T().Context()))

	taskGroupInfo := model.TaskGroupInfo{
		Name:                       "",
		Count:                      5,
		ExpectedDuration:           5 * (30 * time.Minute),
		CountDurationOverThreshold: 2,
		DurationOverThreshold:      60 * time.Minute,
	}

	distroQueueInfo := model.DistroQueueInfo{
		LengthWithDependenciesMet: 5,
		// 2 running tasks that will take forever + 2 tasks that each should finish now means
		// we should have 1 expected free host after scaling by the 0.5 fraction
		// if we have 5 more tasks coming in, we should request 4 hosts
		ExpectedDuration:           5 * (30 * time.Minute),
		MaxDurationThreshold:       evergreen.MaxDurationPerDistroHost,
		CountDurationOverThreshold: 2,
		TaskGroupInfos:             []model.TaskGroupInfo{taskGroupInfo},
	}

	hostAllocatorData := HostAllocatorData{
		Distro:          s.distro,
		ExistingHosts:   []host.Host{h1, h2, h3, h4},
		DistroQueueInfo: distroQueueInfo,
	}

	hosts, free, err := UtilizationBasedHostAllocator(s.ctx, &hostAllocatorData)
	s.NoError(err)
	s.Equal(4, hosts)
	s.Equal(1, free)
}
