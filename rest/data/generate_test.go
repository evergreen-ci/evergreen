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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePoll(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection))
	env := evergreen.GetEnvironment()
	require.NotNil(t, env)
	q := env.RemoteQueueGroup()
	require.NotNil(t, q)

	gc := &GenerateConnector{}
	finished, errs, err := gc.GeneratePoll(context.Background(), "task-0", q)
	assert.False(t, finished)
	assert.Empty(t, errs)
	assert.Error(t, err)

	require.NoError(t, (&task.Task{
		Id:             "task-1",
		Version:        "version-1",
		GenerateTask:   true,
		GeneratedTasks: false,
	}).Insert())

	finished, errs, err = gc.GeneratePoll(context.Background(), "task-1", q)
	assert.False(t, finished)
	assert.Empty(t, errs)
	assert.NoError(t, err)

	require.NoError(t, (&task.Task{
		Id:             "task-2",
		Version:        "version-1",
		GenerateTask:   true,
		GeneratedTasks: true,
	}).Insert())

	finished, errs, err = gc.GeneratePoll(context.Background(), "task-2", q)
	assert.True(t, finished)
	assert.Empty(t, errs)
	assert.NoError(t, err)

	require.NoError(t, (&task.Task{
		Id:                 "task-3",
		Version:            "version-1",
		GenerateTask:       true,
		GeneratedTasks:     true,
		GenerateTasksError: "this is an error",
	}).Insert())

	finished, errs, err = gc.GeneratePoll(context.Background(), "task-3", q)
	assert.True(t, finished)
	assert.Equal(t, []string{"this is an error"}, errs)
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
