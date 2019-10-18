// +build go1.7

package cloud

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
)

func (s *GCESuite) TestGetComputePrices() {
	// repeat multiple times to test caching
	for i := 0; i < 3; i++ {
		prices, err := getComputePrices()
		s.NoError(err)
		s.NotNil(prices)
		s.NotNil(prices.StandardMachine)
		s.NotNil(prices.CustomCPUs)
		s.NotNil(prices.CustomMemory)

		// standard images are parsed and have prices
		machine := machineType{
			Region: "us-central1",
			Name:   "n1-standard-4",
		}
		s.NotZero(prices.StandardMachine[machine])

		machine = machineType{
			Region: "asia-southeast",
			Name:   "n1-highcpu-64",
		}
		s.NotZero(prices.StandardMachine[machine])

		// custom images are parsed and have prices
		s.NotZero(prices.CustomCPUs["us-east1"])
		s.NotZero(prices.CustomMemory["us-west1"])
	}
}

func (s *GCESuite) TestZoneToRegion() {
	// test valid inputs
	reg, err := zoneToRegion("us-west1-c")
	s.NoError(err)
	s.Equal("us-west1", reg)

	reg, err = zoneToRegion("europe-west2-a")
	s.NoError(err)
	s.Equal("europe-west2", reg)

	reg, err = zoneToRegion("asia-northeast1-c")
	s.NoError(err)
	s.Equal("asia-northeast1", reg)

	reg, err = zoneToRegion("abcd-01234-abc")
	s.NoError(err)
	s.Equal("abcd-01234", reg)

	// test invalid inputs
	reg, err = zoneToRegion("us-central1")
	s.Error(err)
	s.Empty(reg)

	reg, err = zoneToRegion("")
	s.Error(err)
	s.Empty(reg)
}

func (s *GCESuite) TestParseMachineType() {
	// custom machine type
	custom := "zones/us-central1-c/machineTypes/custom-2-2048"
	exp := &machineType{
		Region:   "us-central1",
		Custom:   true,
		NumCPUs:  2,
		MemoryMB: 2048,
	}
	m, err := parseMachineType(custom)
	s.NoError(err)
	s.Equal(exp, m)

	// standard machine type
	standard := "zones/us-central1-c/machineTypes/n1-standard-8"
	exp = &machineType{
		Region: "us-central1",
		Custom: false,
		Name:   "n1-standard-8",
	}
	m, err = parseMachineType(standard)
	s.NoError(err)
	s.Equal(exp, m)

	// invalid machine type
	invalid := "zones/us-central1"
	m, err = parseMachineType(invalid)
	s.Error(err)
	s.Nil(m)
}

func (s *GCESuite) TestInstanceCostCustomMachine() {
	m := &machineType{
		Region:   "us-east1",
		Custom:   true,
		NumCPUs:  2,
		MemoryMB: 2048,
	}
	dur, _ := time.ParseDuration("30m")

	cost, err := instanceCost(m, dur)
	s.NoError(err)
	s.True(cost > 0)
}

func (s *GCESuite) TestInstanceCostStandardMachine() {
	m := &machineType{
		Region: "us-central1",
		Custom: false,
		Name:   "n1-standard-8",
	}
	dur, _ := time.ParseDuration("30m")

	cost, err := instanceCost(m, dur)
	s.NoError(err)
	s.True(cost > 0)
}

func (s *GCESuite) TestCostForDuration() {
	start := time.Now()
	end := time.Now()
	h := &host.Host{Id: "id"}

	// end before start
	cost, err := s.manager.CostForDuration(h, end, start)
	s.Error(err)
	s.Zero(cost)

	// valid input
	cost, err = s.manager.CostForDuration(h, start, end)
	s.NoError(err)
	s.NotZero(cost)
}

func (s *GCESuite) TestTimeTilNextPayment() {
	now := time.Now()
	smallTask, _ := time.ParseDuration("-5m")
	mediumTask, _ := time.ParseDuration("-25m")
	largeTask, _ := time.ParseDuration("-3h")

	// pristineHost has just been created and not run any tasks
	pristineHost := &host.Host{
		CreationTime: now,
	}
	dur := s.manager.TimeTilNextPayment(pristineHost)
	s.True(dur.Seconds() > BufferTime.Seconds())
	s.True(dur.Seconds() < MinUptime.Seconds())

	// youngHost has just been created and run a short task
	youngHost := &host.Host{
		LastTaskCompletedTime: now,
		CreationTime:          now.Add(smallTask),
	}
	dur = s.manager.TimeTilNextPayment(youngHost)
	s.True(dur.Seconds() > BufferTime.Seconds())
	s.True(dur.Seconds() < MinUptime.Seconds())

	// averageHost has just been created and run a longer task
	// that puts its uptime at just under minUptime
	averageHost := &host.Host{
		LastTaskCompletedTime: now,
		CreationTime:          now.Add(mediumTask),
	}
	dur = s.manager.TimeTilNextPayment(averageHost)
	s.True(dur.Seconds() > 0)
	s.True(dur.Seconds() < BufferTime.Seconds())

	// seasonedHost has run many tasks since its creation
	seasonedHost := &host.Host{
		LastTaskCompletedTime: now,
		CreationTime:          now.Add(largeTask),
	}
	dur = s.manager.TimeTilNextPayment(seasonedHost)
	s.True(dur.Seconds() > 0)
	s.True(dur.Seconds() < BufferTime.Seconds())
}
