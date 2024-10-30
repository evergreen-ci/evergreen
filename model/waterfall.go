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
	DefaultWaterfallQueryCount   = 20
	DefaultWaterfallVersionLimit = 5
	MaxWaterfallVersionLimit     = 300
)

type WaterfallTask struct {
	Id          string `bson:"_id" json:"_id"`
	DisplayName string `bson:"display_name" json:"display_name"`
	Execution   int    `bson:"execution" json:"execution"`
	Status      string `bson:"status" json:"status"`
}

type WaterfallBuild struct {
	Id          string          `bson:"_id" json:"_id"`
	Activated   bool            `bson:"activated" json:"activated"`
	DisplayName string          `bson:"display_name" json:"display_name"`
	Version     string          `bson:"version" json:"version"`
	Tasks       []WaterfallTask `bson:"tasks" json:"tasks"`
}

type WaterfallBuildVariant struct {
	Id          string           `bson:"_id" json:"_id"`
	DisplayName string           `bson:"display_name" json:"display_name"`
	Builds      []WaterfallBuild `bson:"builds" json:"builds"`
	Version     string           `bson:"version" json:"version"`
}

type WaterfallOptions struct {
	Limit      int      `bson:"-" json:"-"`
	MaxOrder   int      `bson:"-" json:"-"`
	MinOrder   int      `bson:"-" json:"-"`
	Requesters []string `bson:"-" json:"-"`
}

// GetActiveWaterfallVersions returns at most `opts.limit` activated versions for a given project.
func GetActiveWaterfallVersions(ctx context.Context, projectId string, opts WaterfallOptions) ([]Version, error) {
	invalidRequesters, _ := utility.StringSliceSymmetricDifference(opts.Requesters, evergreen.SystemVersionRequesterTypes)
	if len(invalidRequesters) > 0 {
		return nil, errors.Errorf("invalid requester(s) '%s'; only commit-level requesters can be applied to the waterfall query", invalidRequesters)
	}

	if opts.MaxOrder != 0 && opts.MinOrder != 0 {
		return nil, errors.New("cannot provide both max and min order options")
	}

	match := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey: bson.M{
			"$in": opts.Requesters,
		},
		VersionActivatedKey: true,
	}

	pagingForward := opts.MaxOrder != 0
	pagingBackward := opts.MinOrder != 0

	if pagingForward {
		match[VersionRevisionOrderNumberKey] = bson.M{"$lt": opts.MaxOrder}
	} else if pagingBackward {
		match[VersionRevisionOrderNumberKey] = bson.M{"$gt": opts.MinOrder}
	}

	pipeline := []bson.M{{"$match": match}}

	if pagingBackward {
		// When querying with a $gt param, sort ascending so we can take `limit` versions nearest to the MinOrder param
		pipeline = append(pipeline, bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: 1}})
		pipeline = append(pipeline, bson.M{"$limit": opts.Limit})
		// Then apply an acending sort so these versions are returned in the expected descending order
		pipeline = append(pipeline, bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}})

	} else {
		pipeline = append(pipeline, bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}})
		pipeline = append(pipeline, bson.M{"$limit": opts.Limit})

	}

	res := []Version{}
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(VersionCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating active versions")
	}
	if err = cursor.All(ctx, &res); err != nil {
		return nil, err
	}

	return res, nil
}

// GetAllWaterfallVersions returns all of a project's versions within an inclusive range of orders.
func GetAllWaterfallVersions(ctx context.Context, projectId string, minOrder int, maxOrder int) ([]Version, error) {
	if minOrder != 0 && maxOrder != 0 && minOrder >= maxOrder {
		return nil, errors.New("minOrder must be less than maxOrder")
	}
	match := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
	}

	hasOrderFilters := maxOrder != 0 || minOrder != 0
	if hasOrderFilters {
		revisionFilter := bson.M{}
		if minOrder != 0 {
			revisionFilter["$gte"] = minOrder
		}
		if maxOrder != 0 {
			revisionFilter["$lte"] = maxOrder
		}
		match[VersionRevisionOrderNumberKey] = revisionFilter
	}

	pipeline := []bson.M{{"$match": match}}
	pipeline = append(pipeline, bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}})

	res := []Version{}
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(VersionCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating versions")
	}
	if err = cursor.All(ctx, &res); err != nil {
		return nil, err
	}

	return res, nil
}

// GetWaterfallBuildVariants returns all build variants associated with the specified versions. Each build variant contains an array of builds sorted by revision and their tasks.
func GetWaterfallBuildVariants(ctx context.Context, versionIds []string) ([]WaterfallBuildVariant, error) {
	if len(versionIds) == 0 {
		return nil, errors.Errorf("no version IDs specified")
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
			{
				"$project": bson.M{
					task.IdKey:          1,
					task.StatusKey:      1,
					task.DisplayNameKey: 1,
					task.ExecutionKey:   1,
				},
			},
		},
		"as": bsonutil.GetDottedKeyName(buildsKey, build.TasksKey),
	},
	})
	// Sorting builds here guarantees a consistent order in the subsequent $group stage
	pipeline = append(pipeline, bson.M{"$sort": bson.M{bsonutil.GetDottedKeyName(buildsKey, build.RevisionOrderNumberKey): -1}})
	pipeline = append(pipeline, bson.M{
		"$group": bson.M{
			"_id": "$_id",
			buildsKey: bson.M{
				"$push": "$" + buildsKey,
			},
		},
	})
	pipeline = append(pipeline, bson.M{
		"$project": bson.M{
			build.VersionKey: bson.M{
				"$first": "$" + bsonutil.GetDottedKeyName(buildsKey, build.VersionKey),
			},
			build.DisplayNameKey: bson.M{
				"$first": "$" + bsonutil.GetDottedKeyName(buildsKey, build.DisplayNameKey),
			},
			buildsKey: 1,
		},
	})
	pipeline = append(pipeline, bson.M{"$sort": bson.M{build.DisplayNameKey: 1}})

	res := []WaterfallBuildVariant{}
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(VersionCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating versions")
	}
	if err = cursor.All(ctx, &res); err != nil {
		return nil, err
	}

	return res, nil
}

// GetNextRecentActiveWaterfallVersion returns the next recent active version on the waterfall, i.e. a newer
// activated version than the version with the given minOrder.
func GetNextRecentActiveWaterfallVersion(ctx context.Context, projectId string, minOrder int) (*Version, error) {
	match := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
		VersionRevisionOrderNumberKey: bson.M{
			"$gt": minOrder,
		},
		VersionActivatedKey: true,
	}
	pipeline := []bson.M{
		{"$match": match},
		{"$sort": bson.M{VersionRevisionOrderNumberKey: 1}},
		{"$limit": 1},
	}

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
	if len(res) == 0 {
		return nil, nil
	}
	return &res[0], nil
}
