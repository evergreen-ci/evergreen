package model

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/suite"
)

type estimatorSuite struct {
	suite.Suite
	simulator estimatedTimeSimulator
}

func TestEstimatorSuite(t *testing.T) {
	suite.Run(t, &estimatorSuite{})
}

func (s *estimatorSuite) SetupTest() {
	s.simulator = estimatedTimeSimulator{}
	s.simulator.tasks = NewQueue()
	s.NoError(db.Clear(task.Collection))
}

func (s *estimatorSuite) TestCreateModel() {
	now := time.Now()
	hosts := []host.Host{
		{Status: evergreen.HostRunning, RunningTask: "t1"},
		{Status: evergreen.HostRunning, RunningTask: "t2"},
		{Status: evergreen.HostStarting},
	}
	t1 := task.Task{
		Id:               "t1",
		ExpectedDuration: 5 * time.Minute,
		DispatchTime:     now.Add(-4 * time.Minute),
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:               "t2",
		ExpectedDuration: 30 * time.Minute,
		DispatchTime:     now.Add(-10 * time.Minute),
	}
	s.NoError(t2.Insert())
	queue := TaskQueue{
		Queue: []TaskQueueItem{
			{ExpectedDuration: 1 * time.Hour}, {ExpectedDuration: 1 * time.Minute},
		},
	}

	model := createSimulatorModel(queue, hosts)
	s.InDelta(1*time.Minute, model.hosts[0].timeToCompletion, float64(100*time.Millisecond))
	s.InDelta(20*time.Minute, model.hosts[1].timeToCompletion, float64(100*time.Millisecond))
	s.InDelta(hostStartingDelay, model.hosts[2].timeToCompletion, float64(100*time.Millisecond))
}

func (s *estimatorSuite) TestNoHosts() {
	s.simulator.hosts = []estimatedHost{}
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 1 * time.Minute}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 1 * time.Minute}))
	s.EqualValues(-1, s.simulator.simulate(1))
}

func (s *estimatorSuite) TestNoTasks() {
	s.simulator.hosts = []estimatedHost{
		{timeToCompletion: 0}, {timeToCompletion: 0},
	}
	s.EqualValues(-1, s.simulator.simulate(0))
}

func (s *estimatorSuite) TestManyFreeHosts() {
	s.simulator.hosts = []estimatedHost{
		{timeToCompletion: 0}, {timeToCompletion: 0},
	}
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 1 * time.Minute}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 1 * time.Minute}))
	s.Equal(0*time.Minute, s.simulator.simulate(1))
}

func (s *estimatorSuite) TestSingleFreeHost() {
	s.simulator.hosts = []estimatedHost{
		{timeToCompletion: 0},
	}
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 1 * time.Minute}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 2 * time.Minute}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 3 * time.Minute}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 4 * time.Minute}))

	s.Equal(0*time.Minute, s.simulator.simulate(0))
	s.Equal(1*time.Minute, s.simulator.simulate(1))
	s.Equal(3*time.Minute, s.simulator.simulate(2))
	s.Equal(6*time.Minute, s.simulator.simulate(3))
}

func (s *estimatorSuite) TestSingleOccupiedHost() {
	s.simulator.hosts = []estimatedHost{
		{timeToCompletion: 7 * time.Minute},
	}
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 1 * time.Minute}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 2 * time.Minute}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 3 * time.Minute}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 4 * time.Minute}))

	s.Equal(7*time.Minute, s.simulator.simulate(0))
	s.Equal(8*time.Minute, s.simulator.simulate(1))
	s.Equal(10*time.Minute, s.simulator.simulate(2))
	s.Equal(13*time.Minute, s.simulator.simulate(3))
}

func (s *estimatorSuite) TestMultipleHosts() {
	s.simulator.hosts = []estimatedHost{
		{timeToCompletion: 5 * time.Second}, {timeToCompletion: 0 * time.Second}, {timeToCompletion: 15 * time.Second},
	}
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 1 * time.Second}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 5 * time.Second}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 15 * time.Second}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 30 * time.Second}))

	s.Equal(0*time.Second, s.simulator.simulate(0))
	s.Equal(1*time.Second, s.simulator.simulate(1))
	s.Equal(5*time.Second, s.simulator.simulate(2))
	s.Equal(6*time.Second, s.simulator.simulate(3))
}

func (s *estimatorSuite) TestMultipleHostsUnordered() {
	s.simulator.hosts = []estimatedHost{
		{timeToCompletion: 5 * time.Second}, {timeToCompletion: 0 * time.Second}, {timeToCompletion: 15 * time.Second},
	}
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 5 * time.Second}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 1 * time.Second}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 30 * time.Second}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 15 * time.Second}))

	s.Equal(0*time.Second, s.simulator.simulate(0))
	s.Equal(5*time.Second, s.simulator.simulate(1))
	s.Equal(5*time.Second, s.simulator.simulate(2))
	s.Equal(6*time.Second, s.simulator.simulate(3))
}

func (s *estimatorSuite) TestRunningHosts() {
	s.simulator.hosts = []estimatedHost{
		{timeToCompletion: 25 * time.Second}, {timeToCompletion: 10 * time.Second}, {timeToCompletion: 15 * time.Second},
	}
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 30 * time.Second}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 15 * time.Second}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 10 * time.Second}))
	s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 5 * time.Second}))

	s.Equal(10*time.Second, s.simulator.simulate(0))
	s.Equal(15*time.Second, s.simulator.simulate(1))
	s.Equal(25*time.Second, s.simulator.simulate(2))
	s.Equal(30*time.Second, s.simulator.simulate(3))
}

func (s *estimatorSuite) TestEvenDistribution() {
	s.simulator.hosts = []estimatedHost{
		{timeToCompletion: 10 * time.Second},
		{timeToCompletion: 10 * time.Second},
		{timeToCompletion: 10 * time.Second},
		{timeToCompletion: 10 * time.Second},
		{timeToCompletion: 10 * time.Second},
	}
	for i := 0; i < 14; i++ {
		s.NoError(s.simulator.tasks.Enqueue(estimatedTask{duration: 10 * time.Second}))
	}

	for i := 0; i < 14; i++ {
		s.Equal(time.Duration((i/5+1)*10)*time.Second, s.simulator.simulate(i), strconv.Itoa(i))
	}
}

type QueueSuite struct {
	q estimatedTaskQueue
	suite.Suite
}

func TestQueueSuite(t *testing.T) {
	suite.Run(t, &QueueSuite{})
}

func (s *QueueSuite) SetupTest() {
	s.q = NewQueue()
}

func (s *QueueSuite) TestQueueOperations() {
	s.True(s.q.IsEmpty())

	s.NoError(s.q.Enqueue(estimatedTask{duration: 1 * time.Minute}))
	s.False(s.q.IsEmpty())
	s.Equal(1*time.Minute, s.q.Peek().duration)
	s.Equal(1, s.q.Length())

	s.NoError(s.q.Enqueue(estimatedTask{duration: 2 * time.Minute}))
	s.False(s.q.IsEmpty())
	s.Equal(1*time.Minute, s.q.Peek().duration)
	s.Equal(2, s.q.Length())

	s.NoError(s.q.Enqueue(estimatedTask{duration: 3 * time.Minute}))
	s.False(s.q.IsEmpty())
	s.Equal(1*time.Minute, s.q.Peek().duration)
	s.Equal(3, s.q.Length())

	s.Equal(1*time.Minute, s.q.Dequeue().duration)
	s.False(s.q.IsEmpty())
	s.Equal(2*time.Minute, s.q.Peek().duration)
	s.Equal(2, s.q.Length())

	s.Equal(2*time.Minute, s.q.Dequeue().duration)
	s.False(s.q.IsEmpty())
	s.Equal(3*time.Minute, s.q.Peek().duration)
	s.Equal(1, s.q.Length())

	s.Equal(3*time.Minute, s.q.Dequeue().duration)
	s.True(s.q.IsEmpty())
	s.Nil(s.q.Peek())
	s.Equal(0, s.q.Length())
}

func (s *QueueSuite) TestQueueThreadsafe() {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		for i := 0; i < 100; i++ {
			s.NoError(s.q.Enqueue(estimatedTask{}))
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		for i := 0; i < 200; i++ {
			s.NoError(s.q.Enqueue(estimatedTask{}))
		}
		wg.Done()
	}()
	wg.Wait()
	s.Equal(300, s.q.Length())

	wg.Add(1)
	go func() {
		for i := 0; i < 200; i++ {
			s.q.Dequeue()
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		for i := 0; i < 100; i++ {
			s.q.Dequeue()
		}
		wg.Done()
	}()
	wg.Wait()
	s.Equal(0, s.q.Length())
}
