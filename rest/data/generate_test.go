package data

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePoll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(task.Collection))
	env := evergreen.GetEnvironment()
	require.NotNil(t, env)
	q := env.RemoteQueueGroup()
	require.NotNil(t, q)

	require.NoError(t, (&task.Task{
		Id:      "task-1",
		Version: "version-1",
	}).Insert())
	gc := &GenerateConnector{}
	finished, errs, err := gc.GeneratePoll(context.Background(), "task-1", q)
	assert.Empty(t, errs)
	assert.False(t, finished)
	assert.Error(t, err)

	registry.AddJobType("mock", func() amboy.Job { return newMockJob() })
	j := newMockJob()
	j.SetID("generate-tasks-task-1")

	taskQueue, err := q.Get(ctx, "version-1")
	require.NoError(t, err)
	require.NotNil(t, taskQueue)
	require.NoError(t, taskQueue.Put(j))
	finished, errs, err = gc.GeneratePoll(context.Background(), "task-1", q)
	assert.False(t, finished)
	assert.Empty(t, errs)
	assert.NoError(t, err)
	time.Sleep(time.Second)
	finished, errs, err = gc.GeneratePoll(context.Background(), "task-1", q)
	assert.True(t, finished)
	assert.Empty(t, errs)
	assert.NoError(t, err)
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
