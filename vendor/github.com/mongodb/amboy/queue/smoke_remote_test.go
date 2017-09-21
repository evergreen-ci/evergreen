package queue

import (
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/queue/driver"
	"github.com/mongodb/grip"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

// the cases in this file attempt to test the behavior of the remote tasks.

func TestRemoteQueueRunsJobsOnlyOnceWithMultipleWorkers(t *testing.T) {
	assert := assert.New(t)
	opts := driver.DefaultMongoDBOptions()
	name := uuid.NewV4().String()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	d := driver.NewMongoDB(name, opts)
	q := NewRemoteUnordered(3)

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

	amboy.WaitCtxInterval(ctx, q, 500*time.Millisecond)

	grip.Notice(q.Stats())
	assert.Equal(atStart+40, mockJobCounters.Count())
	fmt.Println(atStart+40, mockJobCounters.Count())
}
