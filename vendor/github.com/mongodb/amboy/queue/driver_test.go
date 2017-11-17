package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

// All drivers should be able to pass this suite of tests which
// exercise the complete functionality of the interface, without
// reaching into the implementation details of any specific interface.

type DriverSuite struct {
	uuid              string
	driver            Driver
	driverConstructor func() Driver
	tearDown          func()
	suite.Suite
}

// Each driver should invoke this suite:

func TestDriverSuiteWithLocalInstance(t *testing.T) {
	tests := new(DriverSuite)
	tests.uuid = uuid.NewV4().String()
	tests.driverConstructor = func() Driver {
		return NewInternalDriver()
	}

	suite.Run(t, tests)
}

func TestDriverSuiteWithPriorityInstance(t *testing.T) {
	tests := new(DriverSuite)
	tests.uuid = uuid.NewV4().String()
	tests.driverConstructor = func() Driver {
		return NewPriorityDriver()
	}

	suite.Run(t, tests)
}

func TestDriverSuiteWithMongoDBInstance(t *testing.T) {
	tests := new(DriverSuite)
	tests.uuid = uuid.NewV4().String()
	mDriver := NewMongoDBDriver(
		"test-"+tests.uuid,
		DefaultMongoDBOptions()).(*mongoDB)
	tests.driverConstructor = func() Driver {
		return mDriver
	}
	tests.tearDown = func() {
		session, jobs := mDriver.getJobsCollection()
		defer session.Close()
		err := jobs.DropCollection()
		grip.Infof("removed %s collection (%+v)", jobs.Name, err)
	}

	suite.Run(t, tests)
}

// Implementation of the suite:

func (s *DriverSuite) SetupSuite() {
	job.RegisterDefaultJobs()
}

func (s *DriverSuite) SetupTest() {
	s.driver = s.driverConstructor()
	s.NoError(s.driver.Open(context.Background()))
}

func (s *DriverSuite) TearDownTest() {
	if s.tearDown != nil {
		s.tearDown()
	}
}

func (s *DriverSuite) TestInitialValues() {
	stats := s.driver.Stats()
	s.Equal(0, stats.Completed)
	s.Equal(0, stats.Running)
	s.Equal(0, stats.Pending)
	s.Equal(0, stats.Blocked)
	s.Equal(0, stats.Total)
}

func (s *DriverSuite) TestPutJobDoesNotAllowDuplicateIds() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.NoError(s.driver.Open(ctx))

	j := job.NewShellJob("echo foo", "")

	err := s.driver.Put(j)
	s.NoError(err)

	for i := 0; i < 10; i++ {
		s.Error(s.driver.Put(j))
	}
}

func (s *DriverSuite) TestSaveJobPersistsJobInDriver() {
	j := job.NewShellJob("echo foo", "")

	s.Equal(0, s.driver.Stats().Total)

	err := s.driver.Save(j)
	s.NoError(err)

	s.Equal(1, s.driver.Stats().Total)

	// saving a job a second time shouldn't be an error on save
	// and shouldn't result in a new job
	err = s.driver.Save(j)
	s.NoError(err)

	s.Equal(1, s.driver.Stats().Total)
}

func (s *DriverSuite) TestSaveAndGetRoundTripObjects() {
	j := job.NewShellJob("echo foo", "")
	name := j.ID()

	s.Equal(0, s.driver.Stats().Total)

	err := s.driver.Save(j)
	s.NoError(err)

	s.Equal(1, s.driver.Stats().Total)
	n, err := s.driver.Get(name)

	if s.NoError(err) {
		nsh := n.(*job.ShellJob)
		s.Equal(nsh.ID(), j.ID())
		s.Equal(nsh.Command, j.Command)

		s.Equal(n.ID(), name)
		s.Equal(1, s.driver.Stats().Total)
	}
}

func (s *DriverSuite) TestReloadRefreshesJobFromMemory() {
	j := job.NewShellJob("echo foo", "")

	originalCommand := j.Command
	err := s.driver.Save(j)
	s.NoError(err)

	s.Equal(1, s.driver.Stats().Total)

	newCommand := "echo bar"
	j.Command = newCommand

	err = s.driver.Save(j)
	s.Equal(1, s.driver.Stats().Total)
	s.NoError(err)

	reloadedJob, err := s.driver.Get(j.ID())
	s.NoError(err)
	j = reloadedJob.(*job.ShellJob)
	s.NotEqual(originalCommand, j.Command)
	s.Equal(newCommand, j.Command)
}

func (s *DriverSuite) TestGetReturnsErrorIfJobDoesNotExist() {
	j, err := s.driver.Get("does-not-exist")
	s.Error(err)
	s.Nil(j)
}

func (s *DriverSuite) TestStatsCallReportsCompletedJobs() {
	j := job.NewShellJob("echo foo", "")

	s.Equal(0, s.driver.Stats().Total)
	s.NoError(s.driver.Save(j))
	s.Equal(1, s.driver.Stats().Total)
	s.Equal(0, s.driver.Stats().Completed)
	s.Equal(1, s.driver.Stats().Pending)
	s.Equal(0, s.driver.Stats().Blocked)
	s.Equal(0, s.driver.Stats().Running)

	j.MarkComplete()
	s.NoError(s.driver.Save(j))
	s.Equal(1, s.driver.Stats().Total)
	s.Equal(1, s.driver.Stats().Completed)
	s.Equal(0, s.driver.Stats().Pending)
	s.Equal(0, s.driver.Stats().Blocked)
	s.Equal(0, s.driver.Stats().Running)
}

func (s *DriverSuite) TestNextMethodReturnsJob() {
	ctx := context.Background()
	s.Equal(0, s.driver.Stats().Total)

	j := job.NewShellJob("echo foo", "")

	s.NoError(s.driver.Put(j))
	stats := s.driver.Stats()
	s.Equal(1, stats.Total)
	s.Equal(1, stats.Pending)

	nj := s.driver.Next(ctx)
	stats = s.driver.Stats()
	s.Equal(0, stats.Completed)
	s.Equal(1, stats.Pending)
	s.Equal(0, stats.Blocked)
	s.Equal(0, stats.Running)

	if s.NotNil(nj) {
		s.Equal(j.ID(), nj.ID())
		s.NoError(s.driver.Lock(j))
	}
}

func (s *DriverSuite) TestNextMethodSkipsCompletedJos() {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	j := job.NewShellJob("echo foo", "")
	j.MarkComplete()

	s.NoError(s.driver.Save(j))
	s.Equal(1, s.driver.Stats().Total)

	s.Equal(1, s.driver.Stats().Total)
	s.Equal(0, s.driver.Stats().Blocked)
	s.Equal(0, s.driver.Stats().Pending)
	s.Equal(1, s.driver.Stats().Completed)

	s.Nil(s.driver.Next(ctx), fmt.Sprintf("%T", s.driver))
}

func (s *DriverSuite) TestJobsMethodReturnsAllJobs() {
	mocks := make(map[string]*job.ShellJob)

	for idx := range [24]int{} {
		name := fmt.Sprintf("echo test num %d", idx)
		j := job.NewShellJob(name, "")
		s.NoError(s.driver.Save(j))
		mocks[j.ID()] = j
	}

	counter := 0
	for j := range s.driver.Jobs() {
		task := j.(*job.ShellJob)
		counter++
		mock, ok := mocks[j.ID()]
		if s.True(ok) {
			s.Equal(mock.ID(), task.ID())
			s.Equal(mock.Command, task.Command)
		}
	}

	s.Equal(counter, len(mocks))
}

func (s *DriverSuite) TestStatsMethodReturnsAllJobs() {
	names := make(map[string]struct{})

	for i := 0; i < 30; i++ {
		cmd := fmt.Sprintf("echo 'foo: %d'", i)
		j := job.NewShellJob(cmd, "")

		s.NoError(s.driver.Save(j))
		names[j.ID()] = struct{}{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	counter := 0
	for stat := range s.driver.JobStats(ctx) {
		_, ok := names[stat.ID]
		s.True(ok)
		counter++
	}
	s.Equal(len(names), counter)
	s.Equal(counter, 30)
}
