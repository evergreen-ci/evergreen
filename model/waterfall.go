package model

import (
	"context"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/attribute"
)

const (
	buildsKey                    = "builds"
	DefaultWaterfallQueryCount   = 20
	DefaultWaterfallVersionLimit = 5
	MaxWaterfallVersionLimit     = 300
)

type WaterfallTask struct {
	Id                 string `bson:"_id" json:"_id"`
	DisplayName        string `bson:"display_name" json:"display_name"`
	DisplayStatusCache string `bson:"display_status_cache" json:"display_status_cache"`
	Execution          int    `bson:"execution" json:"execution"`
	Status             string `bson:"status" json:"status"`
}

type WaterfallBuild struct {
	Id           string          `bson:"_id" json:"_id"`
	Activated    bool            `bson:"activated" json:"activated"`
	BuildVariant string          `bson:"build_variant" json:"build_variant"`
	DisplayName  string          `bson:"display_name" json:"display_name"`
	Version      string          `bson:"version" json:"version"`
	Tasks        []WaterfallTask `bson:"tasks" json:"tasks"`
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
	Variants   []string `bson:"-" json:"-"`
}

// Older versions don't have their build display names saved in the version document.
// For those missing it, look up their build details.
// TODO DEVPROD-15118: This function can be removed after 7 February 2026, when the version TTL index applies and all versions include build display names.
func getBuildDisplayNames(match bson.M) bson.M {
	match[bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusDisplayNameKey)] = bson.M{"$exists": false}
	return bson.M{
		"$unionWith": bson.M{
			"coll": VersionCollection,
			"pipeline": []bson.M{
				bson.M{"$match": match},
				bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}},
				bson.M{"$limit": MaxWaterfallVersionLimit},
				bson.M{
					"$lookup": bson.M{
						"from":         build.Collection,
						"localField":   bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusIdKey),
						"foreignField": build.IdKey,
						"as":           VersionBuildVariantsKey,
						"pipeline": []bson.M{
							bson.M{
								"$project": bson.M{
									build.DisplayNameKey:           1,
									VersionBuildStatusActivatedKey: 1,
									VersionBuildStatusIdKey:        build.IdKey,
									build.BuildVariantKey:          1,
								},
							},
						},
					},
				},
			},
		},
	}
}

// This pipeline matches on versions that have a build with an ID or display name that matches variants
func getBuildVariantFilterPipeline(ctx context.Context, variants []string, match bson.M, projectId string) ([]bson.M, error) {
	pipeline := []bson.M{}
	match[bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusDisplayNameKey)] = bson.M{"$exists": true}
	pipeline = append(pipeline, bson.M{"$match": match})
	matchCopy := bson.M{}
	for key := range match {
		matchCopy[key] = match[key]
	}

	mostRecentVersion, err := GetMostRecentWaterfallVersion(ctx, projectId)
	if err != nil {
		return []bson.M{}, errors.Wrap(err, "getting most recent version")
	}

	// TODO DEVPROD-15118: Delete conditional getBuildDisplayNames check

	searchOrder := max(mostRecentVersion.RevisionOrderNumber-MaxWaterfallVersionLimit, 1)
	lastSearchableVersion, err := VersionFindOne(VersionByProjectIdAndOrder(mostRecentVersion.Identifier, searchOrder))
	if err != nil {
		return []bson.M{}, errors.Wrap(err, "fetching version")
	}

	buildVariantStatusDate := time.Date(2025, time.February, 7, 0, 0, 0, 0, time.UTC)
	if lastSearchableVersion != nil && lastSearchableVersion.CreateTime.Before(buildVariantStatusDate) {
		pipeline = append(pipeline, getBuildDisplayNames(matchCopy))
	}
	pipeline = append(pipeline, bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}})

	variantsAsRegex := strings.Join(variants, "|")
	pipeline = append(pipeline, bson.M{
		"$match": bson.M{
			VersionBuildVariantsKey: bson.M{
				"$elemMatch": bson.M{
					VersionBuildStatusActivatedKey: true,
					"$or": []bson.M{
						bson.M{VersionBuildStatusVariantKey: bson.M{"$regex": variantsAsRegex, "$options": "i"}},
						bson.M{VersionBuildStatusDisplayNameKey: bson.M{"$regex": variantsAsRegex, "$options": "i"}},
					},
				},
			},
		},
	})
	return pipeline, nil
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

	pipeline := []bson.M{}

	if len(opts.Variants) > 0 {
		buildVariantPipeline, err := getBuildVariantFilterPipeline(ctx, opts.Variants, match, projectId)
		if err != nil {
			return nil, errors.Wrap(err, "creating build variant filter pipeline")
		}
		pipeline = append(pipeline, buildVariantPipeline...)
	} else {
		pipeline = append(pipeline, bson.M{"$match": match})
	}

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
	pipeline = append(pipeline, bson.M{"$limit": MaxWaterfallVersionLimit})

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

func getVersionTasksPipeline() []bson.M {
	return []bson.M{
		bson.M{
			"$lookup": bson.M{
				"from":         build.Collection,
				"localField":   buildsKey,
				"foreignField": build.IdKey,
				"as":           buildsKey,
			},
		},
		bson.M{
			"$unwind": bson.M{
				"path": "$" + buildsKey,
			},
		},
		// Join all tasks that appear in the build's task cache and overwrite the list of task IDs with partial task documents
		bson.M{
			"$lookup": bson.M{
				"from":         task.Collection,
				"localField":   bsonutil.GetDottedKeyName(buildsKey, build.TasksKey, build.TaskCacheIdKey),
				"foreignField": task.IdKey,
				"pipeline": []bson.M{
					{
						"$match": bson.M{
							task.RequesterKey: bson.M{
								"$in": evergreen.SystemVersionRequesterTypes,
							},
						},
					},
					// The following projection should exactly match the index on the tasks collection in order to function as a covered query
					{
						"$project": bson.M{
							task.IdKey:                 1,
							task.DisplayNameKey:        1,
							task.DisplayStatusCacheKey: 1,
							task.ExecutionKey:          1,
							task.StatusKey:             1,
						},
					},
					{
						"$sort": bson.M{task.DisplayNameKey: 1},
					},
				},
				"as": bsonutil.GetDottedKeyName(buildsKey, build.TasksKey),
			},
		},
	}
}

// GetWaterfallBuildVariants returns all build variants associated with the specified versions. Each build variant contains an array of builds sorted by revision and their tasks.
func GetWaterfallBuildVariants(ctx context.Context, versionIds []string) ([]WaterfallBuildVariant, error) {
	ctx = utility.ContextWithAttributes(ctx, []attribute.KeyValue{attribute.String(evergreen.AggregationNameOtelAttribute, "GetWaterfallBuildVariants")})

	if len(versionIds) == 0 {
		return nil, errors.Errorf("no version IDs specified")
	}

	pipeline := []bson.M{{"$match": bson.M{VersionIdKey: bson.M{"$in": versionIds}}}}
	pipeline = append(pipeline, bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}})

	pipeline = append(pipeline, bson.M{"$unwind": bson.M{"path": "$" + VersionBuildVariantsKey}})
	pipeline = append(pipeline, bson.M{
		"$group": bson.M{
			"_id": "$" + bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusVariantKey),
			VersionBuildIdsKey: bson.M{
				"$push": "$" + bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusIdKey),
			},
		},
	})
	pipeline = append(pipeline, getVersionTasksPipeline()...)
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
			build.BuildVariantKey: bson.M{
				"$first": "$" + bsonutil.GetDottedKeyName(buildsKey, build.BuildVariantKey),
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

// GetVersionBuilds returns a list of builds with populated tasks for a given version.
func GetVersionBuilds(ctx context.Context, versionId string) ([]WaterfallBuild, error) {
	ctx = utility.ContextWithAttributes(ctx, []attribute.KeyValue{attribute.String(evergreen.AggregationNameOtelAttribute, "GetVersionBuilds")})

	pipeline := []bson.M{{"$match": bson.M{VersionIdKey: versionId}}}
	pipeline = append(pipeline, getVersionTasksPipeline()...)
	pipeline = append(pipeline, bson.M{
		"$replaceRoot": bson.M{
			"newRoot": "$" + buildsKey,
		},
	})
	pipeline = append(pipeline, bson.M{
		"$sort": bson.M{
			build.DisplayNameKey: 1,
		},
	})

	res := []WaterfallBuild{}
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(VersionCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating version builds")
	}
	if err = cursor.All(ctx, &res); err != nil {
		return nil, err
	}

	return res, nil
}

// GetNewerActiveWaterfallVersion returns the next newer active version on the waterfall, i.e. a more
// recent activated version than the current version.
func GetNewerActiveWaterfallVersion(ctx context.Context, projectId string, version Version) (*Version, error) {
	match := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
		VersionRevisionOrderNumberKey: bson.M{
			"$gt": version.RevisionOrderNumber,
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

// GetOlderActiveWaterfallVersion returns the next older active version on the waterfall, i.e. an older
// activated version than the current version.
func GetOlderActiveWaterfallVersion(ctx context.Context, projectId string, version Version) (*Version, error) {
	match := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
		VersionRevisionOrderNumberKey: bson.M{
			"$lt": version.RevisionOrderNumber,
		},
		VersionActivatedKey: true,
	}
	pipeline := []bson.M{
		{"$match": match},
		{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}},
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
