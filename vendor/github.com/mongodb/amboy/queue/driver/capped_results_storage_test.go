package driver

import (
	"fmt"
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CappedResultsSuite struct {
	size    int
	cr      *CappedResultStorage
	require *require.Assertions
	suite.Suite
}

func TestCappedResultsSuiteSizeFive(t *testing.T) {
	s := new(CappedResultsSuite)
	s.size = 5
	suite.Run(t, s)
}

func TestCappedResultsSuiteSizeFiveHundred(t *testing.T) {
	s := new(CappedResultsSuite)
	s.size = 500
	suite.Run(t, s)
}

func (s *CappedResultsSuite) SetupSuite() {
	s.require = s.Require()
}

func (s *CappedResultsSuite) SetupTest() {
	s.cr = NewCappedResultStorage(s.size)
}

func (s *CappedResultsSuite) TestInitialInstanceHasNoItems() {
	s.Len(s.cr.table, 0)
	s.Equal(0, s.cr.Size())
}

func (s *CappedResultsSuite) TestAddingJobsToStorageImpactsSize() {
	for i := 1; i <= s.size; i++ {
		s.cr.Add(job.NewShellJob("true", ""))
	}
	s.Len(s.cr.table, s.size)
	s.Equal(s.size, s.cr.Size())
}

func (s *CappedResultsSuite) TestJobsAreUniqueInQueueSafeToAddMultipleTimes() {
	j := job.NewShellJob("echo unique", "")

	s.cr.Add(j)
	s.Equal(1, s.cr.Size())
	s.cr.Add(j)
	s.Equal(1, s.cr.Size())
}

func (s *CappedResultsSuite) TestGetReturnsNamedJob() {
	j := job.NewShellJob("true", "")

	s.cr.Add(j)
	s.Equal(1, s.cr.Size())

	fetched, ok := s.cr.Get(j.ID())
	s.True(ok)
	s.Equal(j, fetched)
	s.Equal(1, s.cr.Size())
}

func (s *CappedResultsSuite) TestGetReturnsNilWhenJobDoesNotExist() {
	j := job.NewShellJob("true", "")

	s.cr.Add(j)
	s.Equal(1, s.cr.Size())

	fetched, ok := s.cr.Get("foo")
	s.False(ok)
	s.Nil(fetched)
	s.Equal(1, s.cr.Size())
}

func (s *CappedResultsSuite) TestAddJobsToCapacityIncreasesSizeToCapacity() {
	s.Equal(0, s.cr.Size())
	for i := 1; i <= s.size; i++ {
		s.cr.Add(job.NewShellJob(fmt.Sprintf("echo job %d", i), ""))
		s.Equal(i, s.cr.Size())

	}
	s.Equal(s.size, s.cr.Size())
	s.cr.Add(job.NewShellJob("echo final job", ""))
	s.Equal(s.size, s.cr.Size())
}

func (s *CappedResultsSuite) TestJobsInStorageAreRetrievable() {
	s.Equal(0, s.cr.Size())
	mirror := make(map[string]amboy.Job)

	for i := 1; i <= s.size; i++ {
		j := job.NewShellJob(fmt.Sprintf("echo job %d", i), "")
		s.cr.Add(j)
		s.Equal(i, s.cr.Size())
		mirror[j.ID()] = j
	}

	s.Equal(len(mirror), s.cr.Size())

	for name, j := range mirror {
		sj, ok := s.cr.Get(name)
		s.True(ok)
		s.Equal(j, sj)
	}
}

func (s *CappedResultsSuite) TestJobsInStorageContentsAreExpected() {
	s.Equal(0, s.cr.Size())
	mirror := make(map[string]amboy.Job)

	for i := 1; i <= s.size; i++ {
		j := job.NewShellJob(fmt.Sprintf("echo job %d", i), "")
		s.cr.Add(j)
		s.Equal(i, s.cr.Size())
		mirror[j.ID()] = j
	}

	s.Equal(len(mirror), s.cr.Size())

	seen := 0
	for j := range s.cr.Contents() {
		seen++
		mj, ok := mirror[j.ID()]
		s.True(ok)
		s.Equal(j, mj)
	}

	s.Equal(s.size, seen)
}
