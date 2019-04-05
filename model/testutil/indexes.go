package testutil

import (
	"strings"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AddTestIndexes drops and adds indexes for a given collection
func AddTestIndexes(collection string, unique, sparse bool, key ...string) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(db.DropAllIndexes(collection))
	spec := bson.D{}
	for _, k := range key {
		if strings.HasPrefix(k, "-") {
			spec = append(spec, bson.E{Key: k[1:], Value: -1})
		} else {
			spec = append(spec, bson.E{Key: k, Value: 1})
		}
	}

	catcher.Add(db.EnsureIndex(collection, mongo.IndexModel{
		Keys:    spec,
		Options: options.Index().SetUnique(unique).SetSparse(sparse),
	}))
	return catcher.Resolve()
}
