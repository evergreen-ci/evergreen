package model

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	DefaultWaterfallVersionLimit = 5
	MaxWaterfallVersionLimit     = 300
)

type WaterfallTask struct {
	Id          string `bson:"_id" json:"_id"`
	DisplayName string `bson:"display_name" json:"display_name,omitempty"`
	Status      string `bson:"status" json:"status,omitempty"`
}

type WaterfallBuild struct {
	Id          string          `bson:"_id" json:"_id"`
	DisplayName string          `bson:"display_name" json:"display_name,omitempty"`
	Version     string          `bson:"version" json:"version,omitempty"`
	Tasks       []WaterfallTask `bson:"tasks" json:"tasks,omitempty"`
}

type WaterfallBuildVariant struct {
	Id          string           `bson:"_id" json:"_id"`
	DisplayName string           `bson:"display_name" json:"display_name,omitempty"`
	Builds      []WaterfallBuild `bson:"builds" json:"builds,omitempty"`
}

type WaterfallOptions struct {
	Limit      int
	Requesters []string
}

func GetWaterfallVersions(ctx context.Context, projectId string, opts WaterfallOptions) ([]Version, error) {
	invalidRequesters, _ := utility.StringSliceSymmetricDifference(opts.Requesters, evergreen.SystemVersionRequesterTypes)
	if len(invalidRequesters) > 0 {
		return nil, errors.Errorf("invalid requesters '%s'", invalidRequesters)
	}
	match := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey: bson.M{
			"$in": opts.Requesters,
		},
	}

	// TODO DEVPROD-10177: Add revision order logic to handle pagination.

	pipeline := []bson.M{{"$match": match}}
	pipeline = append(pipeline, bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}})
	limit := defaultVersionLimit
	if opts.Limit != 0 {
		limit = opts.Limit
	}

	pipeline = append(pipeline, bson.M{"$limit": limit})

	res := []Version{}
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(VersionCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating versions")
	}
	err = cursor.All(ctx, &res)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func GetWaterfallBuildVariants(ctx context.Context, projectId string, versions []Version) ([]WaterfallBuildVariant, error) {
	versionIds := []string{}
	for _, version := range versions {
		versionIds = append(versionIds, version.Id)
	}

	pipeline := []bson.M{{"$match": bson.M{VersionIdKey: bson.M{"$in": versionIds}}}}
	pipeline = append(pipeline, bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}})

	pipeline = append(pipeline, bson.M{"$unwind": bson.M{"path": "$" + VersionBuildVariantsKey}})
	buildsKey := "builds"
	pipeline = append(pipeline, bson.M{
		"$group": bson.M{
			"_id": "$" + bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusVariantKey),
			buildsKey: bson.M{
				"$push": "$" + bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusIdKey),
			},
		},
	})
	pipeline = append(pipeline, bson.M{
		// TODO DEVPROD-10178: Should be able to filter on build variant ID/name here.
		"$lookup": bson.M{
			"from":         build.Collection,
			"localField":   buildsKey,
			"foreignField": build.IdKey,
			"as":           buildsKey,
		},
	})
	pipeline = append(pipeline, bson.M{"$unwind": bson.M{"path": "$" + buildsKey}})

	// Join all tasks that appear in the build's task cache and overwrite the list of task IDs with partial task documents
	pipeline = append(pipeline, bson.M{"$lookup": bson.M{
		"from":         task.Collection,
		"localField":   bsonutil.GetDottedKeyName(buildsKey, build.TasksKey, build.TaskCacheIdKey),
		"foreignField": task.IdKey,
		"pipeline": []bson.M{
			{
				"$sort": bson.M{task.IdKey: 1},
			},
			// TODO DEVPROD-10179, DEVPROD-10180: Should be able to filter on the returned task names and statuses here.
			{
				"$project": bson.M{
					task.IdKey:          1,
					task.StatusKey:      1,
					task.DisplayNameKey: 1,
				},
			},
		},
		"as": bsonutil.GetDottedKeyName(buildsKey, build.TasksKey),
	},
	})
	// Sorting builds here guarantees a consistent order in the subsequent $group stage
	pipeline = append(pipeline, bson.M{"$sort": bson.M{"_id": 1, bsonutil.GetDottedKeyName(buildsKey, build.RevisionOrderNumberKey): -1}})
	pipeline = append(pipeline, bson.M{
		"$group": bson.M{
			"_id": "$_id",
			buildsKey: bson.M{
				"$push": "$" + buildsKey,
			},
		},
	})
	pipeline = append(pipeline, bson.M{"$sort": bson.M{"_id": 1}})
	pipeline = append(pipeline, bson.M{
		"$project": bson.M{
			build.DisplayNameKey: bson.M{
				"$first": "$" + bsonutil.GetDottedKeyName(buildsKey, build.DisplayNameKey),
			},
			buildsKey: 1,
		},
	})

	res := []WaterfallBuildVariant{}
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(VersionCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating versions")
	}
	err = cursor.All(ctx, &res)
	if err != nil {
		return nil, err
	}

	return res, nil
}
