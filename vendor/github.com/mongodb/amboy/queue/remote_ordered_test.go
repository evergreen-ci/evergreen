package queue

import (
	"fmt"
	"testing"
	"time"

	mgo "gopkg.in/mgo.v2"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/queue/driver"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

func init() {
	registry.AddDependencyType("mock", func() dependency.Manager { return dependency.NewMock() })
}

type SimpleRemoteOrderedSuite struct {
	queue             amboy.Queue
	tearDown          func() error
	driver            driver.Driver
	driverConstructor func() driver.Driver
	canceler          context.CancelFunc
	suite.Suite
}

func TestSimpleRemoteOrderedSuiteMongoDB(t *testing.T) {
	suite.Run(t, new(SimpleRemoteOrderedSuite))
}

func (s *SimpleRemoteOrderedSuite) SetupSuite() {
	name := "test-" + uuid.NewV4().String()
	uri := "mongodb://localhost"
	s.driverConstructor = func() driver.Driver {
		return driver.NewMongoDB(name, driver.DefaultMongoDBOptions())
	}

	s.tearDown = func() error {
		session, err := mgo.Dial(uri)
		defer session.Close()

		if err != nil {
			return err
		}

		return session.DB("amboy").C(name + ".jobs").DropCollection()
	}
}

func (s *SimpleRemoteOrderedSuite) SetupTest() {
	ctx, canceler := context.WithCancel(context.Background())
	s.driver = s.driverConstructor()
	s.canceler = canceler
	s.NoError(s.driver.Open(ctx))
	queue := NewSimpleRemoteOrdered(2)
	s.NoError(queue.SetDriver(s.driver))
	s.queue = queue
}

func (s *SimpleRemoteOrderedSuite) TearDownTest() {
	// this order is important, running teardown before canceling
	// the context to prevent closing the connection before
	// running the teardown procedure, given that some connection
	// resources may be shared in the driver.
	grip.CatchError(s.tearDown())
	s.canceler()
}

func (s *SimpleRemoteOrderedSuite) TestQueueSkipsCompletedJobs() {
	j := job.NewShellJob("echo hello", "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	j.MarkComplete()
	s.True(j.Status().Completed)

	s.NoError(s.queue.Start(ctx))
	s.NoError(s.queue.Put(j))

	amboy.WaitCtx(ctx, s.queue)

	stat := s.queue.Stats()

	s.Equal(1, stat.Total)
	s.Equal(1, stat.Completed)
}

func (s *SimpleRemoteOrderedSuite) TestQueueSkipsUnresolvedJobs() {
	j := job.NewShellJob("echo hello", "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.False(j.Status().Completed)
	mockDep := dependency.NewMock()
	mockDep.Response = dependency.Unresolved
	s.Equal(mockDep.State(), dependency.Unresolved)
	j.SetDependency(mockDep)

	s.NoError(s.queue.Start(ctx))
	s.NoError(s.queue.Put(j))

	amboy.WaitCtx(ctx, s.queue)

	stat := s.queue.Stats()

	s.Equal(1, stat.Total, fmt.Sprintf("%+v", stat))
	s.Equal(0, stat.Completed, fmt.Sprintf("%+v", stat))
}

func (s *SimpleRemoteOrderedSuite) TestQueueSkipsBlockedJobsWithNoEdges() {
	j := job.NewShellJob("echo hello", "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.False(j.Status().Completed)
	mockDep := dependency.NewMock()
	mockDep.Response = dependency.Blocked
	s.Equal(mockDep.State(), dependency.Blocked)
	s.Equal(len(mockDep.Edges()), 0)
	j.SetDependency(mockDep)

	s.NoError(s.queue.Start(ctx))
	s.NoError(s.queue.Put(j))

	amboy.WaitCtx(ctx, s.queue)

	stat := s.queue.Stats()

	s.Equal(stat.Total, 1, fmt.Sprintf("%+v", stat))
	s.Equal(stat.Completed, 0, fmt.Sprintf("%+v", stat))
}

func (s *SimpleRemoteOrderedSuite) TestQueueSkipsBlockedJobsWithManyEdges() {
	j := job.NewShellJob("echo hello", "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.False(j.Status().Completed)
	mockDep := dependency.NewMock()
	mockDep.Response = dependency.Blocked
	s.NoError(mockDep.AddEdge("foo"))
	s.NoError(mockDep.AddEdge("bar"))
	s.NoError(mockDep.AddEdge("bas"))
	s.Equal(mockDep.State(), dependency.Blocked)
	s.Equal(len(mockDep.Edges()), 3)
	j.SetDependency(mockDep)

	s.NoError(s.queue.Start(ctx))
	s.NoError(s.queue.Put(j))

	amboy.WaitCtx(ctx, s.queue)

	stat := s.queue.Stats()

	s.Equal(stat.Total, 1)
	s.Equal(stat.Completed, 0)
}

func (s *SimpleRemoteOrderedSuite) TestQueueSkipsBlockedJobsWithOneEdge() {
	j := job.NewShellJob("echo hello", "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.False(j.Status().Completed)
	mockDep := dependency.NewMock()
	mockDep.Response = dependency.Blocked
	s.NoError(mockDep.AddEdge("foo"))
	s.Equal(mockDep.State(), dependency.Blocked)
	s.Equal(len(mockDep.Edges()), 1)
	j.SetDependency(mockDep)

	s.NoError(s.queue.Start(ctx))
	s.NoError(s.queue.Put(j))

	amboy.WaitCtx(ctx, s.queue)

	stat := s.queue.Stats()

	s.Equal(stat.Total, 1)
	s.Equal(stat.Completed, 0)
}
