package ec2instancereferenceprice

import (
	"context"
	"regexp"

	"github.com/evergreen-ci/evergreen/db"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

// Collection is the MongoDB collection for EC2 reference median prices.
const Collection = "ec2_instance_reference_prices"

// FindOne runs a query against the reference price collection, returning one document.
func FindOne(ctx context.Context, query db.Q) (*EC2InstanceReferencePrice, error) {
	out := &EC2InstanceReferencePrice{}
	err := db.FindOneQ(ctx, Collection, query, out)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return out, err
}

// ByInstanceTypeAndOS returns a query for the natural key used by reference pricing.
// operatingSystem is matched case-insensitively so rows stay valid whether the collection
// stores Linux/linux, SUSE/suse, etc.
func ByInstanceTypeAndOS(instanceType, operatingSystem string) db.Q {
	pattern := "^" + regexp.QuoteMeta(operatingSystem) + "$"
	return db.Query(bson.D{
		{Key: InstanceTypeKey, Value: instanceType},
		{Key: OperatingSystemKey, Value: bson.D{
			{Key: "$regex", Value: pattern},
			{Key: "$options", Value: "i"},
		}},
	})
}
