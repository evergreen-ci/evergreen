package testutil

import (
	"strings"

	"github.com/evergreen-ci/evergreen/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AddTestIndexes adds indexes in a given collection for testing.
func AddTestIndexes(collection string, unique, sparse bool, key ...string) error {
	spec := bson.D{}
	for _, k := range key {
		if strings.HasPrefix(k, "-") {
			spec = append(spec, bson.E{Key: k[1:], Value: -1})
		} else {
			spec = append(spec, bson.E{Key: k, Value: 1})
		}
	}

	return db.EnsureIndex(collection, mongo.IndexModel{
		Keys:    spec,
		Options: options.Index().SetUnique(unique).SetSparse(sparse),
	})
}
