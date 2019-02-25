package scheduler

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/distroqueue"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/suite"
)

type fuzzerSettings struct {
	taskDurations             []time.Duration
	expectedDurationMax       time.Duration
	maxNumTasks               int // max # of tasks in the queue
	maxRunningHosts           int // max # of hosts running a task
	maxStartTimeOffsetMinutes int // max time before now that a task has been running
	maxNumFreeHosts           int // max # of hosts running no task
	numIterations             int // # of tests to run
}

type HostAllocatorFuzzerSuite struct {
	ctx              context.Context
	distroName       string
	distro           distro.Distro
	projectName      string
	freeHostFraction float64
	allocator        HostAllocator
	testData         HostAllocatorData
	soonToBeFree     float64
	freeHosts        int
	settings         fuzzerSettings

	suite.Suite
}

func TestHostAllocatorFuzzer(t *testing.T) {
	s := &HostAllocatorFuzzerSuite{}
	s.allocator = UtilizationBasedHostAllocator
	suite.Run(t, s)
}

// SetupSuite sets the static data for the test suite
func (s *HostAllocatorFuzzerSuite) SetupSuite() {
	rand.Seed(time.Now().UnixNano())
	s.distroName = "testDistro"
	s.distro = distro.Distro{
		Id:       s.distroName,
		PoolSize: 100,
		Provider: evergreen.ProviderNameEc2Auto,
	}
	s.projectName = "testProject"
	s.freeHostFraction = 0.5
	s.ctx = context.Background()
	s.settings = fuzzerSettings{
		taskDurations: []time.Duration{
			10 * time.Second,
			1 * time.Minute,
			5 * time.Minute,
			10 * time.Minute,
			15 * time.Minute,
			20 * time.Minute,
			30 * time.Minute,
			1 * time.Hour,
		},
		expectedDurationMax:       1 * time.Hour,
		maxNumTasks:               100,
		maxRunningHosts:           100,
		maxStartTimeOffsetMinutes: 30,
		maxNumFreeHosts:           5,
		numIterations:             100,
	}
}

// randomizeData sets the data that will be randomized for each test run
func (s *HostAllocatorFuzzerSuite) randomizeData() {
	s.NoError(db.Clear(task.Collection))
	s.soonToBeFree = 0

	// generate a random number of scheduled tasks with random durations
	numTasks := rand.Intn(s.settings.maxNumTasks) + 1
	var expectedDuration time.Duration
	for i := 0; i < numTasks; i++ {
		duration := rand.Int63n(int64(s.settings.expectedDurationMax))
		expectedDuration += time.Duration(duration)
	}

	distroQueueInfo := distroqueue.DistroQueueInfo{
		Length:           numTasks,
		ExpectedDuration: expectedDuration,
	}

	// generate a random number of hosts running tasks
	numHosts := rand.Intn(s.settings.maxRunningHosts) + 1
	hosts := []host.Host{}
	for i := 0; i < numHosts; i++ {
		duration := s.settings.taskDurations[rand.Intn(len(s.settings.taskDurations))]
		offset := rand.Intn(s.settings.maxStartTimeOffsetMinutes)
		t := task.Task{
			Id:               fmt.Sprintf("t%d", i),
			Project:          s.projectName,
			ExpectedDuration: duration,
			BuildVariant:     "bv1",
			StartTime:        time.Now().Add(-1 * time.Duration(offset) * time.Minute),
		}
		s.NoError(t.Insert())
		// add up the fraction of hosts free for comparison later
		fractionFree := float64(time.Duration(offset)*time.Minute) / float64(MaxDurationPerDistroHost)
		if fractionFree > 1 {
			fractionFree = 1
		}
		s.soonToBeFree += fractionFree

		h := host.Host{
			Id:          fmt.Sprintf("h%d", i),
			RunningTask: t.Id,
		}
		hosts = append(hosts, h)
	}

	// generate some number of free hosts
	s.freeHosts = rand.Intn(s.settings.maxNumFreeHosts) + 1
	for i := 0; i < s.freeHosts; i++ {
		h := host.Host{
			Id:          fmt.Sprintf("hf%d", i),
			RunningTask: "",
		}
		hosts = append(hosts, h)
	}

	s.testData = HostAllocatorData{
		Distro:           s.distro,
		ExistingHosts:    hosts,
		FreeHostFraction: s.freeHostFraction,
		DistroQueueInfo:  distroQueueInfo,
	}
}

func (s *HostAllocatorFuzzerSuite) TestHeuristics() {
	for i := 0; i < s.settings.numIterations; i++ {
		s.randomizeData()
		newHosts, err := s.allocator(s.ctx, s.testData)
		s.NoError(err)
		distroQueueInfo := s.testData.DistroQueueInfo
		queueSize := distroQueueInfo.Length
		queueDuration := distroQueueInfo.ExpectedDuration

		s.True(newHosts >= 0)
		s.True(newHosts <= queueSize)
		numFree := float64(newHosts+s.freeHosts) + math.Ceil(s.soonToBeFree*s.freeHostFraction)
		// the task duration per host will always be less than 2x the max duration per host (30min)
		// because the longest task used in this test is 1 hr
		s.True(queueDuration.Hours()/numFree < float64(2*MaxDurationPerDistroHost),
			"queue: %v, new: %d, free: %d, soon: %f", queueDuration, newHosts, s.freeHosts, s.soonToBeFree)
	}
}
