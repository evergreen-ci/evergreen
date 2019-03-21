package queue

import (
	"context"
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PriorityStorageSuite struct {
	ps      *priorityStorage
	require *require.Assertions
	suite.Suite
}

func TestPriorityStorageSuite(t *testing.T) {
	suite.Run(t, new(PriorityStorageSuite))
}

func (s *PriorityStorageSuite) SetupSuite() {
	s.require = s.Require()
}

func (s *PriorityStorageSuite) SetupTest() {
	s.ps = makePriorityStorage()
}

func (s *PriorityStorageSuite) TestInitialInstanceHasNoItems() {
	s.Len(s.ps.table, 0)
	s.Equal(0, s.ps.Pending())
	s.Equal(0, s.ps.Size())
}

func (s *PriorityStorageSuite) TestAddingJobsToStorageImpactsSize() {
	for i := 0; i < 10; i++ {
		s.NoError(s.ps.Insert(job.NewShellJob("true", "")))
	}
	s.Len(s.ps.table, 10)
	s.Equal(10, s.ps.Size())
	s.Equal(10, s.ps.Pending())
}

func (s *PriorityStorageSuite) TestJobsAreUniqueInQueueSafeToAddMultipleTimes() {
	j := job.NewShellJob("echo unique", "")

	s.NoError(s.ps.Insert(j))
	s.Equal(1, s.ps.Size())
	s.Equal(1, s.ps.Pending())

	s.ps.Save(j)
	s.Equal(1, s.ps.Size())
	s.Equal(1, s.ps.Pending())

	popedJob := s.ps.Pop()
	s.Equal(1, s.ps.Size())
	s.Equal(0, s.ps.Pending())
	s.Equal(popedJob, j)

	s.Error(s.ps.Insert(popedJob))
	s.Equal(1, s.ps.Size())
	s.Equal(0, s.ps.Pending())

	// legacy behavior holds that this is true for complete jobs.
	s.ps.Save(popedJob)
	s.Equal(1, s.ps.Size())
	s.Equal(0, s.ps.Pending())
}

func (s *PriorityStorageSuite) TestSaveExistingJobUpdatesPriorityInQueue() {
	first := job.NewShellJob("true", "")
	s.NoError(s.ps.Insert(first))
	for i := 0; i < 20; i++ {
		j := job.NewShellJob("echo heard", "")
		j.SetPriority(i + 1)
		s.ps.Save(j)
	}
	s.Equal(21, s.ps.Size())
	s.Equal(21, s.ps.Pending())

	firstOut := s.ps.Pop()
	s.Equal(20, s.ps.Pending())
	s.NotEqual(firstOut.ID(), first.ID())

	first.SetPriority(50)
	s.ps.Save(first)
	s.Equal(20, s.ps.Pending())
	s.Equal(21, s.ps.Size())

	secondOut := s.ps.Pop()
	s.Equal(first.ID(), secondOut.ID())
	s.Equal(19, s.ps.Pending())
	s.Equal(21, s.ps.Size())
}

func (s *PriorityStorageSuite) TestPopWithEmptyInstanceReturnsNil() {
	s.Equal(0, s.ps.Pending())

	s.Nil(s.ps.Pop())
}

func (s *PriorityStorageSuite) TestGetReturnsNamedJob() {
	j := job.NewShellJob("true", "")

	s.NoError(s.ps.Insert(j))
	s.Equal(1, s.ps.Size())
	s.Equal(1, s.ps.Pending())

	fetched, ok := s.ps.Get(j.ID())
	s.True(ok)
	s.Equal(j, fetched)
	s.Equal(1, s.ps.Size())
	s.Equal(1, s.ps.Pending())
}

func (s *PriorityStorageSuite) TestGetReturnsNilWhenJobDoesNotExist() {
	j := job.NewShellJob("true", "")

	s.NoError(s.ps.Insert(j))
	s.Equal(1, s.ps.Size())
	s.Equal(1, s.ps.Pending())

	fetched, ok := s.ps.Get("foo")
	s.False(ok)
	s.Nil(fetched)
	s.Equal(1, s.ps.Size())
	s.Equal(1, s.ps.Pending())
}

func (s *PriorityStorageSuite) TestJobServerPushesJobsInPriorityOrder() {
	for i := 0; i < 25; i++ {
		j := job.NewShellJob("echo ordered", "")
		j.SetPriority(i + 1)
		s.ps.Save(j)
	}

	s.Equal(25, s.ps.Size())
	s.Equal(25, s.ps.Pending())

	base := 25
	ctx, cancel := context.WithCancel(context.Background())
	output := make(chan amboy.Job)
	go s.ps.JobServer(ctx, output)
	defer cancel()

	for {
		fetched := <-output
		s.Equal(base, fetched.Priority())
		base--

		if base == 0 {
			break
		}
	}

	s.Equal(0, base)
	s.Equal(0, s.ps.Pending())
}

func (s *PriorityStorageSuite) TestCanceledJobServerReturnsEarly() {
	ctx, cancel := context.WithCancel(context.Background())
	output := make(chan amboy.Job)
	cancel()
	s.ps.JobServer(ctx, output) // run in main thread
	close(output)               // this would panic if the server were still running
}

func (s *PriorityStorageSuite) TestContentsGeneratorOutputIncludesAllJobs() {
	for i := 0; i < 50; i++ {
		j := job.NewShellJob("echo ordered", "")
		s.ps.Save(j)

		if i%3 == 0 {
			s.NotNil(s.ps.Pop())
		}
	}

	s.Equal(50, s.ps.Size())
	s.NotEqual(s.ps.Size(), s.ps.Pending())

	seen := 0
	for j := range s.ps.Contents() {
		s.NotNil(j)
		seen++
	}
	s.Equal(50, s.ps.Size())
	s.NotEqual(s.ps.Size(), s.ps.Pending())
	s.NotEqual(0, s.ps.Pending())
	s.Equal(seen, s.ps.Size())
}
