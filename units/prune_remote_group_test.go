package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
)

func remoteConstructor(ctx context.Context) (queue.Remote, error) {
	return queue.NewRemoteUnordered(1), nil
}

func TestPruneRemoteQueueGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job.RegisterDefaultJobs()
	client := evergreen.GetEnvironment().Client()
	require.NoError(t, client.Database("amboy_test").Drop(ctx))

	env := &mock.Environment{}

	var err error
	env.RemoteGroup, err = queue.NewMongoRemoteSingleQueueGroup(ctx,
		queue.RemoteQueueGroupOptions{
			DefaultWorkers: 1,
			Prefix:         "gen",
		},
		client,
		queue.MongoDBOptions{
			URI:             "mongodb://localhost:27017",
			DB:              "amboy_test",
			Priority:        false,
			CheckWaitUntil:  true,
			SkipIndexBuilds: true,
		},
	)
	require.NoError(t, err)

	pruneJob := &pruneRemoteQueueGroup{env: env}

	q, err := env.RemoteGroup.Get(ctx, "1")
	require.NoError(t, err)
	j := job.NewShellJob("true", "")
	require.NoError(t, q.Put(j))

	count, err := client.Database("amboy_test").Collection("gen.group").EstimatedDocumentCount(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	time.Sleep(100 * time.Millisecond)

	pruneJob.Run(ctx)
	require.NoError(t, pruneJob.Error())

	count, err = client.Database("amboy_test").Collection("gen.group").EstimatedDocumentCount(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	// the single queue prune job doesn't delete jobs from the db,
	// we need a Len() method on the queue group which isn't there
	// yet... but this should give us some confidence:

	count, err = client.Database("amboy_test").Collection("gen.group").CountDocuments(ctx, bson.M{"group": "1", "status.completed": true})
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}
