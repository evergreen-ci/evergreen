package fakeparameter

import (
	"context"
	"flag"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ExecutionEnvironmentType is the type of environment in which the code is
// running. This exists a safety mechanism against accidentally calling this
// logic in non-testing environment. For tests, this should always be overridden
// to "test".
var ExecutionEnvironmentType = "production"

func init() {
	if ExecutionEnvironmentType != "test" {
		grip.EmergencyFatal(message.Fields{
			"message":     "fake Parameter Store testing code called in a non-testing environment",
			"environment": ExecutionEnvironmentType,
			"args":        flag.Args(),
		})
	}
}

// FakeParameter is the data model for a fake parameter stored in the DB. This
// is for testing only.
type FakeParameter struct {
	// Name is the unique identifying name for the parameter.
	Name string `bson:"_id,omitempty"`
	// Value is the parameter value.
	Value string `bson:"value,omitempty"`
	// LastUpdated is the last time the parameter was updated.
	LastUpdated time.Time `bson:"last_updated,omitempty"`
}

// Insert inserts a single parameter into the fake parameter store.
func (p *FakeParameter) Insert(ctx context.Context) error {
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).InsertOne(ctx, p)
	return err
}

// Upsert inserts a single parameter into the fake parameter store or replaces
// an one if one with the same ID already exists.
func (p *FakeParameter) Upsert(ctx context.Context) error {
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).ReplaceOne(ctx, bson.M{NameKey: p.Name}, p, options.Replace().SetUpsert(true))
	return err
}
