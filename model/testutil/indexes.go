package testutil

import (
	"github.com/evergreen-ci/evergreen/db"
	"gopkg.in/mgo.v2"
)

// AddTestIndexes drops and adds indexes for a given collection
func AddTestIndexes(collection string, unique, sparse bool, key ...string) error {
	_ = db.DropIndex(collection, key...)
	return db.EnsureIndex(collection, mgo.Index{
		Key:    key,
		Unique: unique,
		Sparse: sparse,
	})
}
