package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/amboy/registry"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestGeneratePoll(t *testing.T) {
	registry.AddJobType("mock", func() amboy.Job { return newMockJob() })
	require := require.New(t)
	gc := &GenerateConnector{}

	// Set up the queue
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := queue.DefaultMongoDBOptions()
	opts.DB = "amboy_test_generate"
	client, err := mongo.NewClient(options.Client().ApplyURI(opts.URI))
	require.NoError(err)
	require.NoError(client.Connect(ctx))
	require.NoError(client.Database(opts.DB).Drop(ctx))
	singlemdb, err := queue.OpenNewMongoDriver(ctx, opts.DB, opts, client)
	require.NoError(err)
	q := queue.NewRemoteUnordered(8)
	require.NoError(q.SetDriver(singlemdb))
	require.NoError(q.SetRunner(pool.NewAbortablePool(1, q)))
	require.NoError(q.Start(context.Background()))

	// Insert some jobs
	var j *mockJob
	for i := 2; i < 10; i++ {
		j = newMockJob()
		j.SetID(fmt.Sprintf("generate-tasks-%d", i))
		require.NoError(q.Put(j))
	}

	// Queue does not contain job we're interested in
	finished, errs, err := gc.GeneratePoll(context.Background(), "1", q)
	require.Empty(errs)
	require.False(finished)
	require.Error(err)

	// Insert some more jobs
	for i := 10; i < 20; i++ {
		j = newMockJob()
		j.SetID(fmt.Sprintf("generate-tasks-%d", i))
		require.NoError(q.Put(j))
	}

	// Queue still does not contain job we're interested in
	finished, errs, err = gc.GeneratePoll(context.Background(), "1", q)
	require.Empty(errs)
	require.False(finished)
	require.Error(err)

	// Put the job
	j = newMockJob()
	j.SetID("generate-tasks-1")
	require.NoError(q.Put(j))

	// Job should be there, but not finished
	finished, errs, err = gc.GeneratePoll(context.Background(), "1", q)
	require.False(finished)
	require.Empty(errs)
	require.NoError(err)

	// Job should be there, and finished
	time.Sleep(time.Second)
	finished, errs, err = gc.GeneratePoll(context.Background(), "1", q)
	require.True(finished)
	require.Empty(errs)
	require.NoError(err)
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
