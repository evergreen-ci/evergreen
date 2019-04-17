package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func localConstructor(ctx context.Context) (amboy.Queue, error) {
	return NewLocalUnordered(1), nil
}

func remoteConstructor(ctx context.Context) (Remote, error) {
	return NewRemoteUnordered(1), nil
}

func TestQueueGroupConstructor(t *testing.T) {
	for _, test := range []struct {
		name              string
		valid             bool
		localConstructor  Constructor
		remoteConstructor RemoteConstructor
		ttl               time.Duration
	}{
		{
			name:              "NilConstructorNegativeTime",
			localConstructor:  nil,
			remoteConstructor: nil,
			valid:             false,
			ttl:               -time.Minute,
		},
		{
			name:              "NilConstructorZeroTime",
			localConstructor:  nil,
			remoteConstructor: nil,
			valid:             false,
			ttl:               0,
		},
		{
			name:              "NilConstructorPositiveTime",
			localConstructor:  nil,
			remoteConstructor: nil,
			valid:             false,
			ttl:               time.Minute,
		},
		{
			name:              "ConstructorNegativeTime",
			localConstructor:  localConstructor,
			remoteConstructor: remoteConstructor,
			valid:             false,
			ttl:               -time.Minute,
		},
		{
			name:              "ConstructorZeroTime",
			localConstructor:  localConstructor,
			remoteConstructor: remoteConstructor,
			valid:             true,
			ttl:               0,
		},
		{
			name:              "ConstructorPositiveTime",
			localConstructor:  localConstructor,
			remoteConstructor: remoteConstructor,
			valid:             true,
			ttl:               time.Minute,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Run("local", func(t *testing.T) {
				localOpts := LocalQueueGroupOptions{
					Constructor: test.localConstructor,
					TTL:         test.ttl,
				}
				g, err := NewLocalQueueGroup(context.Background(), localOpts)
				if test.valid {
					require.NotNil(t, g)
					require.NoError(t, err)
				} else {
					require.Nil(t, g)
					require.Error(t, err)
				}
			})
			for _, remoteTest := range []struct {
				name   string
				db     string
				prefix string
				uri    string
				valid  bool
			}{
				{
					name:   "AllFieldsSet",
					db:     "db",
					prefix: "prefix",
					uri:    "uri",
					valid:  true,
				},
				{
					name:   "DBMissing",
					prefix: "prefix",
					uri:    "uri",
					valid:  false,
				},
				{
					name:  "PrefixMissing",
					db:    "db",
					uri:   "uri",
					valid: false,
				},
				{
					name:   "URIMissing",
					db:     "db",
					prefix: "prefix",
					valid:  false,
				},
			} {

				t.Run(remoteTest.name, func(t *testing.T) {
					client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
					require.NoError(t, err)
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					require.NoError(t, client.Connect(ctx))
					remoteOpts := RemoteQueueGroupOptions{
						Client: client,
						MongoOptions: MongoDBOptions{
							DB:  remoteTest.db,
							URI: remoteTest.uri,
						},
						Prefix:         remoteTest.prefix,
						Constructor:    test.remoteConstructor,
						TTL:            test.ttl,
						PruneFrequency: test.ttl,
					}
					g, err := NewRemoteQueueGroup(context.Background(), remoteOpts)
					if test.valid && remoteTest.valid {
						require.NotNil(t, g)
						require.NoError(t, err)
					} else {
						require.Nil(t, g)
						require.Error(t, err)
					}
				})
			}
		})
	}
}

type queueGroupConstructor func(*testing.T, context.Context, time.Duration) (amboy.QueueGroup, error)

func localQueueGroupConstructor(t *testing.T, ctx context.Context, ttl time.Duration) (amboy.QueueGroup, error) {
	return NewLocalQueueGroup(ctx, LocalQueueGroupOptions{Constructor: localConstructor, TTL: ttl})
}

func remoteQueueGroupConstructor(t *testing.T, ctx context.Context, ttl time.Duration) (amboy.QueueGroup, error) {
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		return nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	opts := RemoteQueueGroupOptions{
		Client:      client,
		Constructor: remoteConstructor,
		MongoOptions: MongoDBOptions{
			DB:  "amboy_test",
			URI: "mongodb://localhost:27017",
		},
		Prefix:         "prefix",
		TTL:            ttl,
		PruneFrequency: ttl,
	}
	return NewRemoteQueueGroup(ctx, opts)
}

func dropDatabase(ctx context.Context) error {
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		return err
	}
	if err := client.Connect(ctx); err != nil {
		return err
	}
	if err := client.Database("amboy_test").Drop(ctx); err != nil {
		return err
	}
	return nil
}

func TestQueueGroupOperations(t *testing.T) {
	queueGroups := map[string]queueGroupConstructor{
		"Local":  localQueueGroupConstructor,
		"Remote": remoteQueueGroupConstructor,
	}

	for groupName, constructor := range queueGroups {
		t.Run(groupName, func(t *testing.T) {
			t.Run("Get", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				require.NoError(t, dropDatabase(ctx))

				g, err := constructor(t, ctx, 0)
				require.NoError(t, err)
				require.NotNil(t, g)

				q1, err := g.Get(ctx, "one")
				require.NoError(t, err)
				require.NotNil(t, q1)

				q2, err := g.Get(ctx, "two")
				require.NoError(t, err)
				require.NotNil(t, q2)

				j1 := job.NewShellJob("true", "")
				j2 := job.NewShellJob("true", "")
				j3 := job.NewShellJob("true", "")

				// Add j1 to q1. Add j2 and j3 to q2.
				require.NoError(t, q1.Put(j1))
				require.NoError(t, q2.Put(j2))
				require.NoError(t, q2.Put(j3))

				amboy.Wait(q1)
				amboy.Wait(q2)

				resultsQ1 := []amboy.Job{}
				for result := range q1.Results(ctx) {
					resultsQ1 = append(resultsQ1, result)
				}
				resultsQ2 := []amboy.Job{}
				for result := range q2.Results(ctx) {
					resultsQ2 = append(resultsQ2, result)
				}
				require.Len(t, resultsQ1, 1)
				require.Len(t, resultsQ2, 2)

				// Try getting the queues again
				q1, err = g.Get(ctx, "one")
				require.NoError(t, err)
				require.NotNil(t, q1)

				q2, err = g.Get(ctx, "two")
				require.NoError(t, err)
				require.NotNil(t, q2)

				// The queues should be the same, i.e., contain the jobs we expect
				resultsQ1 = []amboy.Job{}
				for result := range q1.Results(ctx) {
					resultsQ1 = append(resultsQ1, result)
				}
				resultsQ2 = []amboy.Job{}
				for result := range q2.Results(ctx) {
					resultsQ2 = append(resultsQ2, result)
				}
				require.Len(t, resultsQ1, 1)
				require.Len(t, resultsQ2, 2)
			})

			t.Run("Put", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				require.NoError(t, dropDatabase(ctx))

				g, err := constructor(t, ctx, 0)
				require.NoError(t, err)
				require.NotNil(t, g)

				q1, err := g.Get(ctx, "one")
				require.NoError(t, err)
				require.NotNil(t, q1)
				require.NoError(t, q1.Start(ctx))

				q2, err := localConstructor(ctx)
				require.NoError(t, err)
				require.Error(t, g.Put(ctx, "one", q2), "cannot add queue to existing index")
				require.NoError(t, q2.Start(ctx))

				q3, err := localConstructor(ctx)
				require.NoError(t, err)
				require.NoError(t, g.Put(ctx, "three", q3))
				require.NoError(t, q3.Start(ctx))

				q4, err := localConstructor(ctx)
				require.NoError(t, err)
				require.NoError(t, g.Put(ctx, "four", q4))
				require.NoError(t, q4.Start(ctx))

				j1 := job.NewShellJob("true", "")
				j2 := job.NewShellJob("true", "")
				j3 := job.NewShellJob("true", "")

				// Add j1 to q3. Add j2 and j3 to q4.
				require.NoError(t, q3.Put(j1))
				require.NoError(t, q4.Put(j2))
				require.NoError(t, q4.Put(j3))

				amboy.Wait(q3)
				amboy.Wait(q4)

				resultsQ3 := []amboy.Job{}
				for result := range q3.Results(ctx) {
					resultsQ3 = append(resultsQ3, result)
				}
				resultsQ4 := []amboy.Job{}
				for result := range q4.Results(ctx) {
					resultsQ4 = append(resultsQ4, result)
				}
				require.Len(t, resultsQ3, 1)
				require.Len(t, resultsQ4, 2)

				// Try getting the queues again
				q3, err = g.Get(ctx, "three")
				require.NoError(t, err)
				require.NotNil(t, q3)

				q4, err = g.Get(ctx, "four")
				require.NoError(t, err)
				require.NotNil(t, q4)

				// The queues should be the same, i.e., contain the jobs we expect
				resultsQ3 = []amboy.Job{}
				for result := range q3.Results(ctx) {
					resultsQ3 = append(resultsQ3, result)
				}
				resultsQ4 = []amboy.Job{}
				for result := range q4.Results(ctx) {
					resultsQ4 = append(resultsQ4, result)
				}
				require.Len(t, resultsQ3, 1)
				require.Len(t, resultsQ4, 2)
			})

			t.Run("Prune", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				require.NoError(t, dropDatabase(ctx))

				g, err := constructor(t, ctx, 0)
				require.NoError(t, err)
				require.NotNil(t, g)

				q1, err := g.Get(ctx, "one")
				require.NoError(t, err)
				require.NotNil(t, q1)

				q2, err := g.Get(ctx, "two")
				require.NoError(t, err)
				require.NotNil(t, q2)

				j1 := job.NewShellJob("true", "")
				j2 := job.NewShellJob("true", "")
				j3 := job.NewShellJob("true", "")

				// Add j1 to q1. Add j2 and j3 to q2.
				require.NoError(t, q1.Put(j1))
				require.NoError(t, q2.Put(j2))
				require.NoError(t, q2.Put(j3))

				amboy.Wait(q1)
				amboy.Wait(q2)

				// Queues should have completed work
				stats1 := q1.Stats()
				require.Zero(t, stats1.Running)
				require.Equal(t, 1, stats1.Completed)
				require.Zero(t, stats1.Pending)
				require.Zero(t, stats1.Blocked)
				require.Equal(t, 1, stats1.Total)

				stats2 := q2.Stats()
				require.Zero(t, stats2.Running)
				require.Equal(t, 2, stats2.Completed)
				require.Zero(t, stats2.Pending)
				require.Zero(t, stats2.Blocked)
				require.Equal(t, 2, stats2.Total)

				require.NoError(t, g.Prune(ctx))

				// Try getting the queues again
				q1, err = g.Get(ctx, "one")
				require.NoError(t, err)
				require.NotNil(t, q1)

				q2, err = g.Get(ctx, "two")
				require.NoError(t, err)
				require.NotNil(t, q2)

				// Queues should be empty
				stats1 = q1.Stats()
				require.Zero(t, stats1.Running)
				require.Zero(t, stats1.Completed)
				require.Zero(t, stats1.Pending)
				require.Zero(t, stats1.Blocked)
				require.Zero(t, stats1.Total)

				stats2 = q2.Stats()
				require.Zero(t, stats2.Running)
				require.Zero(t, stats2.Completed)
				require.Zero(t, stats2.Pending)
				require.Zero(t, stats2.Blocked)
				require.Zero(t, stats2.Total)
			})

			t.Run("PruneWithTTL", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				require.NoError(t, dropDatabase(ctx))

				g, err := constructor(t, ctx, time.Second)
				require.NoError(t, err)
				require.NotNil(t, g)

				q1, err := g.Get(ctx, "one")
				require.NoError(t, err)
				require.NotNil(t, q1)

				q2, err := g.Get(ctx, "two")
				require.NoError(t, err)
				require.NotNil(t, q2)

				j1 := job.NewShellJob("true", "")
				j2 := job.NewShellJob("true", "")
				j3 := job.NewShellJob("true", "")

				// Add j1 to q1. Add j2 and j3 to q2.
				require.NoError(t, q1.Put(j1))
				require.NoError(t, q2.Put(j2))
				require.NoError(t, q2.Put(j3))

				amboy.Wait(q1)
				amboy.Wait(q2)

				// Queues should have completed work
				stats1 := q1.Stats()
				require.Zero(t, stats1.Running)
				require.Equal(t, 1, stats1.Completed)
				require.Zero(t, stats1.Pending)
				require.Zero(t, stats1.Blocked)
				require.Equal(t, 1, stats1.Total)

				stats2 := q2.Stats()
				require.Zero(t, stats2.Running)
				require.Equal(t, 2, stats2.Completed)
				require.Zero(t, stats2.Pending)
				require.Zero(t, stats2.Blocked)
				require.Equal(t, 2, stats2.Total)

				time.Sleep(2 * time.Second)

				// Try getting the queues again
				q1, err = g.Get(ctx, "one")
				require.NoError(t, err)
				require.NotNil(t, q1)

				q2, err = g.Get(ctx, "two")
				require.NoError(t, err)
				require.NotNil(t, q2)

				// Queues should be empty
				stats1 = q1.Stats()
				require.Zero(t, stats1.Running)
				require.Zero(t, stats1.Completed)
				require.Zero(t, stats1.Pending)
				require.Zero(t, stats1.Blocked)
				require.Zero(t, stats1.Total)

				stats2 = q2.Stats()
				require.Zero(t, stats2.Running)
				require.Zero(t, stats2.Completed)
				require.Zero(t, stats2.Pending)
				require.Zero(t, stats2.Blocked)
				require.Zero(t, stats2.Total)
			})

			t.Run("Close", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				require.NoError(t, dropDatabase(ctx))

				g, err := constructor(t, ctx, 0)
				require.NoError(t, err)
				require.NotNil(t, g)

				q1, err := g.Get(ctx, "one")
				require.NoError(t, err)
				require.NotNil(t, q1)

				q2, err := g.Get(ctx, "two")
				require.NoError(t, err)
				require.NotNil(t, q2)

				j1 := job.NewShellJob("true", "")
				j2 := job.NewShellJob("true", "")
				j3 := job.NewShellJob("true", "")

				// Add j1 to q1. Add j2 and j3 to q2.
				require.NoError(t, q1.Put(j1))
				require.NoError(t, q2.Put(j2))
				require.NoError(t, q2.Put(j3))

				amboy.Wait(q1)
				amboy.Wait(q2)

				g.Close(ctx)
			})
		})
	}
}

func TestQueueGroupConstructorPruneSmokeTest(t *testing.T) {
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, client.Connect(ctx))
	for i := 0; i < 10; i++ {
		_, err := client.Database("amboy_test").Collection(fmt.Sprintf("gen-%d.jobs", i)).InsertOne(ctx, bson.M{"foo": "bar"})
		require.NoError(t, err)
	}
	remoteOpts := RemoteQueueGroupOptions{
		Client: client,
		MongoOptions: MongoDBOptions{
			DB:  "amboy_test",
			URI: "mongodb://localhost:27017",
		},
		Prefix:         "gen",
		Constructor:    remoteConstructor,
		TTL:            time.Second,
		PruneFrequency: time.Second,
	}
	_, err = NewRemoteQueueGroup(ctx, remoteOpts)
	require.NoError(t, err)
	time.Sleep(time.Second)
	for i := 0; i < 10; i++ {
		count, err := client.Database("amboy_test").Collection(fmt.Sprintf("gen-%d.jobs", i)).CountDocuments(ctx, bson.M{})
		require.NoError(t, err)
		require.Zero(t, count, fmt.Sprintf("gen-%d.jobs not dropped", i))
	}

}
