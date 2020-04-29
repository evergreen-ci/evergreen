package management

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ManagerSuite struct {
	queue   amboy.Queue
	manager Manager
	ctx     context.Context
	cancel  context.CancelFunc

	factory func() Manager
	setup   func()
	cleanup func() error
	suite.Suite
}

func TestManagerSuiteBackedByMongoDB(t *testing.T) {
	s := new(ManagerSuite)
	name := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := queue.DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
	require.NoError(t, err)
	s.factory = func() Manager {
		manager, err := MakeDBQueueManager(ctx, DBQueueManagerOptions{
			Options: opts,
			Name:    name,
		}, client)
		require.NoError(t, err)
		return manager
	}

	s.setup = func() {
		s.Require().NoError(client.Database(opts.DB).Drop(ctx))
		args := queue.MongoDBQueueCreationOptions{
			Size:   2,
			Name:   name,
			MDB:    opts,
			Client: client,
		}

		remote, err := queue.NewMongoDBQueue(ctx, args)
		require.NoError(t, err)
		s.queue = remote
	}

	s.cleanup = func() error {
		require.NoError(t, client.Disconnect(ctx))
		s.queue.Runner().Close(ctx)
		return nil
	}

	suite.Run(t, s)
}

func TestManagerSuiteBackedByMongoDBSingleGroup(t *testing.T) {
	s := new(ManagerSuite)
	name := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := queue.DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
	require.NoError(t, err)
	s.factory = func() Manager {
		manager, err := MakeDBQueueManager(ctx, DBQueueManagerOptions{
			Options:     opts,
			Name:        name,
			Group:       "foo",
			SingleGroup: true,
		}, client)
		require.NoError(t, err)
		return manager
	}

	opts.UseGroups = true
	opts.GroupName = "foo"

	s.setup = func() {
		s.Require().NoError(client.Database(opts.DB).Drop(ctx))
		args := queue.MongoDBQueueCreationOptions{
			Size:   2,
			Name:   name,
			MDB:    opts,
			Client: client,
		}

		remote, err := queue.NewMongoDBQueue(ctx, args)
		require.NoError(t, err)
		s.queue = remote
	}

	s.cleanup = func() error {
		require.NoError(t, client.Disconnect(ctx))
		s.queue.Runner().Close(ctx)
		return nil
	}

	suite.Run(t, s)
}

func TestManagerSuiteBackedByMongoDBMultiGroup(t *testing.T) {
	s := new(ManagerSuite)
	name := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := queue.DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
	require.NoError(t, err)
	s.factory = func() Manager {
		manager, err := MakeDBQueueManager(ctx, DBQueueManagerOptions{
			Options:  opts,
			Name:     name,
			Group:    "foo",
			ByGroups: true,
		}, client)
		require.NoError(t, err)
		return manager
	}

	opts.UseGroups = true
	opts.GroupName = "foo"

	s.setup = func() {
		s.Require().NoError(client.Database(opts.DB).Drop(ctx))
		args := queue.MongoDBQueueCreationOptions{
			Size:   2,
			Name:   name,
			MDB:    opts,
			Client: client,
		}

		remote, err := queue.NewMongoDBQueue(ctx, args)
		require.NoError(t, err)
		s.queue = remote
	}

	s.cleanup = func() error {
		require.NoError(t, client.Disconnect(ctx))
		s.queue.Runner().Close(ctx)
		return nil
	}

	suite.Run(t, s)
}

func TestManagerSuiteBackedByQueueMethods(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := new(ManagerSuite)
	s.setup = func() {
		s.queue = queue.NewLocalLimitedSize(2, 128)
		s.Require().NoError(s.queue.Start(ctx))
	}

	s.factory = func() Manager {
		return NewQueueManager(s.queue)
	}

	s.cleanup = func() error {
		return nil
	}
	suite.Run(t, s)
}

func (s *ManagerSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.setup()
	s.manager = s.factory()
}

func (s *ManagerSuite) TearDownTest() {
	s.cancel()
}

func (s *ManagerSuite) TearDownSuite() {
	s.NoError(s.cleanup())
}

func (s *ManagerSuite) TestJobStatusInvalidFilter() {
	for _, f := range []string{"", "foo", "inprog"} {
		r, err := s.manager.JobStatus(s.ctx, StatusFilter(f))
		s.Error(err)
		s.Nil(r)

		rr, err := s.manager.JobIDsByState(s.ctx, "foo", StatusFilter(f))
		s.Error(err)
		s.Nil(rr)
	}
}

func (s *ManagerSuite) TestTimingWithInvalidFilter() {
	for _, f := range []string{"", "foo", "inprog"} {
		r, err := s.manager.RecentTiming(s.ctx, time.Hour, RuntimeFilter(f))
		s.Error(err)
		s.Nil(r)
	}
}

func (s *ManagerSuite) TestErrorsWithInvalidFilter() {
	for _, f := range []string{"", "foo", "inprog"} {
		r, err := s.manager.RecentJobErrors(s.ctx, "foo", time.Hour, ErrorFilter(f))
		s.Error(err)
		s.Nil(r)

		r, err = s.manager.RecentErrors(s.ctx, time.Hour, ErrorFilter(f))
		s.Error(err)
		s.Nil(r)
	}
}

func (s *ManagerSuite) TestJobCounterHighLevel() {
	for _, f := range []StatusFilter{InProgress, Pending, Stale} {
		r, err := s.manager.JobStatus(s.ctx, f)
		s.NoError(err)
		s.NotNil(r)
	}

}

func (s *ManagerSuite) TestJobCountingIDHighLevel() {
	for _, f := range []StatusFilter{InProgress, Pending, Stale, Completed} {
		r, err := s.manager.JobIDsByState(s.ctx, "foo", f)
		s.NoError(err, "%s", f)
		s.NotNil(r)
	}
}

func (s *ManagerSuite) TestJobTimingMustBeLongerThanASecond() {
	for _, dur := range []time.Duration{-1, 0, time.Millisecond, -time.Hour} {
		r, err := s.manager.RecentTiming(s.ctx, dur, Duration)
		s.Error(err)
		s.Nil(r)
		je, err := s.manager.RecentJobErrors(s.ctx, "foo", dur, StatsOnly)
		s.Error(err)
		s.Nil(je)

		je, err = s.manager.RecentErrors(s.ctx, dur, StatsOnly)
		s.Error(err)
		s.Nil(je)

	}
}

func (s *ManagerSuite) TestJobTiming() {
	for _, f := range []RuntimeFilter{Duration, Latency, Running} {
		r, err := s.manager.RecentTiming(s.ctx, time.Minute, f)
		s.NoError(err)
		s.NotNil(r)
	}
}

func (s *ManagerSuite) TestRecentErrors() {
	for _, f := range []ErrorFilter{UniqueErrors, AllErrors, StatsOnly} {
		r, err := s.manager.RecentErrors(s.ctx, time.Minute, f)
		s.NoError(err)
		s.NotNil(r)
	}
}

func (s *ManagerSuite) TestRecentJobErrors() {
	for _, f := range []ErrorFilter{UniqueErrors, AllErrors, StatsOnly} {
		r, err := s.manager.RecentJobErrors(s.ctx, "shell", time.Minute, f)
		s.NoError(err)
		s.NotNil(r)
	}
}

func (s *ManagerSuite) TestCompleteJob() {
	j1 := job.NewShellJob("ls", "")
	s.Require().NoError(s.queue.Put(s.ctx, j1))
	j2 := newTestJob("complete")
	s.Require().NoError(s.queue.Put(s.ctx, j2))
	j3 := newTestJob("uncomplete")
	s.Require().NoError(s.queue.Put(s.ctx, j3))

	s.Require().NoError(s.manager.CompleteJob(s.ctx, "complete"))
	jobCount := 0
	for jobStats := range s.queue.JobStats(s.ctx) {
		if jobStats.ID == "complete" {
			s.True(jobStats.Completed)
			_, ok := s.manager.(*dbQueueManager)
			if ok {
				s.Equal(3, jobStats.ModificationCount)
			}
		} else {
			s.False(jobStats.Completed)
			s.Equal(0, jobStats.ModificationCount)
		}
		jobCount++
	}
	s.Equal(3, jobCount)
}

func (s *ManagerSuite) TestCompleteJobsInvalidFilter() {
	s.Error(s.manager.CompleteJobs(s.ctx, "invalid"))
	s.Error(s.manager.CompleteJobs(s.ctx, Completed))
}

func (s *ManagerSuite) TestCompleteJobsValidFilter() {
	j1 := job.NewShellJob("ls", "")
	s.Require().NoError(s.queue.Put(s.ctx, j1))
	j2 := newTestJob("0")
	s.Require().NoError(s.queue.Put(s.ctx, j2))
	j3 := newTestJob("1")
	s.Require().NoError(s.queue.Put(s.ctx, j3))

	s.Require().NoError(s.manager.CompleteJobs(s.ctx, Pending))
	jobCount := 0
	for jobStats := range s.queue.JobStats(s.ctx) {
		s.True(jobStats.Completed)
		_, ok := s.manager.(*dbQueueManager)
		if ok {
			s.Equal(3, jobStats.ModificationCount)
		}
		jobCount++
	}
	s.Equal(3, jobCount)
}

func (s *ManagerSuite) TestCompleteJobsByTypeInvalidFilter() {
	s.Error(s.manager.CompleteJobsByType(s.ctx, "invalid", "type"))
	s.Error(s.manager.CompleteJobsByType(s.ctx, Completed, "type"))
}

func (s *ManagerSuite) TestCompleteJobsByTypeValidFilter() {
	j1 := job.NewShellJob("ls", "")
	s.Require().NoError(s.queue.Put(s.ctx, j1))
	j2 := newTestJob("0")
	s.Require().NoError(s.queue.Put(s.ctx, j2))
	j3 := newTestJob("1")
	s.Require().NoError(s.queue.Put(s.ctx, j3))

	s.Require().NoError(s.manager.CompleteJobsByType(s.ctx, Pending, "test"))
	jobCount := 0
	for jobStats := range s.queue.JobStats(s.ctx) {
		if jobStats.ID == "0" || jobStats.ID == "1" {
			s.True(jobStats.Completed)
			_, ok := s.manager.(*dbQueueManager)
			if ok {
				s.Equal(3, jobStats.ModificationCount)
			}
		} else {
			s.False(jobStats.Completed)
			s.Equal(0, jobStats.ModificationCount)
		}
		jobCount++
	}
	s.Equal(3, jobCount)
}

type testJob struct {
	job.Base
}

func newTestJob(id string) *testJob {
	j := &testJob{
		Base: job.Base{
			TaskID:  id,
			JobType: amboy.JobType{Name: "test"},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

func (j *testJob) Run(ctx context.Context) {
	time.Sleep(time.Minute)
	return
}
