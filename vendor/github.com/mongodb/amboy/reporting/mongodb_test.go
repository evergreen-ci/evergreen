package reporting

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestMongoDBConstructors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(time.Second))
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))

	t.Run("NilSessionShouldError", func(t *testing.T) {
		opts := queue.DefaultMongoDBOptions()
		opts.DB = "amboy_test"
		conf := DBQueueReporterOptions{Options: opts}

		db, err := MakeDBQueueState(ctx, conf, nil)
		assert.Error(t, err)
		assert.Nil(t, db)
	})
	t.Run("UnpingableSessionError", func(t *testing.T) {
		opts := queue.DefaultMongoDBOptions()
		opts.DB = "amboy_test"
		conf := DBQueueReporterOptions{Options: opts}

		db, err := MakeDBQueueState(ctx, conf, client)
		assert.Error(t, err)
		assert.Nil(t, db)
	})
	t.Run("BuildNewConnector", func(t *testing.T) {
		opts := queue.DefaultMongoDBOptions()
		opts.DB = "amboy_test"
		conf := DBQueueReporterOptions{Name: "foo", Options: opts}

		db, err := MakeDBQueueState(ctx, conf, client)
		assert.NoError(t, err)
		assert.NotNil(t, db)

		r, ok := db.(*dbQueueStat)
		require.True(t, ok)
		require.NotNil(t, r)
		assert.NotZero(t, r.collection)
	})
	t.Run("DialWithNewConstructor", func(t *testing.T) {
		opts := queue.DefaultMongoDBOptions()
		opts.DB = "amboy_test"
		conf := DBQueueReporterOptions{Name: "foo", Options: opts}

		r, err := NewDBQueueState(ctx, conf)
		assert.NoError(t, err)
		assert.NotNil(t, r)
	})
	t.Run("DialWithBadURI", func(t *testing.T) {
		opts := queue.DefaultMongoDBOptions()
		opts.DB = "amboy_test"
		opts.URI = "mongodb://lochost:26016"
		conf := DBQueueReporterOptions{Options: opts}

		r, err := NewDBQueueState(ctx, conf)
		assert.Error(t, err)
		assert.Nil(t, r)
	})
}
