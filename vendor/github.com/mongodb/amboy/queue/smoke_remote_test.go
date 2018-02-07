package queue

import (
	"context"
	"fmt"
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
	q := NewRemoteUnordered(3).(*remoteUnordered)

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

	amboy.WaitCtxInterval(ctx, q, 5*time.Second)

	grip.Notice(q.Stats())
	assert.Equal(atStart+40, mockJobCounters.Count())
}
