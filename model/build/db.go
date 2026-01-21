package build

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// The MongoDB collection for build documents.
const Collection = "builds"

var (
	// bson fields for the build struct
	IdKey                         = bsonutil.MustHaveTag(Build{}, "Id")
	CreateTimeKey                 = bsonutil.MustHaveTag(Build{}, "CreateTime")
	StartTimeKey                  = bsonutil.MustHaveTag(Build{}, "StartTime")
	FinishTimeKey                 = bsonutil.MustHaveTag(Build{}, "FinishTime")
	VersionKey                    = bsonutil.MustHaveTag(Build{}, "Version")
	ProjectKey                    = bsonutil.MustHaveTag(Build{}, "Project")
	RevisionKey                   = bsonutil.MustHaveTag(Build{}, "Revision")
	BuildVariantKey               = bsonutil.MustHaveTag(Build{}, "BuildVariant")
	StatusKey                     = bsonutil.MustHaveTag(Build{}, "Status")
	GithubCheckStatusKey          = bsonutil.MustHaveTag(Build{}, "GithubCheckStatus")
	ActivatedKey                  = bsonutil.MustHaveTag(Build{}, "Activated")
	ActivatedByKey                = bsonutil.MustHaveTag(Build{}, "ActivatedBy")
	ActivatedTimeKey              = bsonutil.MustHaveTag(Build{}, "ActivatedTime")
	RevisionOrderNumberKey        = bsonutil.MustHaveTag(Build{}, "RevisionOrderNumber")
	TasksKey                      = bsonutil.MustHaveTag(Build{}, "Tasks")
	TimeTakenKey                  = bsonutil.MustHaveTag(Build{}, "TimeTaken")
	DisplayNameKey                = bsonutil.MustHaveTag(Build{}, "DisplayName")
	RequesterKey                  = bsonutil.MustHaveTag(Build{}, "Requester")
	PredictedMakespanKey          = bsonutil.MustHaveTag(Build{}, "PredictedMakespan")
	ActualMakespanKey             = bsonutil.MustHaveTag(Build{}, "ActualMakespan")
	IsGithubCheckKey              = bsonutil.MustHaveTag(Build{}, "IsGithubCheck")
	AbortedKey                    = bsonutil.MustHaveTag(Build{}, "Aborted")
	AllTasksBlockedKey            = bsonutil.MustHaveTag(Build{}, "AllTasksBlocked")
	HasUnfinishedEssentialTaskKey = bsonutil.MustHaveTag(Build{}, "HasUnfinishedEssentialTask")

	TaskCacheIdKey = bsonutil.MustHaveTag(TaskCache{}, "Id")
)

var CompletedStatuses = []string{evergreen.BuildSucceeded, evergreen.BuildFailed}

// Queries

// All returns all builds.
var All = db.Query(nil)

// ById creates a query that finds a build by its _id.
func ById(id string) db.Q {
	return db.Query(bson.M{IdKey: id})
}

// ByIds creates a query that finds all builds with the given ids.
func ByIds(ids []string) db.Q {
	return db.Query(bson.M{IdKey: bson.M{"$in": ids}})
}

// ByVersion creates a query that returns all builds for a given version.
func ByVersion(version string) db.Q {
	return db.Query(bson.M{VersionKey: version})
}

// ByVersions creates a query that finds all builds with the given version ids.
func ByVersions(vIds []string) db.Q {
	return db.Query(bson.M{VersionKey: bson.M{"$in": vIds}})
}

// ByVersionAndVariant creates a query that finds all builds in a version for a given variant.
func ByVersionAndVariant(version, bv string) db.Q {
	return db.Query(bson.M{
		VersionKey:      version,
		BuildVariantKey: bv,
	})
}

// ByProject creates a query that finds all builds for a given project id.
func ByProject(proj string) db.Q {
	return db.Query(bson.M{ProjectKey: proj})
}

// ByProjectAndVariant creates a query that finds all completed builds for a given project
// and variant, while also specifying a requester
func ByProjectAndVariant(project, variant, requester string, statuses []string) db.Q {
	return db.Query(bson.M{
		ProjectKey:      project,
		StatusKey:       bson.M{"$in": statuses},
		BuildVariantKey: variant,
		RequesterKey:    requester,
	})
}

// ByRevisionAndVariant creates a query that returns the non-patch build for
// a revision + buildvariant combination.
func ByRevisionAndVariant(revision, variant string) db.Q {
	return db.Query(bson.M{
		RevisionKey: revision,
		RequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
		BuildVariantKey: variant,
	})
}

// ByRevisionWithSystemVersionRequester creates a query that returns all builds for a revision.
func ByRevisionWithSystemVersionRequester(revision string) db.Q {
	return db.Query(bson.M{
		RevisionKey: revision,
		RequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
	})
}

// ByRecentlySuccessfulForProjectAndVariant builds a query that returns all
// builds before a given revision that were successful for a project + variant.
// Builds are sorted from most to least recent.
func ByRecentlySuccessfulForProjectAndVariant(revision int, project, variant string) db.Q {
	return db.Query(bson.M{
		RevisionOrderNumberKey: bson.M{"$lt": revision},
		BuildVariantKey:        variant,
		ProjectKey:             project,
		StatusKey:              evergreen.BuildSucceeded,
		RequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
	}).Sort([]string{"-" + RevisionOrderNumberKey})
}

// ByFinishedAfter creates a query that returns all builds for a project/requester
// that were finished after the given time.
func ByFinishedAfter(finishTime time.Time, project string, requester string) db.Q {
	query := bson.M{
		TimeTakenKey:  bson.M{"$ne": time.Duration(0)},
		FinishTimeKey: bson.M{"$gt": finishTime},
		RequesterKey:  requester,
	}
	// filter by project, optionally
	if project != "" {
		query[ProjectKey] = project
	}
	return db.Query(query)
}

// ByBeforeRevision builds a query that returns all builds
// that happened before the given revision for the project/variant.
// Results are sorted by revision order, descending.
func ByBeforeRevision(project, buildVariant string, revision int) db.Q {
	return db.Query(bson.M{
		ProjectKey:      project,
		BuildVariantKey: buildVariant,
		RequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
		RevisionOrderNumberKey: bson.M{"$lt": revision},
	}).Sort([]string{"-" + RevisionOrderNumberKey})
}

// ByAfterRevision builds a query that returns all builds
// that happened at or after the given revision for the project/variant.
// Results are sorted by revision order, ascending.
func ByAfterRevision(project, buildVariant string, revision int) db.Q {
	return db.Query(bson.M{
		ProjectKey:      project,
		BuildVariantKey: buildVariant,
		RequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
		RevisionOrderNumberKey: bson.M{"$gte": revision},
	}).Sort([]string{RevisionOrderNumberKey})
}

// DB Boilerplate

// FindOne returns one build that satisfies the query.
func FindOne(ctx context.Context, query db.Q) (*Build, error) {
	build := &Build{}
	err := db.FindOneQ(ctx, Collection, query, build)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return build, err
}

// FindOneId returns one build by Id.
func FindOneId(ctx context.Context, id string) (*Build, error) {
	return FindOne(ctx, ById(id))
}

// FindBuildsByVersions finds builds matching the version. This only populates a
// subset of the build fields.
func FindBuildsByVersions(ctx context.Context, versionIds []string) ([]Build, error) {
	return Find(ctx, ByVersions(versionIds).
		WithFields(BuildVariantKey, DisplayNameKey, TasksKey, VersionKey, StatusKey, TimeTakenKey, PredictedMakespanKey, ActualMakespanKey, HasUnfinishedEssentialTaskKey))
}

// Find returns all builds that satisfy the query.
func Find(ctx context.Context, query db.Q) ([]Build, error) {
	builds := []Build{}
	err := db.FindAllQ(ctx, Collection, query, &builds)
	return builds, err
}

// UpdateOne updates one build.
func UpdateOne(ctx context.Context, query any, update any) error {
	return db.Update(
		ctx,
		Collection,
		query,
		update,
	)
}

func UpdateAllBuilds(ctx context.Context, query any, update any) error {
	_, err := db.UpdateAll(
		ctx,
		Collection,
		query,
		update,
	)
	return err
}

func FindProjectForBuild(ctx context.Context, buildID string) (string, error) {
	b, err := FindOne(ctx, ById(buildID).Project(bson.M{ProjectKey: 1}))
	if err != nil {
		return "", err
	}
	if b == nil {
		return "", errors.New("build not found")
	}
	return b.Project, nil
}

// FindBuildsForTasks returns all builds that cover the given tasks
func FindBuildsForTasks(ctx context.Context, tasks []task.Task) ([]Build, error) {
	buildIdsMap := map[string]bool{}
	var buildIds []string
	for _, t := range tasks {
		buildIdsMap[t.BuildId] = true
	}
	for buildId := range buildIdsMap {
		buildIds = append(buildIds, buildId)
	}
	builds, err := Find(ctx, ByIds(buildIds))
	if err != nil {
		return nil, errors.Wrap(err, "getting builds")
	}
	return builds, nil
}

// SetBuildStartedForTasks sets tasks' builds status to started and activates them
func SetBuildStartedForTasks(ctx context.Context, tasks []task.Task, caller string) error {
	buildIdSet := map[string]bool{}
	for _, t := range tasks {
		buildIdSet[t.BuildId] = true
	}
	buildIdList := make([]string, 0, len(buildIdSet))
	for k := range buildIdSet {
		buildIdList = append(buildIdList, k)
	}
	update := getSetBuildActivatedUpdate(true, caller)
	update[StatusKey] = evergreen.BuildStarted
	update[StartTimeKey] = time.Now()
	// Set the build status/activation for all the builds containing the tasks that we touched.
	err := UpdateAllBuilds(
		ctx,
		bson.M{IdKey: bson.M{"$in": buildIdList}},
		bson.M{"$set": update},
	)
	return errors.Wrap(err, "setting builds to started")
}

// FindByVersionAndVariants finds all builds that are in the given version and
// match one of the build variant names.
func FindByVersionAndVariants(ctx context.Context, version string, variants []string) ([]Build, error) {
	if len(variants) == 0 {
		return nil, nil
	}

	cur, err := evergreen.GetEnvironment().DB().Collection(Collection).Find(ctx, bson.M{
		VersionKey:      version,
		BuildVariantKey: bson.M{"$in": variants},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "finding builds with version '%s' and variants %s", version, variants)
	}

	var builds []Build
	if err := cur.All(ctx, &builds); err != nil {
		return nil, errors.Wrap(err, "decoding builds")
	}
	return builds, nil
}
