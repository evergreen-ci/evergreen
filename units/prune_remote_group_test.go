package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/mock"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func remoteConstructor(ctx context.Context) (queue.Remote, error) {
	return queue.NewRemoteUnordered(1), nil
}

func TestPruneRemoteQueueGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job.RegisterDefaultJobs()
	uri := "mongodb://localhost:27017"
	client, err := mongo.NewClient(options.Client().ApplyURI(uri))
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	require.NoError(t, client.Database("amboy_test").Drop(ctx))

	env := &mock.Environment{}
	opts := queue.RemoteQueueGroupOptions{
		Client:      client,
		Constructor: remoteConstructor,
		MongoOptions: queue.MongoDBOptions{
			URI: uri,
			DB:  "amboy_test",
		},
		Prefix: "gen",
		TTL:    time.Millisecond,
	}
	g, err := queue.NewRemoteQueueGroup(ctx, opts)
	require.NoError(t, err)
	env.RemoteGroup = g

	pruneJob := &pruneRemoteQueueGroup{env: env}

	q, err := g.Get(ctx, "1")
	j := job.NewShellJob("true", "")
	require.NoError(t, q.Put(j))

	count, err := client.Database("amboy_test").Collection("gen1.jobs").EstimatedDocumentCount(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	time.Sleep(100 * time.Millisecond)

	pruneJob.Run(ctx)
	require.NoError(t, pruneJob.Error())

	count, err = client.Database("amboy_test").Collection("gen1.jobs").EstimatedDocumentCount(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)
}
