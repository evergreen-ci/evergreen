package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	mgo "gopkg.in/mgo.v2"
)

func localConstructor(ctx context.Context) (amboy.Queue, error) {
	return NewLocalUnordered(1), nil
}

func TestQueueGroupConstructor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const mdburl = "mongodb://localhost:27017"
	client, err := mongo.NewClient(options.Client().ApplyURI(mdburl).SetConnectTimeout(2 * time.Second))
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	defer func() { require.NoError(t, client.Disconnect(ctx)) }()

	session, err := mgo.DialWithTimeout(mdburl, 2*time.Second)
	require.NoError(t, err)
	defer session.Close()

	for _, test := range []struct {
		name             string
		valid            bool
		localConstructor Constructor
		ttl              time.Duration
		skipRemote       bool
	}{
		{
			name:             "NilConstructorNegativeTime",
			localConstructor: nil,
			valid:            false,
			ttl:              -time.Minute,
			skipRemote:       true,
		},
		{
			name:             "NilConstructorZeroTime",
			localConstructor: nil,
			valid:            false,
			ttl:              0,
			skipRemote:       true,
		},
		{
			name:             "NilConstructorPositiveTime",
			localConstructor: nil,
			valid:            false,
			ttl:              time.Minute,
			skipRemote:       true,
		},
		{
			name:             "ConstructorNegativeTime",
			localConstructor: localConstructor,
			valid:            false,
			ttl:              -time.Minute,
		},
		{
			name:             "ConstructorZeroTime",
			localConstructor: localConstructor,
			valid:            true,
			ttl:              0,
		},
		{
			name:             "ConstructorPositiveTime",
			localConstructor: localConstructor,
			valid:            true,
			ttl:              time.Minute,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Run("Local", func(t *testing.T) {
				localOpts := LocalQueueGroupOptions{
					Constructor: test.localConstructor,
					TTL:         test.ttl,
				}
				g, err := NewLocalQueueGroup(ctx, localOpts) // nolint
				if test.valid {
					require.NotNil(t, g)
					require.NoError(t, err)
				} else {
					require.Nil(t, g)
					require.Error(t, err)
				}
			})
			if test.skipRemote {
				return
			}

			remoteTests := []struct {
				name       string
				db         string
				prefix     string
				uri        string
				workers    int
				workerFunc func(string) int
				valid      bool
			}{
				{
					name:       "AllFieldsSet",
					db:         "db",
					prefix:     "prefix",
					uri:        "uri",
					workerFunc: func(s string) int { return 1 },
					workers:    1,
					valid:      true,
				},
				{
					name:   "WorkersMissing",
					db:     "db",
					prefix: "prefix",
					uri:    "uri",
					valid:  false,
				},
				{
					name:       "WorkerFunctions",
					db:         "db",
					prefix:     "prefix",
					workerFunc: func(s string) int { return 1 },
					uri:        "uri",
					valid:      true,
				},
				{
					name:    "WorkerDefault",
					db:      "db",
					prefix:  "prefix",
					workers: 2,
					uri:     "uri",
					valid:   true,
				},
				{
					name:    "DBMissing",
					prefix:  "prefix",
					uri:     "uri",
					workers: 1,
					valid:   false,
				},
				{
					name:    "PrefixMissing",
					db:      "db",
					workers: 1,
					uri:     "uri",
					valid:   false,
				},
				{
					name:    "URIMissing",
					db:      "db",
					prefix:  "prefix",
					workers: 1,
					valid:   false,
				},
			}

			t.Run("Mongo", func(t *testing.T) {
				for _, remoteTest := range remoteTests {
					t.Run(remoteTest.name, func(t *testing.T) {
						require.NoError(t, err)
						tctx, cancel := context.WithCancel(ctx)
						defer cancel()
						mopts := MongoDBOptions{
							DB:  remoteTest.db,
							URI: remoteTest.uri,
						}

						remoteOpts := RemoteQueueGroupOptions{
							DefaultWorkers: remoteTest.workers,
							WorkerPoolSize: remoteTest.workerFunc,
							Prefix:         remoteTest.prefix,
							TTL:            test.ttl,
							PruneFrequency: test.ttl,
						}

						g, err := NewMongoRemoteQueueGroup(tctx, remoteOpts, client, mopts) // nolint
						if test.valid && remoteTest.valid {
							require.NoError(t, err)
							require.NotNil(t, g)
						} else {
							require.Error(t, err)
							require.Nil(t, g)
						}
					})
				}
			})
			t.Run("MongoMerged", func(t *testing.T) {
				for _, remoteTest := range remoteTests {
					t.Run(remoteTest.name, func(t *testing.T) {
						require.NoError(t, err)
						tctx, cancel := context.WithCancel(ctx)
						defer cancel()
						mopts := MongoDBOptions{
							DB:  remoteTest.db,
							URI: remoteTest.uri,
						}

						remoteOpts := RemoteQueueGroupOptions{
							DefaultWorkers: remoteTest.workers,
							WorkerPoolSize: remoteTest.workerFunc,
							Prefix:         remoteTest.prefix,
							TTL:            test.ttl,
							PruneFrequency: test.ttl,
						}

						g, err := NewMongoRemoteSingleQueueGroup(tctx, remoteOpts, client, mopts) // nolint
						if test.valid && remoteTest.valid {
							require.NoError(t, err)
							require.NotNil(t, g)
						} else {
							require.Error(t, err)
							require.Nil(t, g)
						}
					})
				}
			})

			t.Run("LegacyMgo", func(t *testing.T) {
				for _, remoteTest := range remoteTests {
					t.Run(remoteTest.name, func(t *testing.T) {
						require.NoError(t, err)
						tctx, cancel := context.WithCancel(ctx)
						defer cancel()
						mopts := MongoDBOptions{
							DB:  remoteTest.db,
							URI: remoteTest.uri,
						}

						remoteOpts := RemoteQueueGroupOptions{
							DefaultWorkers: remoteTest.workers,
							WorkerPoolSize: remoteTest.workerFunc,
							Prefix:         remoteTest.prefix,
							TTL:            test.ttl,
							PruneFrequency: test.ttl,
						}
						g, err := NewMgoRemoteQueueGroup(tctx, remoteOpts, session, mopts) // nolint
						if test.valid && remoteTest.valid {
							require.NoError(t, err)
							require.NotNil(t, g)
						} else {
							require.Error(t, err)
							require.Nil(t, g)
						}
					})
				}
			})

			t.Run("LegacyMgoMerged", func(t *testing.T) {
				for _, remoteTest := range remoteTests {
					t.Run(remoteTest.name, func(t *testing.T) {
						require.NoError(t, err)
						tctx, cancel := context.WithCancel(ctx)
						defer cancel()
						mopts := MongoDBOptions{
							DB:  remoteTest.db,
							URI: remoteTest.uri,
						}

						remoteOpts := RemoteQueueGroupOptions{
							DefaultWorkers: remoteTest.workers,
							WorkerPoolSize: remoteTest.workerFunc,
							Prefix:         remoteTest.prefix,
							TTL:            test.ttl,
							PruneFrequency: test.ttl,
						}
						g, err := NewMgoRemoteSingleQueueGroup(tctx, remoteOpts, session, mopts) // nolint
						if test.valid && remoteTest.valid {
							require.NoError(t, err)
							require.NotNil(t, g)
						} else {
							require.Error(t, err)
							require.Nil(t, g)
						}
					})
				}
			})

		})
	}
}

type queueGroupCloser func(context.Context) error
type queueGroupConstructor func(context.Context, time.Duration) (amboy.QueueGroup, queueGroupCloser, error)

func localQueueGroupConstructor(ctx context.Context, ttl time.Duration) (amboy.QueueGroup, queueGroupCloser, error) {
	qg, err := NewLocalQueueGroup(ctx, LocalQueueGroupOptions{Constructor: localConstructor, TTL: ttl})
	closer := func(_ context.Context) error { return nil }
	return qg, closer, err
}

func remoteQueueGroupConstructor(ctx context.Context, ttl time.Duration) (amboy.QueueGroup, queueGroupCloser, error) {
	mopts := MongoDBOptions{
		DB:  "amboy_test",
		URI: "mongodb://localhost:27017",
	}

	client, err := mongo.NewClient(options.Client().ApplyURI(mopts.URI).SetConnectTimeout(2 * time.Second))
	if err != nil {
		return nil, func(_ context.Context) error { return nil }, err
	}

	closer := func(cctx context.Context) error {
		catcher := grip.NewBasicCatcher()
		catcher.Add(client.Database(mopts.DB).Drop(cctx))
		catcher.Add(client.Disconnect(cctx))
		return catcher.Resolve()
	}

	if err = client.Connect(ctx); err != nil {
		return nil, closer, err
	}
	opts := RemoteQueueGroupOptions{
		DefaultWorkers: 1,
		Prefix:         "prefix",
		TTL:            ttl,
		PruneFrequency: ttl,
	}

	if err = client.Database(mopts.DB).Drop(ctx); err != nil {
		return nil, closer, err
	}

	if err = client.Ping(ctx, nil); err != nil {
		return nil, closer, errors.Wrap(err, "server not pingable")
	}

	qg, err := NewMongoRemoteQueueGroup(ctx, opts, client, mopts)
	return qg, closer, err
}

func remoteLegacyQueueGroupConstructor(ctx context.Context, ttl time.Duration) (amboy.QueueGroup, queueGroupCloser, error) {
	mopts := MongoDBOptions{
		DB:  "amboy_test",
		URI: "mongodb://localhost:27017",
	}

	session, err := mgo.DialWithTimeout(mopts.URI, time.Second)
	if err != nil {
		return nil, func(_ context.Context) error { return nil }, err
	}

	closer := func(cctx context.Context) error {
		defer session.Close()
		return session.DB(mopts.DB).DropDatabase()
	}

	opts := RemoteQueueGroupOptions{
		DefaultWorkers: 1,
		Prefix:         "prefix",
		TTL:            ttl,
		PruneFrequency: ttl / 2,
	}

	if err = session.DB(mopts.DB).DropDatabase(); err != nil {
		return nil, closer, err
	}

	qg, err := NewMgoRemoteQueueGroup(ctx, opts, session, mopts)
	return qg, closer, err
}

func remoteLegacyQueueGroupMergedConstructor(ctx context.Context, ttl time.Duration) (amboy.QueueGroup, queueGroupCloser, error) {
	mopts := MongoDBOptions{
		DB:  "amboy_test",
		URI: "mongodb://localhost:27017",
	}

	session, err := mgo.DialWithTimeout(mopts.URI, time.Second)
	if err != nil {
		return nil, func(_ context.Context) error { return nil }, err
	}

	closer := func(cctx context.Context) error {
		defer session.Close()
		return session.DB(mopts.DB).DropDatabase()
	}

	if ttl == 0 {
		ttl = time.Hour
	}

	opts := RemoteQueueGroupOptions{
		DefaultWorkers: 2,
		Prefix:         "prefix",
		TTL:            ttl,
		PruneFrequency: ttl / 2,
	}

	if err = session.DB(mopts.DB).DropDatabase(); err != nil {
		return nil, closer, err
	}

	if err = session.DB(mopts.DB).DropDatabase(); err != nil {
		return nil, closer, err
	}

	qg, err := NewMgoRemoteSingleQueueGroup(ctx, opts, session, mopts)
	return qg, closer, err
}

func remoteQueueGroupMergedConstructor(ctx context.Context, ttl time.Duration) (amboy.QueueGroup, queueGroupCloser, error) {
	mopts := MongoDBOptions{
		DB:  "amboy_test",
		URI: "mongodb://localhost:27017",
	}

	client, err := mongo.NewClient(options.Client().ApplyURI(mopts.URI).SetConnectTimeout(time.Second))
	if err != nil {
		return nil, func(_ context.Context) error { return nil }, err
	}

	closer := func(cctx context.Context) error {
		catcher := grip.NewBasicCatcher()
		catcher.Add(client.Database(mopts.DB).Drop(cctx))
		catcher.Add(client.Disconnect(cctx))
		return catcher.Resolve()
	}
	if ttl == 0 {
		ttl = time.Hour
	}

	if err = client.Connect(ctx); err != nil {
		return nil, closer, err
	}
	opts := RemoteQueueGroupOptions{
		DefaultWorkers: 1,
		Prefix:         "prefix",
		TTL:            ttl,
		PruneFrequency: ttl,
	}

	if err = client.Database(mopts.DB).Drop(ctx); err != nil {
		return nil, closer, err
	}

	if err = client.Ping(ctx, nil); err != nil {
		return nil, closer, errors.Wrap(err, "server not pingable")
	}

	qg, err := NewMongoRemoteSingleQueueGroup(ctx, opts, client, mopts)
	return qg, closer, err
}

func TestQueueGroupOperations(t *testing.T) {
	queueGroups := map[string]queueGroupConstructor{
		"Local":           localQueueGroupConstructor,
		"Mongo":           remoteQueueGroupConstructor,
		"MongoMerged":     remoteQueueGroupMergedConstructor,
		"LegacyMgo":       remoteLegacyQueueGroupConstructor,
		"LegacyMgoMerged": remoteLegacyQueueGroupMergedConstructor,
	}

	for groupName, constructor := range queueGroups {
		t.Run(groupName, func(t *testing.T) {
			t.Run("Get", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				g, closer, err := constructor(ctx, 0)
				defer func() { require.NoError(t, closer(ctx)) }()
				require.NoError(t, err)
				require.NotNil(t, g)
				defer g.Close(ctx)

				q1, err := g.Get(ctx, "one")
				require.NoError(t, err)
				require.NotNil(t, q1)
				require.True(t, q1.Started())

				q2, err := g.Get(ctx, "two")
				require.NoError(t, err)
				require.NotNil(t, q2)
				require.True(t, q2.Started())

				j1 := job.NewShellJob("true", "")
				j2 := job.NewShellJob("true", "")
				j3 := job.NewShellJob("true", "")

				// Add j1 to q1. Add j2 and j3 to q2.
				require.NoError(t, q1.Put(j1))
				require.NoError(t, q2.Put(j2))
				require.NoError(t, q2.Put(j3))

				amboy.WaitCtxInterval(ctx, q1, 100*time.Millisecond)
				amboy.WaitCtxInterval(ctx, q2, 100*time.Millisecond)

				resultsQ1 := []amboy.Job{}
				for result := range q1.Results(ctx) {
					resultsQ1 = append(resultsQ1, result)
				}
				resultsQ2 := []amboy.Job{}
				for result := range q2.Results(ctx) {
					resultsQ2 = append(resultsQ2, result)
				}

				require.True(t, assert.Len(t, resultsQ1, 1, "first") && assert.Len(t, resultsQ2, 2, "second"))

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

				g, closer, err := constructor(ctx, 0)
				defer func() { require.NoError(t, closer(ctx)) }()

				require.NoError(t, err)
				require.NotNil(t, g)

				defer g.Close(ctx)

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

				amboy.WaitCtxInterval(ctx, q3, 10*time.Millisecond)
				amboy.WaitCtxInterval(ctx, q4, 10*time.Millisecond)

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

				g, closer, err := constructor(ctx, 0)
				defer func() { require.NoError(t, closer(ctx)) }()
				require.NoError(t, err)
				require.NotNil(t, g)
				defer g.Close(ctx)

				q1, err := g.Get(ctx, "five")
				require.NoError(t, err)
				require.NotNil(t, q1)

				q2, err := g.Get(ctx, "six")
				require.NoError(t, err)
				require.NotNil(t, q2)

				j1 := job.NewShellJob("true", "")
				j2 := job.NewShellJob("true", "")
				j3 := job.NewShellJob("true", "")

				// Add j1 to q1. Add j2 and j3 to q2.
				require.NoError(t, q1.Put(j1))
				require.NoError(t, q2.Put(j2))
				require.NoError(t, q2.Put(j3))

				amboy.WaitCtxInterval(ctx, q2, 10*time.Millisecond)
				amboy.WaitCtxInterval(ctx, q1, 10*time.Millisecond)

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

				require.NoError(t, g.Prune(ctx))

				// Try getting the queues again
				q1, err = g.Get(ctx, "five")
				require.NoError(t, err)
				require.NotNil(t, q1)

				q2, err = g.Get(ctx, "six")
				require.NoError(t, err)
				require.NotNil(t, q2)

				switch mg := g.(type) {
				case *remoteMongoQueueGroupSingle:
					// we should be tracking no
					// local queues
					assert.Len(t, mg.queues, 2)
					require.NoError(t, g.Prune(ctx))
					assert.Len(t, mg.queues, 0)
				case *remoteMgoQueueGroupSingle:
					assert.Len(t, mg.queues, 2)
					require.NoError(t, g.Prune(ctx))
					assert.Len(t, mg.queues, 0)
				default:
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
				}
			})
			t.Run("PruneWithTTL", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer cancel()

				g, closer, err := constructor(ctx, 5*time.Second)
				defer func() { require.NoError(t, closer(ctx)) }()
				require.NoError(t, err)
				require.NotNil(t, g)
				defer g.Close(ctx)

				q1, err := g.Get(ctx, "seven")
				require.NoError(t, err)
				require.NotNil(t, q1)

				q2, err := g.Get(ctx, "eight")
				require.NoError(t, err)
				require.NotNil(t, q2)

				j1 := job.NewShellJob("true", "")
				j2 := job.NewShellJob("true", "")
				j3 := job.NewShellJob("true", "")

				// Add j1 to q1. Add j2 and j3 to q2.
				require.NoError(t, q1.Put(j1))
				require.NoError(t, q2.Put(j2))
				require.NoError(t, q2.Put(j3))

				amboy.WaitCtxInterval(ctx, q1, 100*time.Millisecond)
				amboy.WaitCtxInterval(ctx, q2, 100*time.Millisecond)

				// Queues should have completed work
				stats1 := q1.Stats()
				require.Equal(t, 1, stats1.Total)
				assert.Zero(t, stats1.Running)
				assert.Equal(t, 1, stats1.Completed, stats1.String())
				assert.Zero(t, stats1.Pending)
				assert.Zero(t, stats1.Blocked)

				stats2 := q2.Stats()
				require.Equal(t, 2, stats2.Total)
				assert.Zero(t, stats2.Running)
				assert.Equal(t, 2, stats2.Completed, stats2.String())
				assert.Zero(t, stats2.Pending)
				assert.Zero(t, stats2.Blocked)

				time.Sleep(20 * time.Second)

				switch mg := g.(type) {
				case *remoteMongoQueueGroupSingle:
					assert.Len(t, mg.queues, 0)
				case *remoteMgoQueueGroupSingle:
					assert.Len(t, mg.queues, 0)
				}

				// Try getting the queues again
				q1, err = g.Get(ctx, "seven")
				require.NoError(t, err)
				require.NotNil(t, q1)

				q2, err = g.Get(ctx, "eight")
				require.NoError(t, err)
				require.NotNil(t, q2)

				switch mg := g.(type) {
				case *remoteMongoQueueGroupSingle:
					// we should be tracking no
					// local queues
					assert.Len(t, mg.queues, 2)
					require.NoError(t, g.Prune(ctx))
					assert.Len(t, mg.queues, 0)
				case *remoteMgoQueueGroupSingle:
					assert.Len(t, mg.queues, 2)
					require.NoError(t, g.Prune(ctx))
					assert.Len(t, mg.queues, 0)
				default:
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
				}
			})
			t.Run("Close", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()

				g, closer, err := constructor(ctx, 0)
				defer func() { require.NoError(t, closer(ctx)) }()
				require.NoError(t, err)
				require.NotNil(t, g)

				q1, err := g.Get(ctx, "nine")
				require.NoError(t, err)
				require.NotNil(t, q1)

				q2, err := g.Get(ctx, "ten")
				require.NoError(t, err)
				require.NotNil(t, q2)

				j1 := job.NewShellJob("true", "")
				j2 := job.NewShellJob("true", "")
				j3 := job.NewShellJob("true", "")

				// Add j1 to q1. Add j2 and j3 to q2.
				require.NoError(t, q1.Put(j1))
				require.NoError(t, q2.Put(j2))
				require.NoError(t, q2.Put(j3))

				amboy.WaitCtxInterval(ctx, q1, 10*time.Millisecond)
				amboy.WaitCtxInterval(ctx, q2, 10*time.Millisecond)

				g.Close(ctx)
			})
		})
	}
}

func TestQueueGroupConstructorPruneSmokeTest(t *testing.T) {
	mopts := MongoDBOptions{
		DB:  "amboy_test",
		URI: "mongodb://localhost:27017",
	}

	client, err := mongo.NewClient(options.Client().ApplyURI(mopts.URI).SetConnectTimeout(time.Second))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, client.Connect(ctx))
	for i := 0; i < 10; i++ {
		_, err = client.Database("amboy_test").Collection(fmt.Sprintf("gen-%d.jobs", i)).InsertOne(ctx, bson.M{"foo": "bar"})
		require.NoError(t, err)
	}
	remoteOpts := RemoteQueueGroupOptions{
		Prefix:         "gen",
		DefaultWorkers: 1,
		TTL:            time.Second,
		PruneFrequency: time.Second,
	}
	_, err = NewMongoRemoteQueueGroup(ctx, remoteOpts, client, mopts)
	require.NoError(t, err)
	time.Sleep(time.Second)
	for i := 0; i < 10; i++ {
		count, err := client.Database("amboy_test").Collection(fmt.Sprintf("gen-%d.jobs", i)).CountDocuments(ctx, bson.M{})
		require.NoError(t, err)
		require.Zero(t, count, fmt.Sprintf("gen-%d.jobs not dropped", i))
	}
}
