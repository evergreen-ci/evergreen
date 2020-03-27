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

type ManagementSuite struct {
	queue   amboy.Queue
	manager Management
	ctx     context.Context
	cancel  context.CancelFunc

	factory func() Management
	setup   func()
	cleanup func() error
	suite.Suite
}

func TestManagementSuiteBackedByMongoDB(t *testing.T) {
	s := new(ManagementSuite)
	name := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := queue.DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
	require.NoError(t, err)
	s.factory = func() Management {
		manager, err := MakeDBQueueManager(ctx, DBQueueManagerOptions{
			Options: opts,
			Name:    name,
		}, client)
		require.NoError(t, err)
		return manager
	}

	s.setup = func() {
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

func TestManagementSuiteBackedByMongoDBSingleGroup(t *testing.T) {
	s := new(ManagementSuite)
	name := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := queue.DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
	require.NoError(t, err)
	s.factory = func() Management {
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

func TestManagementSuiteBackedByMongoDBMultiGroup(t *testing.T) {
	s := new(ManagementSuite)
	name := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := queue.DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
	require.NoError(t, err)
	s.factory = func() Management {
		manager, err := MakeDBQueueManager(ctx, DBQueueManagerOptions{
			Options:  opts,
			Name:     name,
			ByGroups: true,
		}, client)
		require.NoError(t, err)
		return manager
	}

	opts.UseGroups = true
	opts.GroupName = "foo"

	s.setup = func() {
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

func TestManagementSuiteBackedByQueueMethods(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := new(ManagementSuite)
	s.setup = func() {
		s.queue = queue.NewLocalLimitedSize(2, 128)
		s.Require().NoError(s.queue.Start(ctx))
	}

	s.factory = func() Management {
		return NewQueueManager(s.queue)
	}

	s.cleanup = func() error {
		return nil
	}
	suite.Run(t, s)
}

func (s *ManagementSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.setup()
	s.manager = s.factory()
}

func (s *ManagementSuite) TearDownTest() {
	s.cancel()
}

func (s *ManagementSuite) TearDownSuite() {
	s.NoError(s.cleanup())
}

func (s *ManagementSuite) TestJobStatusInvalidFilter() {
	for _, f := range []string{"", "foo", "inprog"} {
		r, err := s.manager.JobStatus(s.ctx, CounterFilter(f))
		s.Error(err)
		s.Nil(r)

		rr, err := s.manager.JobIDsByState(s.ctx, "foo", CounterFilter(f))
		s.Error(err)
		s.Nil(rr)
	}
}

func (s *ManagementSuite) TestTimingWithInvalidFilter() {
	for _, f := range []string{"", "foo", "inprog"} {
		r, err := s.manager.RecentTiming(s.ctx, time.Hour, RuntimeFilter(f))
		s.Error(err)
		s.Nil(r)
	}
}

func (s *ManagementSuite) TestErrorsWithInvalidFilter() {
	for _, f := range []string{"", "foo", "inprog"} {
		r, err := s.manager.RecentJobErrors(s.ctx, "foo", time.Hour, ErrorFilter(f))
		s.Error(err)
		s.Nil(r)

		r, err = s.manager.RecentErrors(s.ctx, time.Hour, ErrorFilter(f))
		s.Error(err)
		s.Nil(r)
	}
}

func (s *ManagementSuite) TestJobCounterHighLevel() {
	for _, f := range []CounterFilter{InProgress, Pending, Stale} {
		r, err := s.manager.JobStatus(s.ctx, f)
		s.NoError(err)
		s.NotNil(r)
	}

}

func (s *ManagementSuite) TestJobCountingIDHighLevel() {
	for _, f := range []CounterFilter{InProgress, Pending, Stale} {
		r, err := s.manager.JobIDsByState(s.ctx, "foo", f)
		s.NoError(err)
		s.NotNil(r)
	}
}

func (s *ManagementSuite) TestJobTimingMustBeLongerThanASecond() {
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

func (s *ManagementSuite) TestJobTiming() {
	for _, f := range []RuntimeFilter{Duration, Latency, Running} {
		r, err := s.manager.RecentTiming(s.ctx, time.Minute, f)
		s.NoError(err)
		s.NotNil(r)
	}
}

func (s *ManagementSuite) TestRecentErrors() {
	for _, f := range []ErrorFilter{UniqueErrors, AllErrors, StatsOnly} {
		r, err := s.manager.RecentErrors(s.ctx, time.Minute, f)
		s.NoError(err)
		s.NotNil(r)
	}
}

func (s *ManagementSuite) TestRecentJobErrors() {
	for _, f := range []ErrorFilter{UniqueErrors, AllErrors, StatsOnly} {
		r, err := s.manager.RecentJobErrors(s.ctx, "shell", time.Minute, f)
		s.NoError(err)
		s.NotNil(r)
	}
}

func (s *ManagementSuite) TestCompleteJobsByType() {
	j1 := job.NewShellJob("ls", "")
	s.Require().NoError(s.queue.Put(s.ctx, j1))
	j2 := newTestJob("0")
	s.Require().NoError(s.queue.Put(s.ctx, j2))
	j3 := newTestJob("1")
	s.Require().NoError(s.queue.Put(s.ctx, j3))

	s.Require().NoError(s.manager.CompleteJobsByType(s.ctx, "test"))
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
	}
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
