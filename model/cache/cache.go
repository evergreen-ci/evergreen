package cache

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const collection = "data_cache"

// DBCache stores and retrieves binary data in the database.
type DBCache struct{}

type cacheItem struct {
	ID       string    `bson:"_id"`
	Contents []byte    `bson:"contents"`
	Updated  time.Time `bson:"updated"`
}

var (
	IDKey       = bsonutil.MustHaveTag(cacheItem{}, "ID")
	ContentsKey = bsonutil.MustHaveTag(cacheItem{}, "Contents")
	UpdatedKey  = bsonutil.MustHaveTag(cacheItem{}, "Updated")
)

// Get returns the []byte representation of a cached value and a bool
// set to true if the value isn't empty. The cache is best-effort, so treat
// any failure as a cache miss rather than surfacing an error that would fail the caller.
func (c *DBCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	item := cacheItem{}
	err := db.FindOneQ(ctx, collection,
		db.Query(bson.M{IDKey: key}),
		&item,
	)
	if err != nil {
		if !adb.ResultsNotFound(err) {
			grip.Error(ctx, message.WrapError(err, message.Fields{
				"message":   "getting cached value",
				"key":       key,
				"operation": "Get",
				"source":    "DBCache",
			}))
		}
		return nil, false, nil
	}
	return item.Contents, true, nil
}

// Set stores valueBytes for key. The cache is best-effort, so a write failure
// is logged but not surfaced to the caller.
func (c *DBCache) Set(ctx context.Context, key string, valueBytes []byte) error {
	_, err := db.Upsert(
		ctx,
		collection,
		bson.M{IDKey: key},
		bson.M{
			"$set": bson.M{
				ContentsKey: valueBytes,
				UpdatedKey:  time.Now(),
			},
		},
	)

	grip.Error(ctx, message.WrapError(err, message.Fields{
		"message":   "setting cached value",
		"key":       key,
		"operation": "Set",
		"source":    "DBCache",
	}))

	return nil
}

// Delete removes the value associated with the key. The cache is best-effort,
// so a delete failure is logged but not surfaced to the caller.
func (c *DBCache) Delete(ctx context.Context, key string) error {
	err := db.Remove(ctx, collection, bson.M{IDKey: key})
	grip.Error(ctx, message.WrapError(err,
		message.Fields{
			"message":   "deleting cached value",
			"key":       key,
			"operation": "Delete",
			"source":    "DBCache",
		}))
	return nil
}
