package queue

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type OrderedQueueSuite struct {
	size     int
	queue    amboy.Queue
	setup    func()
	tearDown func()
	reset    func()
	suite.Suite
}

func TestLocalOrderedQueueSuiteOneWorker(t *testing.T) {
	s := &OrderedQueueSuite{}
	s.size = 1
	s.setup = func() { s.queue = NewLocalOrdered(s.size) }

	suite.Run(t, s)
}

func TestLocalOrderedQueueSuiteThreeWorker(t *testing.T) {
	s := &OrderedQueueSuite{}
	s.size = 3
	s.setup = func() { s.queue = NewLocalOrdered(s.size) }
	suite.Run(t, s)
}

func TestRemoteMongoDBOrderedQueueSuiteFourWorkers(t *testing.T) {
	s := &OrderedQueueSuite{}
	name := "test-" + uuid.NewV4().String()
	uri := "mongodb://localhost"
	ctx, cancel := context.WithCancel(context.Background())

	session, err := mgo.Dial(uri)
	if err != nil || session == nil {
		t.Fatal("problem configuring connection to:", uri)
	}
	defer session.Close()

	s.size = 4

	s.setup = func() {
		remote := NewSimpleRemoteOrdered(s.size).(*remoteSimpleOrdered)
		d := NewMongoDBDriver(name, DefaultMongoDBOptions())
		s.Require().NoError(d.Open(ctx))
		s.Require().NoError(remote.SetDriver(d))
		s.queue = remote
		s.Require().NotNil(remote.remoteBase)
	}

	s.tearDown = func() {
		cancel()

		grip.CatchError(session.DB("amboy").C(name + ".jobs").DropCollection())
		grip.CatchError(session.DB("amboy").C(name + ".locks").DropCollection())
	}

	s.reset = func() {
		_, err = session.DB("amboy").C(name + ".jobs").RemoveAll(bson.M{})
		grip.CatchError(err)
		_, err = session.DB("amboy").C(name + ".locks").RemoveAll(bson.M{})
		grip.CatchError(err)
	}

	suite.Run(t, s)
}

func TestRemoteInternalOrderedQueueSuite(t *testing.T) {
	s := &OrderedQueueSuite{}
	ctx, cancel := context.WithCancel(context.Background())

	s.size = 4

	s.reset = func() {
		d := NewPriorityDriver()
		remote := NewSimpleRemoteOrdered(s.size).(*remoteSimpleOrdered)
		s.Require().NotNil(remote.remoteBase)
		s.Require().NoError(d.Open(ctx))
		s.Require().NoError(remote.SetDriver(d))
		s.queue = remote
	}

	s.setup = func() {
		s.reset()
	}

	s.tearDown = func() {
		cancel()
	}

	suite.Run(t, s)
}

func TestRemotePriorityOrderedQueueSuite(t *testing.T) {
	s := &OrderedQueueSuite{}
	ctx, cancel := context.WithCancel(context.Background())

	s.size = 4

	s.reset = func() {
		d := NewInternalDriver()
		remote := NewSimpleRemoteOrdered(s.size).(*remoteSimpleOrdered)
		s.Require().NotNil(remote.remoteBase)
		s.Require().NoError(d.Open(ctx))
		s.Require().NoError(remote.SetDriver(d))
		s.queue = remote
	}

	s.setup = func() {
		s.reset()
	}

	s.tearDown = func() {
		cancel()
	}

	suite.Run(t, s)
}

func (s *OrderedQueueSuite) SetupTest() {
	if s.setup != nil {
		s.setup()
	}
}

func (s *OrderedQueueSuite) TearDownTest() {
	if s.reset != nil {
		s.reset()
	}
}

func (s *OrderedQueueSuite) TearDownSuite() {
	if s.tearDown != nil {
		s.tearDown()
	}
}

func (s *OrderedQueueSuite) TestPutReturnsErrorForDuplicateNameTasks() {
	j := job.NewShellJob("true", "")

	s.Equal(0, s.queue.Stats().Total)
	s.NoError(s.queue.Put(j))
	s.Equal(1, s.queue.Stats().Total)
	s.Error(s.queue.Put(j))
	s.Equal(1, s.queue.Stats().Total)

}

func (s *OrderedQueueSuite) TestPuttingAJobIntoAQueueImpactsStats() {
	stats := s.queue.Stats()
	s.Equal(0, stats.Total)
	s.Equal(0, stats.Pending)
	s.Equal(0, stats.Running)
	s.Equal(0, stats.Completed)

	j := job.NewShellJob("true", "")
	s.NoError(s.queue.Put(j))

	jReturn, ok := s.queue.Get(j.ID())
	s.True(ok)

	jActual := jReturn.(*job.ShellJob)

	j.Base.SetDependency(jActual.Dependency())

	stats = s.queue.Stats()
	s.Equal(1, stats.Total)
	s.Equal(1, stats.Pending)
	s.Equal(0, stats.Running)
	s.Equal(0, stats.Completed)
}

func (s *OrderedQueueSuite) TestInternalRunnerCanBeChangedBeforeStartingTheQueue() {
	newRunner := pool.NewLocalWorkers(2, s.queue)
	originalRunner := s.queue.Runner()
	s.NotEqual(originalRunner, newRunner)

	s.NoError(s.queue.SetRunner(newRunner))
	s.Exactly(newRunner, s.queue.Runner())
}

func (s *OrderedQueueSuite) TestInternalRunnerCannotBeChangedAfterStartingAQueue() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runner := s.queue.Runner()
	s.False(s.queue.Started())
	s.NoError(s.queue.Start(ctx))
	s.True(s.queue.Started())

	newRunner := pool.NewLocalWorkers(2, s.queue)
	s.Error(s.queue.SetRunner(newRunner))
	s.NotEqual(runner, newRunner)
}

func (s *OrderedQueueSuite) TestResultsChannelProducesPointersToConsistentJobObjects() {
	j := job.NewShellJob("echo true", "")
	s.False(j.Status().Completed)

	s.NoError(s.queue.Put(j))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.NoError(s.queue.Start(ctx))

	grip.Critical(s.queue.Stats())
	amboy.WaitCtxInterval(ctx, s.queue, 250*time.Millisecond)
	grip.Critical(s.queue.Stats())

	result, ok := <-s.queue.Results(ctx)
	if s.True(ok, "%+v", s.queue.Stats()) {
		s.Equal(j.ID(), result.ID())
		s.True(result.Status().Completed)
	}
}

func (s *OrderedQueueSuite) TestQueueCanOnlyBeStartedOnce() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.False(s.queue.Started())
	s.NoError(s.queue.Start(ctx))
	s.True(s.queue.Started())

	amboy.Wait(s.queue)
	s.True(s.queue.Started())

	// you can call start more than once until the queue has
	// completed
	s.NoError(s.queue.Start(ctx))
	s.True(s.queue.Started())
}

func (s *OrderedQueueSuite) TestPassedIsCompletedButDoesNotRun() {
	cwd := GetDirectoryOfFile()

	j1 := job.NewShellJob("echo foo", "")
	j2 := job.NewShellJob("echo true", "")
	j1.SetDependency(dependency.NewCreatesFile(filepath.Join(cwd, "ordered_test.go")))
	s.NoError(j1.Dependency().AddEdge(j2.ID()))

	s.Equal(j1.Dependency().State(), dependency.Passed)
	s.Equal(j2.Dependency().State(), dependency.Ready)

	s.False(j1.Status().Completed)
	s.False(j2.Status().Completed)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.NoError(s.queue.Put(j2))
	s.NoError(s.queue.Put(j1))

	s.NoError(s.queue.Start(ctx))

	grip.Critical(s.queue.Stats())
	amboy.WaitCtxInterval(ctx, s.queue, 250*time.Millisecond)
	grip.Critical(s.queue.Stats())

	j1Refreshed, ok1 := s.queue.Get(j1.ID())
	j2Refreshed, ok2 := s.queue.Get(j2.ID())
	if s.True(ok1) {
		s.False(j1Refreshed.Status().Completed)
	}
	if s.True(ok2) {
		s.True(j2Refreshed.Status().Completed, "%+v", j2Refreshed.Status())
	}
}

////////////////////////////////////////////////////////////////////////
//
// The following tests are specific to the local implementation of the ordered queue
//
////////////////////////////////////////////////////////////////////////

type LocalOrderedSuite struct {
	queue *depGraphOrderedLocal
	suite.Suite
}

func TestLocalOrderedSuite(t *testing.T) {
	suite.Run(t, new(LocalOrderedSuite))
}

func (s *LocalOrderedSuite) SetupTest() {
	s.queue = NewLocalOrdered(2).(*depGraphOrderedLocal)
}

func (s *LocalOrderedSuite) TestLocalQueueFailsToStartIfGraphIsOutOfSync() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// this shouldn't be possible, but if there's a bug in Put,
	// there's some internal structures that might get out of
	// sync, so a test seems in order.

	// first simulate a put bug
	j := job.NewShellJob("true", "")

	s.NoError(s.queue.Put(j))

	jtwo := job.NewShellJob("echo foo", "")
	s.queue.tasks.m["foo"] = jtwo

	s.Error(s.queue.Start(ctx))
}

func (s *LocalOrderedSuite) TestQueueFailsToStartIfDependencyDoesNotExist() {
	// this shouldn't be possible, but if the tasks and graph
	// mappings get out of sync, then there's an error on start.

	j1 := job.NewShellJob("true", "")
	j2 := job.NewShellJob("true", "")
	s.NoError(j2.Dependency().AddEdge(j1.ID()))
	s.NoError(j2.Dependency().AddEdge("fooo"))

	s.Len(j1.Dependency().Edges(), 0)
	s.Len(j2.Dependency().Edges(), 2)

	s.NoError(s.queue.Put(j1))
	s.NoError(s.queue.Put(j2))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Error(s.queue.Start(ctx))
}

func (s *LocalOrderedSuite) TestQueueFailsToStartIfTaskGraphIsCyclic() {
	j1 := job.NewShellJob("true", "")
	j2 := job.NewShellJob("true", "")

	s.NoError(j1.Dependency().AddEdge(j2.ID()))
	s.NoError(j2.Dependency().AddEdge(j1.ID()))

	s.Len(j1.Dependency().Edges(), 1)
	s.Len(j2.Dependency().Edges(), 1)

	s.NoError(s.queue.Put(j1))
	s.NoError(s.queue.Put(j2))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Error(s.queue.Start(ctx))
}

func (s *LocalOrderedSuite) TestPuttingJobIntoQueueAfterStartingReturnsError() {
	j := job.NewShellJob("true", "")
	s.NoError(s.queue.Put(j))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(s.queue.Start(ctx))
	s.Error(s.queue.Put(j))
}

func GetDirectoryOfFile() string {
	_, file, _, _ := runtime.Caller(1)

	return filepath.Dir(file)
}
