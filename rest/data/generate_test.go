package data

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
)

func TestGeneratePoll(t *testing.T) {
	assert := assert.New(t)
	gc := &GenerateConnector{}
	q := queue.NewLocalUnordered(1)
	finished, errs, err := gc.GeneratePoll(context.Background(), "1", q)
	assert.Empty(errs)
	assert.False(finished)
	assert.Error(err)
	j := &mockJob{}
	j.SetID("generate-tasks-1")
	assert.NoError(q.Start(context.Background()))
	assert.NoError(q.Put(j))
	finished, errs, err = gc.GeneratePoll(context.Background(), "1", q)
	assert.False(finished)
	assert.Empty(errs)
	assert.NoError(err)
	time.Sleep(20 * time.Millisecond)
	finished, errs, err = gc.GeneratePoll(context.Background(), "1", q)
	assert.True(finished)
	assert.Empty(errs)
	assert.NoError(err)
}

type mockJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func newMockJob() *mockJob {
	j := &mockJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "mock",
				Version: 1,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *mockJob) Run(_ context.Context) {
	time.Sleep(10 * time.Millisecond)
	defer j.MarkComplete()
}
