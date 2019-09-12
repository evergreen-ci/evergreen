package reporting

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ReportingSuite struct {
	queue    amboy.Queue
	reporter Reporter
	ctx      context.Context
	cancel   context.CancelFunc

	factory func() Reporter
	setup   func()
	cleanup func() error
	suite.Suite
}

func TestReportingSuiteBackedByLegacyMongoDB(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := new(ReportingSuite)
	name := uuid.NewV4().String()
	opts := queue.DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(time.Second))
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer func() { client.Disconnect(ctx) }()

	s.factory = func() Reporter {
		conf := DBQueueReporterOptions{Name: name, Options: opts}

		reporter, err := MakeDBQueueState(ctx, conf, client)
		s.Require().NoError(err)
		return reporter
	}

	s.setup = func() {
		remote := queue.NewRemoteUnordered(2)
		driver := queue.NewMongoDriver(name, opts)
		s.NoError(remote.SetDriver(driver))
		s.queue = remote
	}

	s.cleanup = func() error {
		s.queue.Runner().Close(ctx)
		return nil
	}

	suite.Run(t, s)
}

func TestReportingSuiteBackedByMongoDB(t *testing.T) {
	s := new(ReportingSuite)
	name := uuid.NewV4().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := queue.DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
	require.NoError(t, err)
	s.factory = func() Reporter {
		reporter, err := MakeDBQueueState(ctx, DBQueueReporterOptions{
			Options: opts,
			Name:    name,
		}, client)
		require.NoError(t, err)
		return reporter
	}

	s.setup = func() {
		remote := queue.NewRemoteUnordered(2)
		driver, err := queue.OpenNewMongoDriver(ctx, name, opts, client)
		require.NoError(t, err)
		require.NoError(t, remote.SetDriver(driver))
		s.queue = remote
	}

	s.cleanup = func() error {
		require.NoError(t, client.Disconnect(ctx))
		s.queue.Runner().Close(ctx)
		return nil
	}

	suite.Run(t, s)
}

func TestReportingSuiteBackedByMongoDBSingleGroup(t *testing.T) {
	s := new(ReportingSuite)
	name := uuid.NewV4().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := queue.DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
	require.NoError(t, err)
	s.factory = func() Reporter {
		reporter, err := MakeDBQueueState(ctx, DBQueueReporterOptions{
			Options:     opts,
			Name:        name,
			Group:       "foo",
			SingleGroup: true,
		}, client)
		require.NoError(t, err)
		return reporter
	}

	s.setup = func() {
		remote := queue.NewRemoteUnordered(2)
		driver, err := queue.OpenNewMongoGroupDriver(ctx, name, opts, "foo", client)
		require.NoError(t, err)
		require.NoError(t, remote.SetDriver(driver))
		s.queue = remote
	}

	s.cleanup = func() error {
		require.NoError(t, client.Disconnect(ctx))
		s.queue.Runner().Close(ctx)
		return nil
	}

	suite.Run(t, s)
}

func TestReportingSuiteBackedByMongoDBMultiGroup(t *testing.T) {
	s := new(ReportingSuite)
	name := uuid.NewV4().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := queue.DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
	require.NoError(t, err)
	s.factory = func() Reporter {
		reporter, err := MakeDBQueueState(ctx, DBQueueReporterOptions{
			Options:  opts,
			Name:     name,
			ByGroups: true,
		}, client)
		require.NoError(t, err)
		return reporter
	}

	s.setup = func() {
		remote := queue.NewRemoteUnordered(2)
		driver, err := queue.OpenNewMongoGroupDriver(ctx, name, opts, "foo", client)
		require.NoError(t, err)
		require.NoError(t, remote.SetDriver(driver))
		s.queue = remote
	}

	s.cleanup = func() error {
		require.NoError(t, client.Disconnect(ctx))
		s.queue.Runner().Close(ctx)
		return nil
	}

	suite.Run(t, s)
}

func TestReportingSuiteBackedByQueueMethods(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := new(ReportingSuite)
	s.setup = func() {
		s.queue = queue.NewLocalLimitedSize(2, 128)
		s.Require().NoError(s.queue.Start(ctx))
	}

	s.factory = func() Reporter {
		return NewQueueReporter(s.queue)
	}

	s.cleanup = func() error {
		return nil
	}
	suite.Run(t, s)
}

func (s *ReportingSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.setup()
	s.reporter = s.factory()
}

func (s *ReportingSuite) TearDownTest() {
	s.cancel()
}

func (s *ReportingSuite) TearDownSuite() {
	s.NoError(s.cleanup())
}

func (s *ReportingSuite) TestJobStatusInvalidFilter() {
	for _, f := range []string{"", "foo", "inprog"} {
		r, err := s.reporter.JobStatus(s.ctx, CounterFilter(f))
		s.Error(err)
		s.Nil(r)

		rr, err := s.reporter.JobIDsByState(s.ctx, "foo", CounterFilter(f))
		s.Error(err)
		s.Nil(rr)
	}
}

func (s *ReportingSuite) TestTimingWithInvalidFilter() {
	for _, f := range []string{"", "foo", "inprog"} {
		r, err := s.reporter.RecentTiming(s.ctx, time.Hour, RuntimeFilter(f))
		s.Error(err)
		s.Nil(r)
	}
}

func (s *ReportingSuite) TestErrorsWithInvalidFilter() {
	for _, f := range []string{"", "foo", "inprog"} {
		r, err := s.reporter.RecentJobErrors(s.ctx, "foo", time.Hour, ErrorFilter(f))
		s.Error(err)
		s.Nil(r)

		r, err = s.reporter.RecentErrors(s.ctx, time.Hour, ErrorFilter(f))
		s.Error(err)
		s.Nil(r)
	}
}

func (s *ReportingSuite) TestJobCounterHighLevel() {
	for _, f := range []CounterFilter{InProgress, Pending, Stale} {
		r, err := s.reporter.JobStatus(s.ctx, f)
		s.NoError(err)
		s.NotNil(r)
	}

}

func (s *ReportingSuite) TestJobCountingIDHighLevel() {
	for _, f := range []CounterFilter{InProgress, Pending, Stale} {
		r, err := s.reporter.JobIDsByState(s.ctx, "foo", f)
		s.NoError(err)
		s.NotNil(r)
	}
}

func (s *ReportingSuite) TestJobTimingMustBeLongerThanASecond() {
	for _, dur := range []time.Duration{-1, 0, time.Millisecond, -time.Hour} {
		r, err := s.reporter.RecentTiming(s.ctx, dur, Duration)
		s.Error(err)
		s.Nil(r)
		je, err := s.reporter.RecentJobErrors(s.ctx, "foo", dur, StatsOnly)
		s.Error(err)
		s.Nil(je)

		je, err = s.reporter.RecentErrors(s.ctx, dur, StatsOnly)
		s.Error(err)
		s.Nil(je)

	}
}

func (s *ReportingSuite) TestJobTiming() {
	for _, f := range []RuntimeFilter{Duration, Latency, Running} {
		r, err := s.reporter.RecentTiming(s.ctx, time.Minute, f)
		s.NoError(err)
		s.NotNil(r)
	}
}

func (s *ReportingSuite) TestRecentErrors() {
	for _, f := range []ErrorFilter{UniqueErrors, AllErrors, StatsOnly} {
		r, err := s.reporter.RecentErrors(s.ctx, time.Minute, f)
		s.NoError(err)
		s.NotNil(r)
	}
}

func (s *ReportingSuite) TestRecentJobErrors() {
	for _, f := range []ErrorFilter{UniqueErrors, AllErrors, StatsOnly} {
		r, err := s.reporter.RecentJobErrors(s.ctx, "shell", time.Minute, f)
		s.NoError(err)
		s.NotNil(r)
	}
}
