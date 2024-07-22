package parameterstore

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	collection = "parameters"

	idKey         = bsonutil.MustHaveTag(parameter{}, "ID")
	lastUpdateKey = bsonutil.MustHaveTag(parameter{}, "LastUpdate")
	valueKey      = bsonutil.MustHaveTag(parameter{}, "Value")
)

func (p *parameterStore) find(ctx context.Context, names []string) ([]parameter, error) {
	cur, err := p.opts.Database.Collection(collection).Find(ctx, bson.M{idKey: bson.M{"$in": names}})
	if err != nil {
		return nil, errors.Wrap(err, "getting cursor for parameters")
	}
	var res []parameter
	if err := cur.All(ctx, &res); err != nil {
		return nil, errors.Wrap(err, "iterating parameter cursor")
	}
	return res, nil
}

func (p *parameterStore) setLocalValue(ctx context.Context, name, value string) error {
	_, err := p.opts.Database.Collection(collection).UpdateOne(
		ctx,
		bson.M{idKey: name},
		bson.M{"$set": bson.M{valueKey: value}}, options.Update().SetUpsert(true),
	)
	return errors.Wrapf(err, "setting value for '%s'", name)
}

// SetLastUpdate sets the time a parameter has last been updated in its backing data source.
func (p *parameterStore) SetLastUpdate(ctx context.Context, name string, updated time.Time) error {
	_, err := p.opts.Database.Collection(collection).UpdateOne(
		ctx,
		bson.M{idKey: name},
		bson.M{"$set": bson.M{lastUpdateKey: updated}}, options.Update().SetUpsert(true),
	)
	return errors.Wrapf(err, "setting updated time")
}
