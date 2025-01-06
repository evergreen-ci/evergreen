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
	NameKey        = bsonutil.MustHaveTag(FakeParameter{}, "Name")
	ValueKey       = bsonutil.MustHaveTag(FakeParameter{}, "Value")
	LastUpdatedKey = bsonutil.MustHaveTag(FakeParameter{}, "LastUpdated")
)

// FindOneID finds a single fake parameter by its name.
func FindOneID(ctx context.Context, id string) (*FakeParameter, error) {
	checkTestingEnvironment()

	var p FakeParameter
	err := evergreen.GetEnvironment().DB().Collection(Collection).FindOne(ctx, bson.M{NameKey: id}).Decode(&p)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "finding parameter '%s'", id)
	}
	return &p, nil
}

// FindByIDs finds one or more fake parameters by their names.
func FindByIDs(ctx context.Context, ids ...string) ([]FakeParameter, error) {
	checkTestingEnvironment()

	if len(ids) == 0 {
		return nil, nil
	}
	params := []FakeParameter{}
	cur, err := evergreen.GetEnvironment().DB().Collection(Collection).Find(ctx, bson.M{NameKey: bson.M{"$in": ids}})
	if err != nil {
		return nil, errors.Wrap(err, "finding fake parameters by IDs")
	}
	if err := cur.All(ctx, &params); err != nil {
		return nil, errors.Wrap(err, "decoding fake parameters")
	}
	return params, nil
}

// DeleteOneID deletes a fake parameter with the given ID. Returns whether any
// matching parameter was deleted.
func DeleteOneID(ctx context.Context, id string) (bool, error) {
	checkTestingEnvironment()

	res, err := evergreen.GetEnvironment().DB().Collection(Collection).DeleteOne(ctx, bson.M{NameKey: id})
	if err != nil {
		return false, errors.Wrap(err, "deleting fake parameter by ID")
	}
	return res.DeletedCount > 0, nil
}
