package fakeparameter

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
)

// Collection is the name of the collection in the DB holding the fake
// parameters. This is for testing only.
const Collection = "fake_parameters"

var (
	IDKey          = bsonutil.MustHaveTag(FakeParameter{}, "ID")
	ValueKey       = bsonutil.MustHaveTag(FakeParameter{}, "Value")
	LastUpdatedKey = bsonutil.MustHaveTag(FakeParameter{}, "LastUpdated")
)

// FindOneID finds a single parameter by its name.
func FindOneID(ctx context.Context, id string) (*FakeParameter, error) {
	var p FakeParameter
	err := evergreen.GetEnvironment().DB().Collection(Collection).FindOne(ctx, bson.M{IDKey: id}).Decode(&p)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding parameter by ID")
	}
	return &p, nil
}
