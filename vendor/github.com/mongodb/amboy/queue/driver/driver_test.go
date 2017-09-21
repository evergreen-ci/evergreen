package driver

import (
	"fmt"
	"testing"

	"github.com/mongodb/amboy/job"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/mongodb/grip"
	"golang.org/x/net/context"
)

// All drivers should be able to pass this suite of tests which
// exercise the complete functionality of the interface, without
// reaching into the implementation details of any specific interface.

type DriverSuite struct {
	uuid              string
	driver            Driver
	driverConstructor func() Driver
	tearDown          func()
	require           *require.Assertions
	suite.Suite
}

// Each driver should invoke this suite:

func TestDriverSuiteWithLocalInstance(t *testing.T) {
	tests := new(DriverSuite)
	tests.uuid = uuid.NewV4().String()
	tests.driverConstructor = func() Driver {
		return NewInternal()
	}

	suite.Run(t, tests)
}

func TestDriverSuiteWithPriorityInstance(t *testing.T) {
	tests := new(DriverSuite)
	tests.uuid = uuid.NewV4().String()
	tests.driverConstructor = func() Driver {
		return NewPriority()
	}

	suite.Run(t, tests)
}

func TestDriverSuiteWithMongoDBInstance(t *testing.T) {
	tests := new(DriverSuite)
	tests.uuid = uuid.NewV4().String()
	mDriver := NewMongoDB(
		"test-"+tests.uuid,
		DefaultMongoDBOptions())
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
	s.require = s.Require()
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
	s.Equal(0, s.driver.Stats().Total)
	s.Nil(s.driver.Next())

	j := job.NewShellJob("echo foo", "")

	s.NoError(s.driver.Save(j))
	stats := s.driver.Stats()
	s.Equal(1, stats.Total)
	s.Equal(1, stats.Pending)

	nj := s.driver.Next()
	stats = s.driver.Stats()
	s.Equal(0, stats.Completed)
	s.Equal(1, stats.Pending)
	s.Equal(0, stats.Blocked)
	s.Equal(0, stats.Running)

	if s.NotNil(nj) {
		s.Equal(j.ID(), nj.ID())
		s.NoError(s.driver.Lock(j))
		// won't dispatch the same job more than once.
		s.Nil(s.driver.Next())
		s.Nil(s.driver.Next())
		s.Nil(s.driver.Next())
	}
}

func (s *DriverSuite) TestNextMethodSkipsCompletedJos() {
	j := job.NewShellJob("echo foo", "")
	j.MarkComplete()

	s.NoError(s.driver.Save(j))
	s.Equal(1, s.driver.Stats().Total)

	s.Equal(1, s.driver.Stats().Total)
	s.Equal(0, s.driver.Stats().Blocked)
	s.Equal(0, s.driver.Stats().Pending)
	s.Equal(1, s.driver.Stats().Completed)

	s.Nil(s.driver.Next(), fmt.Sprintf("%T", s.driver))
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
