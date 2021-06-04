package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/google/uuid"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// All drivers should be able to pass this suite of tests which
// exercise the complete functionality of the interface, without
// reaching into the implementation details of any specific interface.

type DriverSuite struct {
	driver            remoteQueueDriver
	driverConstructor func() (remoteQueueDriver, error)
	tearDown          func() error
	ctx               context.Context
	cancel            context.CancelFunc
	suite.Suite
}

// Each driver should invoke this suite:

func TestDriverSuiteWithMongoDBInstance(t *testing.T) {
	tests := new(DriverSuite)
	for driverName, driverType := range map[string]struct {
		constructor func() (remoteQueueDriver, error)
		tearDown    func() error
	}{
		"Basic": {
			constructor: func() (remoteQueueDriver, error) {
				id := "test-" + uuid.New().String()
				opts := defaultMongoDBTestOptions()

				return newMongoDriver(id, opts)
			},
			tearDown: func() error {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				mDriver, ok := tests.driver.(*mongoDriver)
				if !ok {
					return errors.New("cannot tear down mongo driver tests because test suite is not running a mongo driver")
				}
				if err := mDriver.getCollection().Database().Drop(ctx); err != nil {
					return errors.Wrapf(err, "removing collection '%s'", mDriver.getCollection().Name())
				}
				return nil

			},
		},
		"Group": {
			constructor: func() (remoteQueueDriver, error) {
				id := "test-" + uuid.New().String()
				groupName := "group-" + uuid.New().String()
				opts := defaultMongoDBTestOptions()
				opts.UseGroups = true
				opts.GroupName = groupName

				return newMongoDriver(id, opts)
			},
			tearDown: func() error {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				mDriver, ok := tests.driver.(*mongoDriver)
				if !ok {
					return errors.New("cannot tear down mongo driver tests because test suite is not running a mongo driver")
				}
				if err := mDriver.getCollection().Database().Drop(ctx); err != nil {
					return errors.Wrapf(err, "removing collection '%s'", mDriver.getCollection().Name())
				}
				return nil

			},
		},
	} {
		t.Run(driverName, func(t *testing.T) {
			tests.driverConstructor = driverType.constructor
			tests.tearDown = driverType.tearDown

			suite.Run(t, tests)
		})
	}
}

// Implementation of the suite:

func (s *DriverSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

func (s *DriverSuite) SetupTest() {
	var err error
	s.driver, err = s.driverConstructor()
	s.Require().NoError(err)
	s.NoError(s.driver.Open(s.ctx))
}

func (s *DriverSuite) TearDownTest() {
	if s.tearDown != nil {
		s.Require().NoError(s.tearDown())
	}
}

func (s *DriverSuite) TearDownSuite() {
	s.cancel()
}

func (s *DriverSuite) TestInitialValues() {
	stats := s.driver.Stats(s.ctx)
	s.Equal(0, stats.Completed)
	s.Equal(0, stats.Running)
	s.Equal(0, stats.Pending)
	s.Equal(0, stats.Blocked)
	s.Equal(0, stats.Total)
}

func (s *DriverSuite) TestPutJobDoesNotAllowDuplicateIds() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	j := job.NewShellJob("echo foo", "")

	err := s.driver.Put(ctx, j)
	s.NoError(err)

	for i := 0; i < 10; i++ {
		err := s.driver.Put(ctx, j)
		s.Error(err)
		s.Require().True(amboy.IsDuplicateJobError(err))
		s.Require().False(amboy.IsDuplicateJobScopeError(err))
	}
}

func (s *DriverSuite) TestPutJobDoesNotAllowDuplicateScopeAppliedInQueue() {
	j1 := job.NewShellJob("echo foo", "")
	j2 := job.NewShellJob("echo bar", "")

	scopes := []string{"scope"}
	j1.SetShouldApplyScopesOnEnqueue(true)
	j1.SetScopes(scopes)
	j2.SetShouldApplyScopesOnEnqueue(true)
	j2.SetScopes(scopes)

	s.Require().NoError(s.driver.Put(s.ctx, j1))
	err := s.driver.Put(s.ctx, j2)
	s.True(amboy.IsDuplicateJobError(err))
	s.True(amboy.IsDuplicateJobScopeError(err))
}

func (s *DriverSuite) TestPutJobDoesNotApplyScopesInQueueByDefault() {
	j := job.NewShellJob("echo foo", "")
	j.SetScopes([]string{"scope"})

	s.Require().NoError(s.driver.Put(s.ctx, j))

	s.Equal(1, s.driver.Stats(s.ctx).Total)

	j = job.NewShellJob("echo bar", "")
	j.SetScopes([]string{"scope"})

	s.Require().NoError(s.driver.Put(s.ctx, j))

	s.Equal(2, s.driver.Stats(s.ctx).Total)
}

func (s *DriverSuite) TestPutJobAppliesScopesInQueueIfSet() {
	j := job.NewShellJob("echo foo", "")
	j.SetScopes([]string{"scope"})
	j.SetShouldApplyScopesOnEnqueue(true)

	s.Require().NoError(s.driver.Put(s.ctx, j))

	s.Equal(1, s.driver.Stats(s.ctx).Total)

	j = job.NewShellJob("echo bar", "")
	j.SetScopes([]string{"scope"})
	j.SetShouldApplyScopesOnEnqueue(true)

	s.Error(s.driver.Put(s.ctx, j))

	s.Equal(1, s.driver.Stats(s.ctx).Total)
}

func (s *DriverSuite) TestPutJobAllowsSameScopesInQueueIfDuplicateScopedJobDoesNotApplyScopesOnEnqueue() {
	j := job.NewShellJob("echo foo", "")
	j.SetScopes([]string{"scope"})
	j.SetShouldApplyScopesOnEnqueue(true)

	s.Require().NoError(s.driver.Put(s.ctx, j))

	s.Equal(1, s.driver.Stats(s.ctx).Total)

	j = job.NewShellJob("echo bar", "")
	j.SetScopes([]string{"scope"})

	s.Require().NoError(s.driver.Put(s.ctx, j))

	s.Equal(2, s.driver.Stats(s.ctx).Total)
}

func (s *DriverSuite) TestPutJobAllowsSameScopesInQueueIfInitialScopedJobDoesNotApplyScopeOnEnqueue() {
	j := job.NewShellJob("echo foo", "")
	j.SetScopes([]string{"scope"})

	s.Require().NoError(s.driver.Put(s.ctx, j))

	s.Equal(1, s.driver.Stats(s.ctx).Total)

	j = job.NewShellJob("echo bar", "")
	j.SetScopes([]string{"scope"})
	j.SetShouldApplyScopesOnEnqueue(true)

	s.Require().NoError(s.driver.Put(s.ctx, j))

	s.Equal(2, s.driver.Stats(s.ctx).Total)
}

func (s *DriverSuite) TestPutAndSaveJobSucceedsIfScopeIsAppliedOnEnqueue() {
	j := job.NewShellJob("echo foo", "")
	j.SetScopes([]string{"scope"})

	s.Require().NoError(s.driver.Put(s.ctx, j))
	s.Require().NoError(s.driver.Save(s.ctx, j))
}

func (s *DriverSuite) TestSaveJobPersistsJobInDriver() {
	j := job.NewShellJob("echo foo", "")

	s.Equal(0, s.driver.Stats(s.ctx).Total)

	err := s.driver.Put(s.ctx, j)
	s.NoError(err)

	s.Equal(1, s.driver.Stats(s.ctx).Total)

	// saving a job a second time shouldn't be an error on save
	// and shouldn't result in a new job
	err = s.driver.Save(s.ctx, j)
	s.NoError(err)

	s.Equal(1, s.driver.Stats(s.ctx).Total)
}

func (s *DriverSuite) TestPutAndGetJobRoundtripsSingleRetryableJob() {
	j := newMockRetryableJob("id")

	s.Require().NoError(s.driver.Put(s.ctx, j))

	storedJob, err := s.driver.Get(s.ctx, j.ID())
	s.Require().NoError(err)

	s.Equal(j.ID(), storedJob.ID())
	s.Equal(j.RetryInfo(), storedJob.RetryInfo())

	s.Equal(1, s.driver.Stats(s.ctx).Total)
}

func (s *DriverSuite) TestPutAndGetJobRoundtripsLatestRetryableJob() {
	j0 := newMockRetryableJob("id")
	jobID := j0.ID()
	j1 := newMockRetryableJob("id")
	j1.UpdateRetryInfo(amboy.JobRetryOptions{
		CurrentAttempt: utility.ToIntPtr(1),
	})
	j2 := newMockRetryableJob("id")
	j2.UpdateRetryInfo(amboy.JobRetryOptions{
		CurrentAttempt: utility.ToIntPtr(2),
	})

	s.Require().NoError(s.driver.Put(s.ctx, j1))
	s.Require().NoError(s.driver.Put(s.ctx, j0))
	s.Require().NoError(s.driver.Put(s.ctx, j2))

	storedJob, err := s.driver.Get(s.ctx, jobID)
	s.Require().NoError(err)

	s.Equal(jobID, storedJob.ID())
	s.Equal(j2.RetryInfo(), storedJob.RetryInfo())

	s.Equal(3, s.driver.Stats(s.ctx).Total)
}

func (s *DriverSuite) TestPutAndGetAttemptRoundtripsRetryableJob() {
	var jobs []amboy.Job
	var jobID string
	for i := 0; i < 3; i++ {
		j := newMockRetryableJob("id")
		jobID = j.ID()
		j.UpdateRetryInfo(amboy.JobRetryOptions{
			CurrentAttempt: utility.ToIntPtr(i),
		})
		s.Require().NoError(s.driver.Put(s.ctx, j))
	}

	for _, j := range jobs {
		storedJob, err := s.driver.GetAttempt(s.ctx, jobID, j.RetryInfo().CurrentAttempt)
		s.Require().NoError(err)

		s.Equal(jobID, j.ID())
		s.Equal(jobID, storedJob.ID())
		s.Equal(j.RetryInfo().CurrentAttempt, storedJob.RetryInfo().CurrentAttempt)
	}
	s.Equal(3, s.driver.Stats(s.ctx).Total)
}

func (s *DriverSuite) TestPutAndGetAttemptOnNonretryableJobFails() {
	j := newMockJob()
	j.SetID("foo")
	s.Require().NoError(s.driver.Put(s.ctx, j))

	storedJob, err := s.driver.GetAttempt(s.ctx, j.ID(), 0)
	s.Error(err)
	s.Zero(storedJob)
}

func (s *DriverSuite) TestPutAndGetRoundTripObjects() {
	j := job.NewShellJob("echo foo", "")
	name := j.ID()

	s.Equal(0, s.driver.Stats(s.ctx).Total)

	err := s.driver.Put(s.ctx, j)
	s.NoError(err)

	s.Equal(1, s.driver.Stats(s.ctx).Total)
	n, err := s.driver.Get(s.ctx, name)

	if s.NoError(err) {
		nsh := n.(*job.ShellJob)
		s.Equal(nsh.ID(), j.ID())
		s.Equal(nsh.Command, j.Command)

		s.Equal(n.ID(), name)
		s.Equal(1, s.driver.Stats(s.ctx).Total)
	}
}

func (s *DriverSuite) TestCompleteAndPutJobsUpdatesExistingAndAddsNewJob() {
	j1 := job.NewShellJob("echo foo", "")
	j2 := job.NewShellJob("echo bar", "")

	s.Require().NoError(s.driver.Put(s.ctx, j1))

	j1.SetStatus(amboy.JobStatusInfo{
		ModificationCount: 50,
	})
	j2.SetStatus(amboy.JobStatusInfo{
		ModificationCount: 50,
	})

	s.Require().NoError(s.driver.CompleteAndPut(s.ctx, j1, j2))

	reloaded1, err := s.driver.Get(s.ctx, j1.ID())
	s.Require().NoError(err)
	s.Equal(j1.Status().ModificationCount, reloaded1.Status().ModificationCount)

	reloaded2, err := s.driver.Get(s.ctx, j2.ID())
	s.Require().NoError(err)
	s.Equal(j2.Status().ModificationCount, reloaded2.Status().ModificationCount)
}

func (s *DriverSuite) TestCompleteAndPutJobsIsAtomic() {
	j := job.NewShellJob("echo foo", "")

	j.SetStatus(amboy.JobStatusInfo{
		ModificationCount: 5,
	})

	s.Require().NoError(s.driver.Put(s.ctx, j))

	j.SetStatus(amboy.JobStatusInfo{
		ModificationCount: 50,
	})

	s.Require().Error(s.driver.CompleteAndPut(s.ctx, j, j))

	reloaded, err := s.driver.Get(s.ctx, j.ID())
	s.Require().NoError(err)
	s.Equal(5, reloaded.Status().ModificationCount, "CompleteAndPut should be atomic")
}

func (s *DriverSuite) TestCompleteAndPutJobsAtomicallySwapsScopes() {
	j1 := job.NewShellJob("echo foo", "")
	j2 := job.NewShellJob("echo bar", "")

	j1.SetScopes([]string{"scope"})
	j1.SetShouldApplyScopesOnEnqueue(true)

	s.Require().NoError(s.driver.Put(s.ctx, j1))

	j2.SetScopes(j1.Scopes())
	j2.SetShouldApplyScopesOnEnqueue(true)
	j1.SetScopes(nil)

	s.Require().NoError(s.driver.CompleteAndPut(s.ctx, j1, j2))

	reloaded1, err := s.driver.Get(s.ctx, j1.ID())
	s.Require().NoError(err)
	s.Equal(j1.Scopes(), reloaded1.Scopes())

	reloaded2, err := s.driver.Get(s.ctx, j2.ID())
	s.Require().NoError(err)
	s.Equal(j2.Scopes(), reloaded2.Scopes())
}

func (s *DriverSuite) TestCompleteAndPutJobsFailsWithDuplicateJobID() {
	j1 := job.NewShellJob("echo foo", "")
	j2 := job.NewShellJob("echo bar", "")

	s.Require().NoError(s.driver.Put(s.ctx, j1))
	s.Require().NoError(s.driver.Put(s.ctx, j2))

	err := s.driver.CompleteAndPut(s.ctx, j1, j2)
	s.Require().Error(err)
	s.True(amboy.IsDuplicateJobError(err), "error: %v", err)
	s.False(amboy.IsDuplicateJobScopeError(err), "error: %v", err)
}

func (s *DriverSuite) TestCompleteAndPutJobsSucceedsWithDuplicateScopes() {
	j1 := job.NewShellJob("echo foo", "")
	j2 := job.NewShellJob("echo bar", "")
	j3 := job.NewShellJob("echo bat", "")

	scopes := []string{"scope"}
	j3.SetScopes(scopes)

	s.Require().NoError(s.driver.Put(s.ctx, j1))
	s.Require().NoError(s.driver.Put(s.ctx, j3))

	j2.SetScopes(scopes)

	s.NoError(s.driver.CompleteAndPut(s.ctx, j1, j2))
}

func (s *DriverSuite) TestCompleteAndPutJobsFailsWithDuplicateJobScopesAppliedOnEnqueue() {
	j1 := job.NewShellJob("echo foo", "")
	j2 := job.NewShellJob("echo bar", "")
	j3 := job.NewShellJob("echo bat", "")

	scopes := []string{"scope"}
	j3.SetScopes(scopes)
	j3.SetShouldApplyScopesOnEnqueue(true)

	s.Require().NoError(s.driver.Put(s.ctx, j1))
	s.Require().NoError(s.driver.Put(s.ctx, j3))

	j2.SetScopes(scopes)
	j2.SetShouldApplyScopesOnEnqueue(true)

	err := s.driver.CompleteAndPut(s.ctx, j1, j2)
	s.Require().Error(err)
	s.True(amboy.IsDuplicateJobError(err), "error: %v", err)
	s.True(amboy.IsDuplicateJobScopeError(err), "error: %v", err)
}

func (s *DriverSuite) TestCompleteMarksJobCompleted() {
	j := job.NewShellJob("echo foo", "")
	j.SetStatus(amboy.JobStatusInfo{
		InProgress: true,
		Owner:      s.driver.ID(),
	})
	s.Require().NoError(s.driver.Put(s.ctx, j))
	s.Require().NoError(s.driver.Complete(s.ctx, j))
	s.NotZero(utility.BSONTime(j.Status().ModificationTime))
	s.Zero(j.Status().ModificationCount)
}

func (s *DriverSuite) TestCompleteFailsWhenModCountDiffers() {
	j := job.NewShellJob("echo foo", "")
	j.SetStatus(amboy.JobStatusInfo{
		ModificationTime: time.Now(),
	})
	s.Require().NoError(s.driver.Put(s.ctx, j))
	stat := j.Status()
	stat.ModificationCount += 10
	j.SetStatus(stat)
	s.True(amboy.IsJobNotFoundError(s.driver.Complete(s.ctx, j)))
}

func (s *DriverSuite) TestReloadRefreshesJobFromMemory() {
	j := job.NewShellJob("echo foo", "")

	originalCommand := j.Command
	err := s.driver.Put(s.ctx, j)
	s.NoError(err)

	s.Equal(1, s.driver.Stats(s.ctx).Total)

	newCommand := "echo bar"
	j.Command = newCommand

	err = s.driver.Save(s.ctx, j)
	s.Equal(1, s.driver.Stats(s.ctx).Total)
	s.NoError(err)

	reloadedJob, err := s.driver.Get(s.ctx, j.ID())
	s.Require().NoError(err)
	j = reloadedJob.(*job.ShellJob)
	s.NotEqual(originalCommand, j.Command)
	s.Equal(newCommand, j.Command)
}

func (s *DriverSuite) TestGetReturnsErrorIfJobDoesNotExist() {
	j, err := s.driver.Get(s.ctx, "does-not-exist")
	s.Error(err)
	s.Nil(j)
}

func (s *DriverSuite) TestStatsCallReportsCompletedJobs() {
	j := job.NewShellJob("echo foo", "")

	s.Equal(0, s.driver.Stats(s.ctx).Total)
	s.NoError(s.driver.Put(s.ctx, j))
	s.Equal(1, s.driver.Stats(s.ctx).Total)
	s.Equal(0, s.driver.Stats(s.ctx).Completed)
	s.Equal(1, s.driver.Stats(s.ctx).Pending)
	s.Equal(0, s.driver.Stats(s.ctx).Blocked)
	s.Equal(0, s.driver.Stats(s.ctx).Running)

	j.MarkComplete()
	s.NoError(s.driver.Save(s.ctx, j))
	s.Equal(1, s.driver.Stats(s.ctx).Total)
	s.Equal(1, s.driver.Stats(s.ctx).Completed)
	s.Equal(0, s.driver.Stats(s.ctx).Pending)
	s.Equal(0, s.driver.Stats(s.ctx).Blocked)
	s.Equal(0, s.driver.Stats(s.ctx).Running)
}

func (s *DriverSuite) TestStatsCountsAreAccurate() {
	const numEnqueued = 30
	for i := 0; i < numEnqueued; i++ {
		j := newMockJob()
		j.SetID(uuid.New().String())
		s.Require().NoError(s.driver.Put(s.ctx, j))
	}

	const numRunning = 50
	for i := 0; i < numRunning; i++ {
		j := newMockJob()
		j.SetID(uuid.New().String())
		j.SetStatus(amboy.JobStatusInfo{InProgress: true})
		s.Require().NoError(s.driver.Put(s.ctx, j))
	}

	const numCompleted = 10
	for i := 0; i < numCompleted; i++ {
		j := newMockJob()
		j.SetID(uuid.New().String())
		j.SetStatus(amboy.JobStatusInfo{Completed: true})
		s.Require().NoError(s.driver.Put(s.ctx, j))
	}

	const numRetrying = 5
	for i := 0; i < numRetrying; i++ {
		j := newMockRetryableJob(uuid.New().String())
		j.UpdateRetryInfo(amboy.JobRetryOptions{NeedsRetry: utility.TruePtr()})
		j.SetStatus(amboy.JobStatusInfo{Completed: true})
		s.Require().NoError(s.driver.Put(s.ctx, j))
	}

	stats := s.driver.Stats(s.ctx)
	s.Equal(numEnqueued, stats.Pending)
	s.Equal(numRunning, stats.Running)
	s.Equal(numCompleted+numRetrying, stats.Completed)
	s.Equal(numRetrying, stats.Retrying)
	s.Equal(numEnqueued+numRunning+numCompleted+numRetrying, stats.Total)
}

func (s *DriverSuite) TestNextMethodDoesNotReturnLastJob() {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	j := job.NewShellJob("echo foo", "")
	j.SetStatus(amboy.JobStatusInfo{
		InProgress: true,
	})
	s.Require().NoError(j.Lock("taken", amboy.LockTimeout))

	s.NoError(s.driver.Put(ctx, j))
	s.Equal(1, s.driver.Stats(ctx).Total)
	s.Equal(0, s.driver.Stats(ctx).Blocked)
	s.Equal(1, s.driver.Stats(ctx).Running)
	s.Equal(0, s.driver.Stats(ctx).Completed)

	s.Nil(s.driver.Next(ctx), fmt.Sprintf("%T", s.driver))
}

func (s *DriverSuite) TestJobsMethodReturnsAllJobs() {
	mocks := make(map[string]*job.ShellJob)

	for i := 0; i < 24; i++ {
		name := fmt.Sprintf("echo test num %d", i)
		j := job.NewShellJob(name, "")
		s.NoError(s.driver.Put(s.ctx, j))
		mocks[j.ID()] = j
	}

	counter := 0
	for j := range s.driver.Jobs(s.ctx) {
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

func (s *DriverSuite) TestRetryableJobsReturnsAllRetryableJobs() {
	rj := newMockRetryableJob("id")
	s.Require().NoError(s.driver.Put(s.ctx, rj))
	s.Require().NoError(s.driver.Put(s.ctx, newMockJob()))
	s.Require().NoError(s.driver.Put(s.ctx, job.NewShellJob("echo foo", "")))

	var found int
	for j := range s.driver.RetryableJobs(s.ctx, retryableJobAll) {
		found++
		s.Equal(rj.ID(), j.ID())
	}
	s.Equal(1, found)
}

func (s *DriverSuite) TestRetryableJobsStopsWithContextError() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var ids []string
	for i := 0; i < 100; i++ {
		j := newMockRetryableJob(fmt.Sprintf("id%d", i))
		s.Require().NoError(s.driver.Put(ctx, j))
		ids = append(ids, j.ID())
	}

	var found int
	for j := range s.driver.RetryableJobs(ctx, retryableJobAll) {
		cancel()
		found++
		s.Contains(ids, j.ID())
	}
	s.NotZero(found)
	s.True(found < len(ids))
}

func (s *DriverSuite) TestRetryableJobsReturnsAllRetryingJobs() {
	var expectedIDs []string

	j0 := newMockRetryableJob("id0")
	s.Require().NoError(s.driver.Put(s.ctx, j0))

	j1 := newMockRetryableJob("id1")
	j1.SetStatus(amboy.JobStatusInfo{
		Completed:         true,
		ModificationTime:  time.Now(),
		ModificationCount: 50,
	})
	j1.UpdateRetryInfo(amboy.JobRetryOptions{
		NeedsRetry: utility.TruePtr(),
	})
	expectedIDs = append(expectedIDs, j1.ID())
	s.Require().NoError(s.driver.Put(s.ctx, j1))

	j2 := newMockRetryableJob("id2")
	j2.SetStatus(amboy.JobStatusInfo{
		Completed: true,
	})
	j2.UpdateRetryInfo(amboy.JobRetryOptions{
		NeedsRetry: utility.TruePtr(),
	})
	expectedIDs = append(expectedIDs, j2.ID())
	s.Require().NoError(s.driver.Put(s.ctx, j2))

	j3 := newMockRetryableJob("id3")
	j3.UpdateRetryInfo(amboy.JobRetryOptions{
		NeedsRetry: utility.TruePtr(),
	})
	s.Require().NoError(s.driver.Put(s.ctx, j3))

	var foundIDs []string
	for j := range s.driver.RetryableJobs(s.ctx, retryableJobAllRetrying) {
		foundIDs = append(foundIDs, j.ID())
	}
	missingExpected, foundUnexpected := utility.StringSliceSymmetricDifference(expectedIDs, foundIDs)
	s.Empty(missingExpected, "missing expected IDs %s", missingExpected)
	s.Empty(foundUnexpected, "found unexpected IDs %s", foundUnexpected)
}

func (s *DriverSuite) TestRetryableJobsReturnsActiveRetryingJobs() {
	var expectedIDs []string

	j0 := newMockRetryableJob("id0")
	s.Require().NoError(s.driver.Put(s.ctx, j0))

	j1 := newMockRetryableJob("id1")
	j1.SetStatus(amboy.JobStatusInfo{
		Completed:         true,
		ModificationTime:  time.Now(),
		ModificationCount: 50,
	})
	j1.UpdateRetryInfo(amboy.JobRetryOptions{
		NeedsRetry: utility.TruePtr(),
	})
	expectedIDs = append(expectedIDs, j1.ID())
	s.Require().NoError(s.driver.Put(s.ctx, j1))

	j2 := newMockRetryableJob("id2")
	j2.SetStatus(amboy.JobStatusInfo{
		Completed: true,
	})
	j2.UpdateRetryInfo(amboy.JobRetryOptions{
		NeedsRetry: utility.TruePtr(),
	})
	s.Require().NoError(s.driver.Put(s.ctx, j2))

	j3 := newMockRetryableJob("id3")
	j3.UpdateRetryInfo(amboy.JobRetryOptions{
		NeedsRetry: utility.TruePtr(),
	})
	s.Require().NoError(s.driver.Put(s.ctx, j3))

	j4 := newMockRetryableJob("id4")
	j4.SetStatus(amboy.JobStatusInfo{
		Completed:        true,
		ModificationTime: time.Now().Add(time.Minute),
	})
	j4.UpdateRetryInfo(amboy.JobRetryOptions{
		NeedsRetry: utility.TruePtr(),
	})
	expectedIDs = append(expectedIDs, j4.ID())
	s.Require().NoError(s.driver.Put(s.ctx, j4))

	var foundIDs []string
	for j := range s.driver.RetryableJobs(s.ctx, retryableJobActiveRetrying) {
		foundIDs = append(foundIDs, j.ID())
	}
	missingExpected, foundUnexpected := utility.StringSliceSymmetricDifference(expectedIDs, foundIDs)
	s.Empty(missingExpected, "missing expected IDs %s", missingExpected)
	s.Empty(foundUnexpected, "found unexpected IDs %s", foundUnexpected)
}

func (s *DriverSuite) TestRetryableJobsReturnsStaleRetryingJobs() {
	var expectedIDs []string

	j0 := newMockRetryableJob("id0")
	s.Require().NoError(s.driver.Put(s.ctx, j0))

	j1 := newMockRetryableJob("id1")
	j1.SetStatus(amboy.JobStatusInfo{
		Completed:         true,
		ModificationTime:  time.Now(),
		ModificationCount: 50,
	})
	j1.UpdateRetryInfo(amboy.JobRetryOptions{
		NeedsRetry: utility.TruePtr(),
	})
	s.Require().NoError(s.driver.Put(s.ctx, j1))

	j2 := newMockRetryableJob("id2")
	j2.SetStatus(amboy.JobStatusInfo{
		Completed: true,
	})
	j2.UpdateRetryInfo(amboy.JobRetryOptions{
		NeedsRetry: utility.TruePtr(),
	})
	s.Require().NoError(s.driver.Put(s.ctx, j2))
	expectedIDs = append(expectedIDs, j2.ID())

	j3 := newMockRetryableJob("id3")
	j3.UpdateRetryInfo(amboy.JobRetryOptions{
		NeedsRetry: utility.TruePtr(),
	})
	s.Require().NoError(s.driver.Put(s.ctx, j3))

	j4 := newMockRetryableJob("id4")
	j4.SetStatus(amboy.JobStatusInfo{
		Completed:         true,
		ModificationTime:  time.Now().Add(-100 * s.driver.LockTimeout()),
		ModificationCount: 50,
	})
	j4.UpdateRetryInfo(amboy.JobRetryOptions{
		NeedsRetry: utility.TruePtr(),
	})
	expectedIDs = append(expectedIDs, j4.ID())
	s.Require().NoError(s.driver.Put(s.ctx, j4))

	var foundIDs []string
	for j := range s.driver.RetryableJobs(s.ctx, retryableJobStaleRetrying) {
		foundIDs = append(foundIDs, j.ID())
	}
	missingExpected, foundUnexpected := utility.StringSliceSymmetricDifference(expectedIDs, foundIDs)
	s.Empty(missingExpected, "missing expected IDs %s", missingExpected)
	s.Empty(foundUnexpected, "found unexpected IDs %s", foundUnexpected)
}

func (s *DriverSuite) TestJobInfoMethodReturnsAllJobs() {
	names := make(map[string]struct{})

	for i := 0; i < 30; i++ {
		cmd := fmt.Sprintf("echo 'foo: %d'", i)
		j := job.NewShellJob(cmd, "")

		s.NoError(s.driver.Put(s.ctx, j))
		names[j.ID()] = struct{}{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	counter := 0
	for info := range s.driver.JobInfo(ctx) {
		_, ok := names[info.ID]
		s.True(ok)
		counter++
	}
	s.Equal(len(names), counter)
	s.Equal(counter, 30)
}

func (s *DriverSuite) TestReturnsDefaultLockTimeout() {
	s.Equal(amboy.LockTimeout, s.driver.LockTimeout())
}

func (s *DriverSuite) TestInfoReturnsConfigurableLockTimeout() {
	opts := defaultMongoDBTestOptions()
	opts.LockTimeout = 25 * time.Minute
	d, err := newMongoDriver(s.T().Name(), opts)
	s.Require().NoError(err)
	s.Equal(opts.LockTimeout, d.LockTimeout())
}

func TestDriverDispatcherIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const size = 10
	mdbOpts := defaultMongoDBTestOptions()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mdbOpts.URI).SetConnectTimeout(time.Second))
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, client.Disconnect(ctx))
	}()

	t.Run("NextJobIsSampled", func(t *testing.T) {
		mdbOpts.SampleSize = size
		qName := t.Name()
		opts := MongoDBQueueCreationOptions{
			Name:   qName,
			Size:   size,
			MDB:    mdbOpts,
			Client: client,
		}
		opts.MDB.Format = amboy.BSON2
		q, err := NewMongoDBQueue(ctx, opts)
		require.NoError(t, err)
		defer func() {
			q.Close(ctx)
		}()
		rq, ok := q.(remoteQueue)
		require.True(t, ok, "MongoDB queue should be a remote queue")
		mq, err := newMockRemoteQueue(mockRemoteQueueOptions{
			queue:  rq,
			driver: rq.Driver(),
		})
		require.NoError(t, err)

		defer func() {
			assert.NoError(t, client.Database(opts.MDB.DB).Collection(addJobsSuffix(qName)).Drop(ctx))
		}()

		checkDispatched := func(t *testing.T, stat amboy.JobStatusInfo) {
			assert.True(t, stat.InProgress)
			assert.False(t, stat.Completed)
			assert.NotZero(t, stat.ModificationCount)
			assert.NotZero(t, stat.ModificationTime)
		}

		for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, driver *mongoDriver, dispatcher *mockDispatcher){
			"NextReturnsNoJobIfNoneExists": func(ctx context.Context, t *testing.T, driver *mongoDriver, dispatcher *mockDispatcher) {
				j := driver.Next(ctx)
				assert.Nil(t, j)
			},
			"NextDispatchesOnePendingJob": func(ctx context.Context, t *testing.T, driver *mongoDriver, dispatcher *mockDispatcher) {
				j := newMockJob()
				j.SetID("id")
				require.NoError(t, driver.Put(ctx, j))

				next := driver.Next(ctx)
				require.NotNil(t, next)
				assert.Equal(t, j.ID(), next.ID())

				checkDispatched(t, next.Status())
			},
			"NextDispatchesEachPendingJob": func(ctx context.Context, t *testing.T, driver *mongoDriver, dispatcher *mockDispatcher) {
				j0 := newMockJob()
				j0.SetID("id0")
				j1 := newMockJob()
				j0.SetID("id1")
				require.NoError(t, driver.Put(ctx, j0))
				require.NoError(t, driver.Put(ctx, j1))

				next0 := driver.Next(ctx)
				require.NotNil(t, next0)
				checkDispatched(t, next0.Status())

				switch next0.ID() {
				case j0.ID():
					next1 := driver.Next(ctx)
					require.NotNil(t, next1)
					assert.Equal(t, j1.ID(), next1.ID(), "the other job should be returned by the second call to Next")
					checkDispatched(t, next1.Status())
				case j1.ID():
					next1 := driver.Next(ctx)
					require.NotNil(t, next1)
					assert.Equal(t, j0.ID(), next1.ID(), "the other job should be returned by the second call to Next")
					checkDispatched(t, next1.Status())
				default:
					require.FailNow(t, "next job should be one of the enqueued jobs")
				}
			},
		} {
			t.Run(testName, func(t *testing.T) {
				tctx, tcancel := context.WithTimeout(ctx, time.Second)
				defer tcancel()
				mDriver, ok := mq.Driver().(*mongoDriver)
				require.True(t, ok, "driver must be a MongoDB driver")
				mDispatcher, ok := mq.dispatcher.(*mockDispatcher)
				require.True(t, ok)
				testCase(tctx, t, mDriver, mDispatcher)
			})
		}
	})
}
