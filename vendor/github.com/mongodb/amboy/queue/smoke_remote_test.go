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

func TestSmokeRemoteQueueRunsJobsOnlyOnce(t *testing.T) {
	mockJobCounters.Reset()

	assert := assert.New(t)
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	name := uuid.NewV4().String()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	d := NewMongoDBDriver(name, opts)
	q := NewRemoteUnordered(4).(*remoteUnordered)

	defer cleanupMongoDB(name, opts)
	defer cancel()

	assert.NoError(d.Open(ctx))
	assert.NoError(q.SetDriver(d))
	assert.NoError(q.Start(ctx))
	const single = 40

	for i := 0; i < single; i++ {
		j := newMockJob()
		jobID := fmt.Sprintf("%d.%s.%d", i, name, job.GetNumber())
		j.SetID(jobID)
		assert.NoError(q.Put(j))
	}

	amboy.WaitCtxInterval(ctx, q, 10*time.Millisecond)
	assert.Equal(single, mockJobCounters.Count())
}

func TestSmokeRemoteMultipleQueueRunsJobsOnlyOnce(t *testing.T) {
	// case two

	mockJobCounters.Reset()

	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	name := uuid.NewV4().String()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	defer cleanupMongoDB(name, opts)
	defer cancel()

	d := NewMongoDBDriver(name, opts)
	q := NewRemoteUnordered(4).(*remoteUnordered)
	assert.NoError(d.Open(ctx))
	assert.NoError(q.SetDriver(d))
	assert.NoError(q.Start(ctx))

	d2 := NewMongoDBDriver(name, opts)
	q2 := NewRemoteUnordered(4).(*remoteUnordered)

	assert.NoError(d2.Open(ctx))
	assert.NoError(q2.SetDriver(d2))
	assert.NoError(q2.Start(ctx))

	const (
		inside  = 15
		outside = 10
	)

	wg := &sync.WaitGroup{}
	for i := 0; i < outside; i++ {
		wg.Add(1)
		go func(i int) {
			for ii := 0; ii < inside; ii++ {
				j := newMockJob()
				jobID := fmt.Sprintf("%d-%d-%d", i, ii, job.GetNumber())
				j.SetID(jobID)
				assert.NoError(q2.Put(j))
			}
			wg.Done()
		}(i)
	}
	grip.Notice("waiting to add all jobs")
	wg.Wait()

	grip.Notice("waiting to run jobs")

	amboy.WaitCtxInterval(ctx, q, 100*time.Millisecond)
	amboy.WaitCtxInterval(ctx, q2, 100*time.Millisecond)

	grip.Alertln("one", q.Stats())
	grip.Alertln("two", q2.Stats())
	assert.Equal(inside*outside, mockJobCounters.Count())
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
