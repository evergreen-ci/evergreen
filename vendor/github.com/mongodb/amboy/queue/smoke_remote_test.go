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

func TestSmokeRemoteQueueRunsJobsOnlyOnceWithMultipleWorkers(t *testing.T) {
	mockJobCounters.Reset()

	assert := assert.New(t)
	opts := DefaultMongoDBOptions()
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

func TestSmokeRemoteMultipleQueueRunsJobsOnlyOnceWithMultipleWorkers(t *testing.T) {
	// case two

	mockJobCounters.Reset()

	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	name := uuid.NewV4().String()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

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
		inside  = 25
		outside = 10
	)

	wg := &sync.WaitGroup{}
	for i := 0; i < outside; i++ {
		wg.Add(1)
		go func() {
			for ii := 0; ii < inside; ii++ {
				j := newMockJob()
				jobID := fmt.Sprintf("%d", job.GetNumber())
				j.SetID(jobID)
				assert.NoError(q2.Put(j))
			}
			wg.Done()
		}()
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
