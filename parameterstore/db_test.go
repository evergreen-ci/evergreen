package parameterstore

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	dbName = "mci_test"
	dbURL  = "mongodb://localhost:27017"
)

func initDB(ctx context.Context, t *testing.T) *mongo.Database {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(dbURL))
	if err != nil {
		require.FailNow(t, errors.Wrap(err, "connecting to the test database").Error())
	}

	db := client.Database(dbName)
	return db
}

func TestFind(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db := initDB(ctx, t)
	defer func() { require.NoError(t, db.Drop(ctx)) }()

	params := []parameter{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
	}
	for _, param := range params {
		_, err := db.Collection(Collection).InsertOne(ctx, param)
		require.NoError(t, err)
	}

	p := parameterStore{}
	p.opts.Database = db
	dbParams, err := p.find(ctx, []string{"a", "b", "c"})
	assert.NoError(t, err)
	assert.Len(t, dbParams, 3)
	sort.Slice(dbParams, func(i, j int) bool { return dbParams[i].ID < dbParams[j].ID })
	for i := range params {
		assert.Equal(t, params[i], dbParams[i])
	}
}

func TestSetLocalValue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db := initDB(ctx, t)
	defer func() { require.NoError(t, db.Drop(ctx)) }()

	param := parameter{ID: "a"}
	_, err := db.Collection(Collection).InsertOne(ctx, param)
	require.NoError(t, err)

	p := parameterStore{}
	p.opts.Database = db
	val := "val"
	assert.NoError(t, p.setLocalValue(ctx, param.ID, val))
	dbParams, err := p.find(ctx, []string{param.ID})
	assert.NoError(t, err)
	require.Len(t, dbParams, 1)
	assert.Equal(t, val, dbParams[0].Value)
	assert.True(t, param.LastUpdate.Equal(dbParams[0].LastUpdate))
}

func TestSetLastUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db := initDB(ctx, t)
	defer func() { require.NoError(t, db.Drop(ctx)) }()

	param := parameter{ID: "a", Value: "val"}
	_, err := db.Collection(Collection).InsertOne(ctx, param)
	require.NoError(t, err)

	p := parameterStore{}
	p.opts.Database = db
	now := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	assert.NoError(t, p.SetLastUpdate(ctx, param.ID, now))
	dbParams, err := p.find(ctx, []string{param.ID})
	assert.NoError(t, err)
	require.Len(t, dbParams, 1)
	assert.True(t, now.Equal(dbParams[0].LastUpdate))
	assert.Equal(t, param.Value, dbParams[0].Value)
}
