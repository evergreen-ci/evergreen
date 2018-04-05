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
	atStart := mockJobCounters.Count()
	for i := 0; i < 40; i++ {
		j := newMockJob()
		jobID := fmt.Sprintf("%d.%s.%d", i, name, job.GetNumber())
		j.SetID(jobID)
		assert.NoError(q.Put(j))
	}

	amboy.WaitCtxInterval(ctx, q, 10*time.Millisecond)

	grip.Notice(q.Stats())
	assert.Equal(atStart+40, mockJobCounters.Count())

	// case two

	mockJobCounters.Reset()

	d2 := NewMongoDBDriver(name, opts)
	q2 := NewRemoteUnordered(4).(*remoteUnordered)

	assert.NoError(d2.Open(ctx))
	assert.NoError(q2.SetDriver(d2))
	assert.NoError(q2.Start(ctx))

	atStart = mockJobCounters.Count()
	wg := &sync.WaitGroup{}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ii := 0; ii < 20; ii++ {
				j := newMockJob()
				jobID := fmt.Sprintf("%d.%s.%d", i, name, job.GetNumber())
				j.SetID(jobID)
				assert.NoError(q.Put(j))
			}
		}()
	}
	grip.Notice("waiting to add all jobs")
	wg.Wait()

	grip.Notice("waiting to run jobs")
	amboy.WaitCtxInterval(ctx, q, 10*time.Millisecond)
	amboy.WaitCtxInterval(ctx, q2, 10*time.Millisecond)

	grip.Notice(q.Stats())
	assert.Equal(atStart+50*20, mockJobCounters.Count())
}
