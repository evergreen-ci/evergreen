package queue

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

// the cases in this file attempt to test the behavior of the remote tasks.

func runSmokeRemoteQueuesRunsJobsOnce(ctx context.Context, driver Driver, cleanup func(), assert *assert.Assertions) {
	q := NewRemoteUnordered(4).(*remoteUnordered)

	assert.NoError(driver.Open(ctx))
	assert.NoError(q.SetDriver(driver))
	assert.NoError(q.Start(ctx))
	const single = 40

	for i := 0; i < single; i++ {
		j := newMockJob()
		jobID := fmt.Sprintf("%d.%s.%d", i, driver.ID(), job.GetNumber())
		j.SetID(jobID)
		assert.NoError(q.Put(j))
	}

	amboy.WaitCtxInterval(ctx, q, 10*time.Millisecond)
	assert.Equal(single, mockJobCounters.Count())

	cleanup()
}

func runSmokeMultipleQueuesRunJobsOnce(ctx context.Context, drivers []Driver, cleanup func(), assert *assert.Assertions) {
	queues := []*remoteUnordered{}

	for _, driver := range drivers {
		q := NewRemoteUnordered(4).(*remoteUnordered)
		assert.NoError(driver.Open(ctx))
		assert.NoError(q.SetDriver(driver))
		assert.NoError(q.Start(ctx))
		queues = append(queues, q)
	}
	const (
		inside  = 15
		outside = 10
	)

	wg := &sync.WaitGroup{}
	for i := 0; i < len(drivers); i++ {
		for ii := 0; ii < outside; ii++ {
			wg.Add(1)
			go func(i int) {
				for iii := 0; iii < inside; iii++ {
					j := newMockJob()
					jobID := fmt.Sprintf("%d-%d-%d-%d", i, i, iii, job.GetNumber())
					j.SetID(jobID)
					assert.NoError(queues[0].Put(j))
				}
				wg.Done()
			}(i)
		}
	}

	grip.Notice("waiting to add all jobs")
	wg.Wait()

	grip.Notice("waiting to run jobs")

	for _, q := range queues {
		amboy.WaitCtxInterval(ctx, q, 100*time.Millisecond)
	}

	for idx, q := range queues {
		grip.Alertln(idx, q.Stats())
	}

	assert.Equal(len(drivers)*inside*outside, mockJobCounters.Count())
	cleanup()
}

func TestSmokeMgoDriverRemoteQueueRunsJobsOnlyOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mockJobCounters.Reset()
	assert := assert.New(t)
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	name := uuid.NewV4().String()
	d := NewMgoDriver(name, opts)
	cleanup := func() { cleanupMgo(opts.DB, name, d.(*mgoDriver).session.Clone()) }

	runSmokeRemoteQueuesRunsJobsOnce(ctx, d, cleanup, assert)
}

func TestSmokeMongoDriverRemoteQueueRunsJobsOnlyOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mockJobCounters.Reset()
	assert := assert.New(t)
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	name := uuid.NewV4().String()
	d := NewMongoDriver(name, opts)
	cleanup := func() { cleanupMongo(ctx, opts.DB, name, d.(*mongoDriver).client) }

	runSmokeRemoteQueuesRunsJobsOnce(ctx, d, cleanup, assert)
}

func TestSmokeMgoDriverRemoteTwoQueueRunsJobsOnlyOnce(t *testing.T) {
	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	name := uuid.NewV4().String()
	drivers := []Driver{
		NewMgoDriver(name, opts),
		NewMgoDriver(name, opts),
	}
	cleanup := func() { cleanupMgo(opts.DB, name, drivers[0].(*mgoDriver).session.Clone()) }

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	mockJobCounters.Reset()
	runSmokeMultipleQueuesRunJobsOnce(ctx, drivers, cleanup, assert)
}

func TestSmokeMgoDriverRemoteManyQueueRunsJobsOnlyOnce(t *testing.T) {
	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	name := uuid.NewV4().String()
	drivers := []Driver{
		NewMgoDriver(name, opts),
		NewMgoDriver(name, opts),
		NewMgoDriver(name, opts),
		NewMgoDriver(name, opts),
		NewMgoDriver(name, opts),
		NewMgoDriver(name, opts),
	}
	cleanup := func() { cleanupMgo(opts.DB, name, drivers[0].(*mgoDriver).session.Clone()) }

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	mockJobCounters.Reset()
	runSmokeMultipleQueuesRunJobsOnce(ctx, drivers, cleanup, assert)
}

func TestSmokeMongoDriverRemoteTwoQueueRunsJobsOnlyOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	name := uuid.NewV4().String()
	drivers := []Driver{
		NewMongoDriver(name, opts),
		NewMongoDriver(name, opts),
	}
	cleanup := func() { cleanupMongo(ctx, opts.DB, name, drivers[0].(*mongoDriver).client) }

	ctx, cancel = context.WithTimeout(ctx, time.Minute)
	defer cancel()

	mockJobCounters.Reset()
	runSmokeMultipleQueuesRunJobsOnce(ctx, drivers, cleanup, assert)
}

func TestSmokeMongoDriverRemoteManyQueueRunsJobsOnlyOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	name := uuid.NewV4().String()
	drivers := []Driver{
		NewMongoDriver(name, opts),
		NewMongoDriver(name, opts),
		NewMongoDriver(name, opts),
		NewMongoDriver(name, opts),
		NewMongoDriver(name, opts),
		NewMongoDriver(name, opts),
	}
	cleanup := func() { cleanupMongo(ctx, opts.DB, name, drivers[0].(*mongoDriver).client) }

	ctx, cancel = context.WithTimeout(ctx, time.Minute)
	defer cancel()

	mockJobCounters.Reset()
	runSmokeMultipleQueuesRunJobsOnce(ctx, drivers, cleanup, assert)
}

func TestSQSFifoQueueRunsJobsOnlyOnce(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	q, err := NewSQSFifoQueue(randomString(8), 4)
	assert.NoError(err)
	assert.NoError(q.Start(ctx))
	wg := &sync.WaitGroup{}
	count := 0
	const (
		inside  = 250
		outside = 2
	)
	for i := 0; i < outside; i++ {
		wg.Add(1)
		go func(i int) {
			for ii := 0; ii < inside; ii++ {
				count++
				j := newMockJob()
				jobID := fmt.Sprintf("%d-%d-%d", i, ii, count)
				j.SetID(jobID)
				assert.NoError(q.Put(j))
			}
			wg.Done()
		}(i)
	}

	grip.Notice("waiting to add all jobs")
	wg.Wait()

	amboy.WaitCtxInterval(ctx, q, 20*time.Second)
	stats := q.Stats()
	grip.Alertln(stats)
	assert.True(stats.Total <= inside*outside)
	assert.Equal(stats.Total, stats.Pending+stats.Completed)
}

func TestMultipleSQSFifoQueueRunsJobsOnlyOnce(t *testing.T) {
	// case two
	mockJobCounters.Reset()

	assert := assert.New(t) // nolint
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	defer cancel()
	name := randomString(8)
	q, err := NewSQSFifoQueue(name, 4)
	assert.NoError(err)
	assert.NoError(q.Start(ctx))

	q2, err := NewSQSFifoQueue(name, 4)
	assert.NoError(err)
	assert.NoError(q2.Start(ctx))

	const (
		inside  = 250
		outside = 3
	)

	wg := &sync.WaitGroup{}
	count := 0
	for i := 0; i < outside; i++ {
		wg.Add(1)
		go func(i int) {
			for ii := 0; ii < inside; ii++ {
				count++
				j := newMockJob()
				jobID := fmt.Sprintf("%d-%d-%d", i, ii, count)
				j.SetID(jobID)
				assert.NoError(q2.Put(j))
			}
			wg.Done()
		}(i)
	}
	grip.Notice("waiting to add all jobs")
	wg.Wait()

	grip.Notice("waiting to run jobs")

	amboy.WaitCtxInterval(ctx, q, 10*time.Second)
	amboy.WaitCtxInterval(ctx, q2, 10*time.Second)

	stats1 := q.Stats()
	stats2 := q2.Stats()
	grip.Alertln("one", stats1)
	grip.Alertln("two", stats2)
	assert.Equal(stats1.Pending, stats2.Pending)

	sum := stats1.Pending + stats2.Pending + stats1.Completed + stats2.Completed
	assert.Equal(sum, stats1.Total+stats2.Total)
}
