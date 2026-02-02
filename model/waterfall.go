package model

import (
	"context"
	"fmt"
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
	Limit                int      `bson:"-" json:"-"`
	MaxOrder             int      `bson:"-" json:"-"`
	MinOrder             int      `bson:"-" json:"-"`
	OmitInactiveBuilds   bool     `bson:"-" json:"-"`
	Requesters           []string `bson:"-" json:"-"`
	Statuses             []string `bson:"-" json:"-"`
	Tasks                []string `bson:"-" json:"-"`
	TaskCaseSensitive    bool     `bson:"-" json:"-"`
	Variants             []string `bson:"-" json:"-"`
	VariantCaseSensitive bool     `bson:"-" json:"-"`
}

const (
	minRevisionLength = 7
)

// Older versions don't have their build display names saved in the version document.
// For those missing it, look up their build details.
// TODO DEVPROD-15118: This function can be removed after 7 February 2026, when the version TTL index applies and all versions include build display names.
func getBuildDisplayNames(match bson.M) bson.M {
	match[bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusDisplayNameKey)] = bson.M{"$exists": false}
	return bson.M{
		"$unionWith": bson.M{
			"coll": VersionCollection,
			"pipeline": []bson.M{
				{"$match": match},
				{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}},
				{"$limit": MaxWaterfallVersionLimit},
				{
					"$lookup": bson.M{
						"from":         build.Collection,
						"localField":   bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusIdKey),
						"foreignField": build.IdKey,
						"as":           VersionBuildVariantsKey,
						"pipeline": []bson.M{
							{
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
// It checks if the version with order number versionSearchCutoff is old enough to require a join with the builds collection in order to obtain build variant display names.
func getBuildVariantFilterPipeline(ctx context.Context, variants []string, caseSensitive bool, match bson.M, projectId string, versionSearchCutoff int, omitInactiveBuilds bool) ([]bson.M, error) {
	pipeline := []bson.M{}
	match[bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusDisplayNameKey)] = bson.M{"$exists": true}
	pipeline = append(pipeline, bson.M{"$match": match})
	matchCopy := bson.M{}
	for key := range match {
		matchCopy[key] = match[key]
	}

	// TODO DEVPROD-15118: Delete conditional getBuildDisplayNames check
	searchOrder := max(versionSearchCutoff, 1)
	lastSearchableVersion, err := VersionFindOne(ctx, VersionByProjectIdAndOrder(projectId, searchOrder).WithFields(VersionCreateTimeKey))
	if err != nil {
		return []bson.M{}, errors.Wrap(err, "fetching version")
	}

	buildVariantStatusDate := time.Date(2025, time.February, 7, 0, 0, 0, 0, time.UTC)
	if lastSearchableVersion != nil && lastSearchableVersion.CreateTime.Before(buildVariantStatusDate) {
		// Add display names to Version.BuildVariants array.
		pipeline = append(pipeline, getBuildDisplayNames(matchCopy))
	}
	pipeline = append(pipeline, bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}})

	variantsAsRegex := strings.Join(variants, "|")

	var buildVariantMatch []bson.M
	if caseSensitive {
		buildVariantMatch = []bson.M{
			{VersionBuildStatusVariantKey: bson.M{"$regex": variantsAsRegex}},
			{VersionBuildStatusDisplayNameKey: bson.M{"$regex": variantsAsRegex}},
		}
	} else {
		buildVariantMatch = []bson.M{
			{VersionBuildStatusVariantKey: bson.M{"$regex": variantsAsRegex, "$options": "i"}},
			{VersionBuildStatusDisplayNameKey: bson.M{"$regex": variantsAsRegex, "$options": "i"}},
		}
	}

	buildsElemMatch := bson.M{
		"$or": buildVariantMatch,
	}

	if omitInactiveBuilds {
		buildsElemMatch[VersionBuildStatusActivatedKey] = true
	}

	pipeline = append(pipeline, bson.M{
		"$match": bson.M{
			VersionBuildVariantsKey: bson.M{
				"$elemMatch": buildsElemMatch,
			},
		},
	})
	return pipeline, nil
}

// GetActiveVersionsByTaskFilters returns limit versions that satisfy a task name or status filter. It also applies any requester and build variant filters.
// If neither of these filters is specified, use GetActiveWaterfallVersions: it's faster.
func GetActiveVersionsByTaskFilters(ctx context.Context, projectId string, opts WaterfallOptions, searchOffset int) ([]Version, error) {
	match := bson.M{
		task.ProjectKey: projectId,
		task.RequesterKey: bson.M{
			"$in": opts.Requesters,
		},
		task.ActivatedKey: true,
	}

	if opts.MaxOrder != 0 && opts.MinOrder != 0 {
		return nil, errors.New("cannot provide both max and min order options")
	}

	pagingBackward := opts.MinOrder != 0

	revisionFilter := bson.M{}
	if pagingBackward {
		revisionFilter["$lte"] = searchOffset + MaxWaterfallVersionLimit
		revisionFilter["$gt"] = searchOffset
	} else {
		revisionFilter["$gte"] = searchOffset - MaxWaterfallVersionLimit
		revisionFilter["$lt"] = searchOffset
	}
	match[task.RevisionOrderNumberKey] = revisionFilter

	if len(opts.Statuses) > 0 {
		match[task.DisplayStatusCacheKey] = bson.M{"$in": opts.Statuses}
	}

	if len(opts.Tasks) > 0 {
		taskNamesAsRegex := strings.Join(opts.Tasks, "|")
		if opts.TaskCaseSensitive {
			match[task.DisplayNameKey] = bson.M{"$regex": taskNamesAsRegex}
		} else {
			match[task.DisplayNameKey] = bson.M{"$regex": taskNamesAsRegex, "$options": "i"}
		}

	}

	if len(opts.Variants) > 0 {
		variantsAsRegex := strings.Join(opts.Variants, "|")
		if opts.VariantCaseSensitive {
			match["$or"] = []bson.M{
				{task.BuildVariantKey: bson.M{"$regex": variantsAsRegex}},
				{task.BuildVariantDisplayNameKey: bson.M{"$regex": variantsAsRegex}},
			}
		} else {
			match["$or"] = []bson.M{
				{task.BuildVariantKey: bson.M{"$regex": variantsAsRegex, "$options": "i"}},
				{task.BuildVariantDisplayNameKey: bson.M{"$regex": variantsAsRegex, "$options": "i"}},
			}
		}
	}

	pipeline := []bson.M{{"$match": match}}

	pipeline = append(pipeline, bson.M{
		"$group": bson.M{
			task.IdKey: "$" + task.VersionKey,
			// All tasks with the same version key should have the same order number, but $max is the safest way to ensure we can sort from newest to oldest upon grouping.
			task.RevisionOrderNumberKey: bson.M{
				"$max": "$" + task.RevisionOrderNumberKey,
			},
		},
	})

	if pagingBackward {
		// When querying with a $gt param, sort ascending so we can take `limit` versions nearest to the MinOrder param
		pipeline = append(pipeline, bson.M{"$sort": bson.M{task.RevisionOrderNumberKey: 1}})
		pipeline = append(pipeline, bson.M{"$limit": opts.Limit})
		// Then apply an ascending sort so these versions are returned in the expected descending order
		pipeline = append(pipeline, bson.M{"$sort": bson.M{task.RevisionOrderNumberKey: -1}})

	} else {
		pipeline = append(pipeline, bson.M{"$sort": bson.M{task.RevisionOrderNumberKey: -1}})
		pipeline = append(pipeline, bson.M{"$limit": opts.Limit})

	}

	versionLookupKey := "version"

	// Get version documents
	pipeline = append(pipeline, bson.M{
		"$lookup": bson.M{
			"from":         VersionCollection,
			"localField":   task.IdKey,
			"foreignField": VersionIdKey,
			"as":           versionLookupKey,
		},
	})

	// Reroot to only return version docs
	pipeline = append(pipeline, bson.M{
		"$replaceRoot": bson.M{
			"newRoot": bson.M{"$arrayElemAt": bson.A{"$" + versionLookupKey, 0}},
		},
	})

	res := []Version{}
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(task.Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "finding versions matching task filters")
	}
	if err = cursor.All(ctx, &res); err != nil {
		return nil, err
	}

	return res, nil
}

// GetActiveWaterfallVersions returns at most `opts.limit` activated versions for a given project.
// It performantly applies build variant and requester filters; for task-related filters, see GetActiveVersionsByTaskFilters.
func GetActiveWaterfallVersions(ctx context.Context, projectId string, opts WaterfallOptions) ([]Version, error) {
	ctx = utility.ContextWithAppendedAttributes(ctx, []attribute.KeyValue{attribute.String(evergreen.AggregationNameOtelAttribute, "GetActiveWaterfallVersions")})

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
		var versionSearchCutoff int
		if pagingForward {
			versionSearchCutoff = opts.MaxOrder - MaxWaterfallVersionLimit
		} else if pagingBackward {
			// When paginating backwards, the order specifies the oldest version that will be investigated in the query. No need to increment by MaxWaterfallVersionLimit
			versionSearchCutoff = opts.MinOrder
		} else {
			mostRecentVersion, err := GetMostRecentWaterfallVersion(ctx, projectId)
			if err != nil {
				return nil, errors.Wrap(err, "getting most recent version")
			}
			versionSearchCutoff = mostRecentVersion.RevisionOrderNumber - MaxWaterfallVersionLimit
		}

		buildVariantPipeline, err := getBuildVariantFilterPipeline(ctx, opts.Variants, opts.VariantCaseSensitive, match, projectId, versionSearchCutoff, opts.OmitInactiveBuilds)
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
	if minOrder != 0 && maxOrder != 0 && minOrder > maxOrder {
		return nil, errors.New("minOrder must be less than or equal to maxOrder")
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

// GetVersionBuilds returns a list of builds with populated tasks for the given build IDs.
func GetVersionBuilds(ctx context.Context, buildIds []string) ([]WaterfallBuild, error) {
	ctx = utility.ContextWithAppendedAttributes(ctx, []attribute.KeyValue{attribute.String(evergreen.AggregationNameOtelAttribute, "GetVersionBuilds")})

	if len(buildIds) == 0 {
		return []WaterfallBuild{}, nil
	}

	pipeline := []bson.M{
		{
			"$match": bson.M{
				task.BuildIdKey:     bson.M{"$in": buildIds},
				task.DisplayOnlyKey: bson.M{"$ne": true},
			},
		},
		{
			"$project": bson.M{
				task.IdKey:                 1,
				task.DisplayNameKey:        1,
				task.DisplayStatusCacheKey: 1,
				task.ExecutionKey:          1,
				task.StatusKey:             1,
				task.BuildIdKey:            1,
			},
		},
		{
			"$group": bson.M{
				"_id": "$" + task.BuildIdKey,
				"tasks": bson.M{
					"$push": bson.M{
						"_id":                  "$" + task.IdKey,
						"display_name":         "$" + task.DisplayNameKey,
						"display_status_cache": "$" + task.DisplayStatusCacheKey,
						"execution":            "$" + task.ExecutionKey,
						"status":               "$" + task.StatusKey,
					},
				},
			},
		},
		{
			"$set": bson.M{
				"tasks": bson.M{
					"$sortArray": bson.M{
						"input":  "$tasks",
						"sortBy": bson.M{"display_name": 1},
					},
				},
			},
		},
		{
			"$lookup": bson.M{
				"from":         build.Collection,
				"localField":   "_id",
				"foreignField": build.IdKey,
				"as":           "build",
			},
		},
		{"$unwind": "$build"},
		{
			"$project": bson.M{
				"_id":           "$build." + build.IdKey,
				"activated":     "$build." + build.ActivatedKey,
				"build_variant": "$build." + build.BuildVariantKey,
				"display_name":  "$build." + build.DisplayNameKey,
				"version":       "$build." + build.VersionKey,
				"tasks":         1,
			},
		},
		{"$sort": bson.M{"display_name": 1}},
	}

	res := []WaterfallBuild{}
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(task.Collection).Aggregate(ctx, pipeline)
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
		{"$project": bson.M{VersionBuildVariantsKey: 0}},
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
		{"$project": bson.M{VersionBuildVariantsKey: 0}},
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

// GetOffsetVersionOrderByRevision returns the revision order of a system-requested version within close range of the given
// githash revision. Notably, it does NOT return the revision order of the version with that githash revision. This
// is because we want the target commit to be shown in the center of the page.
func GetOffsetVersionOrderByRevision(ctx context.Context, revision string, projectId string, limit int) (int, error) {
	if len(revision) < minRevisionLength {
		return 0, errors.New(fmt.Sprintf("at least %d characters must be provided for the revision", minRevisionLength))
	}
	found, err := VersionFindOne(ctx, VersionByProjectIdAndRevisionPrefix(projectId, revision).WithFields(VersionRevisionOrderNumberKey))
	if err != nil {
		return 0, errors.New(fmt.Sprintf("finding version with revision '%s': %s", revision, err.Error()))
	} else if found == nil {
		return 0, errors.New(fmt.Sprintf("version with revision '%s' not found", revision))
	}
	// Offset the order number so the specified revision lands nearer to the center of the page.
	// Increment by 1 to account for Waterfall query being non-inclusive.
	return found.RevisionOrderNumber + limit/2 + 1, nil
}

// GetOffsetVersionOrderByDate returns the revision order of a system-requested version created on or before the given date,
// incremented by 1 to account for the Waterfall query being non-inclusive.
func GetOffsetVersionOrderByDate(ctx context.Context, date time.Time, projectId string) (int, error) {
	found, err := VersionFindOne(ctx, VersionByProjectIdAndCreateTime(projectId, date).WithFields(VersionRevisionOrderNumberKey))
	if err != nil {
		return 0, errors.New(fmt.Sprintf("finding version on or before date '%s': %s", date.Format(time.DateOnly), err.Error()))
	} else if found == nil {
		return 0, errors.New(fmt.Sprintf("version on or before date '%s' not found", date.Format(time.DateOnly)))
	}
	// Increment by 1 to account for Waterfall query being non-inclusive.
	return found.RevisionOrderNumber + 1, nil
}
