package cache

import (
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
// set to true if the value isn't empty.
func (c *DBCache) Get(key string) ([]byte, bool) {
	item := cacheItem{}
	err := db.FindOneQ(collection,
		db.Query(bson.M{IDKey: key}),
		&item,
	)
	if err != nil {
		if !adb.ResultsNotFound(err) {
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "getting cached value",
				"key":       key,
				"operation": "Get",
				"source":    "DBCache",
			}))
		}
		return nil, false
	}
	return item.Contents, true
}

// Set stores valueBytes for key.
func (c *DBCache) Set(key string, valueBytes []byte) {
	_, err := db.Upsert(
		collection,
		bson.M{IDKey: key},
		bson.M{
			"$set": bson.M{
				ContentsKey: valueBytes,
				UpdatedKey:  time.Now(),
			},
		},
	)

	grip.Error(message.WrapError(err, message.Fields{
		"message":   "setting cached value",
		"key":       key,
		"operation": "Set",
		"source":    "DBCache",
	}))
}

// Delete removes the value associated with the key.
func (c *DBCache) Delete(key string) {
	grip.Error(message.WrapError(
		db.Remove(collection, bson.M{IDKey: key}),
		message.Fields{
			"message":   "deleting cached value",
			"key":       key,
			"operation": "Delete",
			"source":    "DBCache",
		}))
}
