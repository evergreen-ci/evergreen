package model

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

const VersionTaskBatchTimeCollection = "version_task_batchtimes"

// VersionTaskBatchTimes stores the task batchtime activation statuses.
// Older versions used to store this information in the version document itself,
// however it had to be moved out due to performance issues.
type VersionTaskBatchTimes struct {
	// Id is the version ID.
	Id string `bson:"_id"`
	// The following are fields inherited from the version.
	Identifier string `bson:"identifier"`
	Requester  string `bson:"r"`
	Order      int    `bson:"order"`
	Ignored    bool   `bson:"ignored,omitempty"`

	BuildVariants []BuildVariantTaskBatchTime `bson:"build_variants"`
}

// BuildVariantTaskBatchTime holds the task-level batchtime activation statuses
// for a single build variant.
type BuildVariantTaskBatchTime struct {
	BuildVariant   string                `bson:"build_variant"`
	BatchTimeTasks []BatchTimeTaskStatus `bson:"batchtime_tasks,omitempty"`
}

var (
	VersionTaskBatchTimesIdKey            = bsonutil.MustHaveTag(VersionTaskBatchTimes{}, "Id")
	VersionTaskBatchTimesIdentifierKey    = bsonutil.MustHaveTag(VersionTaskBatchTimes{}, "Identifier")
	VersionTaskBatchTimesRequesterKey     = bsonutil.MustHaveTag(VersionTaskBatchTimes{}, "Requester")
	VersionTaskBatchTimesOrderKey         = bsonutil.MustHaveTag(VersionTaskBatchTimes{}, "Order")
	VersionTaskBatchTimesIgnoredKey       = bsonutil.MustHaveTag(VersionTaskBatchTimes{}, "Ignored")
	VersionTaskBatchTimesBuildVariantsKey = bsonutil.MustHaveTag(VersionTaskBatchTimes{}, "BuildVariants")

	BuildVariantTaskBatchTimeBuildVariantKey = bsonutil.MustHaveTag(BuildVariantTaskBatchTime{}, "BuildVariant")
	BuildVariantTaskBatchTimeTasksKey        = bsonutil.MustHaveTag(BuildVariantTaskBatchTime{}, "BatchTimeTasks")
)

// VersionTaskBatchTimesFindOne returns the task batchtimes for a version, or
// nil if the version has none.
func VersionTaskBatchTimesFindOne(ctx context.Context, versionID string) (*VersionTaskBatchTimes, error) {
	bt := &VersionTaskBatchTimes{}
	err := db.FindOneQ(ctx, VersionTaskBatchTimeCollection, db.Query(bson.M{VersionTaskBatchTimesIdKey: versionID}), bt)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return bt, nil
}

// VersionTaskBatchTimesUpsert upserts the task batchtimes for a version.
func VersionTaskBatchTimesUpsert(ctx context.Context, bt *VersionTaskBatchTimes) error {
	update := bson.M{
		"$set": bson.M{
			VersionTaskBatchTimesIdentifierKey:    bt.Identifier,
			VersionTaskBatchTimesRequesterKey:     bt.Requester,
			VersionTaskBatchTimesOrderKey:         bt.Order,
			VersionTaskBatchTimesIgnoredKey:       bt.Ignored,
			VersionTaskBatchTimesBuildVariantsKey: bt.BuildVariants,
		},
	}
	_, err := db.Upsert(ctx, VersionTaskBatchTimeCollection, bson.M{VersionTaskBatchTimesIdKey: bt.Id}, update)
	return err
}

// VersionTaskBatchTimesFindByLastTaskActivation returns the most recent version
// whose task batchtimes include an activated task matching the build variant
// and task name, or nil if none is found.
func VersionTaskBatchTimesFindByLastTaskActivation(ctx context.Context, projectID, bvName, taskName string) (*VersionTaskBatchTimes, error) {
	q := db.Query(bson.M{
		VersionTaskBatchTimesIdentifierKey: projectID,
		VersionTaskBatchTimesIgnoredKey:    bson.M{"$ne": true},
		VersionTaskBatchTimesRequesterKey:  bson.M{"$in": evergreen.SystemVersionRequesterTypes},
		VersionTaskBatchTimesBuildVariantsKey: bson.M{"$elemMatch": bson.M{
			BuildVariantTaskBatchTimeBuildVariantKey: bvName,
			BuildVariantTaskBatchTimeTasksKey: bson.M{"$elemMatch": bson.M{
				BatchTimeTaskStatusTaskNameKey:  taskName,
				BatchTimeTaskStatusActivatedKey: true,
			}},
		}},
	}).Sort([]string{"-" + VersionTaskBatchTimesOrderKey})

	bt := &VersionTaskBatchTimes{}
	err := db.FindOneQ(ctx, VersionTaskBatchTimeCollection, q, bt)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return bt, nil
}
