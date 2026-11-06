package model

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

const VersionBatchTimeTasksCollection = "version_batchtime_tasks"

// VersionBatchTimeTasks stores the batchtime task activation statuses for a
// version's build variants. It lives in a dedicated collection because
// batchtime_tasks is the dominant contributor to a version document's size, so
// keeping it out of the version document keeps version writes cheap. A version
// document's build_variants_status still holds the per-variant fields; the two
// are combined by (Version).GetBuildVariants.
type VersionBatchTimeTasks struct {
	// Id is the version ID.
	Id string `bson:"_id"`
	// Identifier, Requester, Order, and Ignored are denormalized from the
	// version so that batchtime task activation can be queried without joining
	// against the versions collection.
	Identifier string `bson:"identifier"`
	Requester  string `bson:"r"`
	Order      int    `bson:"order"`
	Ignored    bool   `bson:"ignored,omitempty"`

	Variants []VersionVariantBatchTimeTasks `bson:"variants"`
}

// VersionVariantBatchTimeTasks holds the batchtime task activation statuses for
// a single build variant.
type VersionVariantBatchTimeTasks struct {
	BuildVariant   string                `bson:"build_variant"`
	BatchTimeTasks []BatchTimeTaskStatus `bson:"batchtime_tasks,omitempty"`
}

var (
	VersionBatchTimeTasksIdKey         = bsonutil.MustHaveTag(VersionBatchTimeTasks{}, "Id")
	VersionBatchTimeTasksIdentifierKey = bsonutil.MustHaveTag(VersionBatchTimeTasks{}, "Identifier")
	VersionBatchTimeTasksRequesterKey  = bsonutil.MustHaveTag(VersionBatchTimeTasks{}, "Requester")
	VersionBatchTimeTasksOrderKey      = bsonutil.MustHaveTag(VersionBatchTimeTasks{}, "Order")
	VersionBatchTimeTasksIgnoredKey    = bsonutil.MustHaveTag(VersionBatchTimeTasks{}, "Ignored")
	VersionBatchTimeTasksVariantsKey   = bsonutil.MustHaveTag(VersionBatchTimeTasks{}, "Variants")

	VersionVariantBatchTimeTasksVariantKey = bsonutil.MustHaveTag(VersionVariantBatchTimeTasks{}, "BuildVariant")
	VersionVariantBatchTimeTasksTasksKey   = bsonutil.MustHaveTag(VersionVariantBatchTimeTasks{}, "BatchTimeTasks")
)

// VersionBatchTimeTasksFindOne returns the batchtime tasks doc for a version, or
// nil if the version has none.
func VersionBatchTimeTasksFindOne(ctx context.Context, versionID string) (*VersionBatchTimeTasks, error) {
	doc := &VersionBatchTimeTasks{}
	err := db.FindOneQ(ctx, VersionBatchTimeTasksCollection, db.Query(bson.M{VersionBatchTimeTasksIdKey: versionID}), doc)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// VersionBatchTimeTasksSet replaces the batchtime tasks doc for a version,
// inserting it if it doesn't exist.
func VersionBatchTimeTasksSet(ctx context.Context, doc *VersionBatchTimeTasks) error {
	update := bson.M{
		"$set": bson.M{
			VersionBatchTimeTasksIdentifierKey: doc.Identifier,
			VersionBatchTimeTasksRequesterKey:  doc.Requester,
			VersionBatchTimeTasksOrderKey:      doc.Order,
			VersionBatchTimeTasksIgnoredKey:    doc.Ignored,
			VersionBatchTimeTasksVariantsKey:   doc.Variants,
		},
	}
	_, err := db.Upsert(ctx, VersionBatchTimeTasksCollection, bson.M{VersionBatchTimeTasksIdKey: doc.Id}, update)
	return err
}

// VersionBatchTimeTasksPushVariants appends the given variants' batchtime tasks
// to a version's doc, creating the doc if it doesn't exist.
func VersionBatchTimeTasksPushVariants(ctx context.Context, doc *VersionBatchTimeTasks) error {
	update := bson.M{
		"$push": bson.M{
			VersionBatchTimeTasksVariantsKey: bson.M{"$each": doc.Variants},
		},
		"$setOnInsert": bson.M{
			VersionBatchTimeTasksIdentifierKey: doc.Identifier,
			VersionBatchTimeTasksRequesterKey:  doc.Requester,
			VersionBatchTimeTasksOrderKey:      doc.Order,
			VersionBatchTimeTasksIgnoredKey:    doc.Ignored,
		},
	}
	_, err := db.Upsert(ctx, VersionBatchTimeTasksCollection, bson.M{VersionBatchTimeTasksIdKey: doc.Id}, update)
	return err
}

// VersionBatchTimeTasksFindByLastTaskActivation returns the most recent version
// whose batchtime tasks include an activated task matching the variant and task
// name, or nil if none is found.
func VersionBatchTimeTasksFindByLastTaskActivation(ctx context.Context, projectID, variant, taskName string) (*VersionBatchTimeTasks, error) {
	q := db.Query(bson.M{
		VersionBatchTimeTasksIdentifierKey: projectID,
		VersionBatchTimeTasksIgnoredKey:    bson.M{"$ne": true},
		VersionBatchTimeTasksRequesterKey:  bson.M{"$in": evergreen.SystemVersionRequesterTypes},
		VersionBatchTimeTasksVariantsKey: bson.M{"$elemMatch": bson.M{
			VersionVariantBatchTimeTasksVariantKey: variant,
			VersionVariantBatchTimeTasksTasksKey: bson.M{"$elemMatch": bson.M{
				BatchTimeTaskStatusTaskNameKey:  taskName,
				BatchTimeTaskStatusActivatedKey: true,
			}},
		}},
	}).Sort([]string{"-" + VersionBatchTimeTasksOrderKey})

	doc := &VersionBatchTimeTasks{}
	err := db.FindOneQ(ctx, VersionBatchTimeTasksCollection, q, doc)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return doc, nil
}
