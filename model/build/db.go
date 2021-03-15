package build

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// The MongoDB collection for build documents.
const Collection = "builds"

var (
	// bson fields for the build struct
	IdKey                  = bsonutil.MustHaveTag(Build{}, "Id")
	CreateTimeKey          = bsonutil.MustHaveTag(Build{}, "CreateTime")
	StartTimeKey           = bsonutil.MustHaveTag(Build{}, "StartTime")
	FinishTimeKey          = bsonutil.MustHaveTag(Build{}, "FinishTime")
	VersionKey             = bsonutil.MustHaveTag(Build{}, "Version")
	ProjectKey             = bsonutil.MustHaveTag(Build{}, "Project")
	RevisionKey            = bsonutil.MustHaveTag(Build{}, "Revision")
	BuildVariantKey        = bsonutil.MustHaveTag(Build{}, "BuildVariant")
	BuildNumberKey         = bsonutil.MustHaveTag(Build{}, "BuildNumber")
	StatusKey              = bsonutil.MustHaveTag(Build{}, "Status")
	GithubCheckStatusKey   = bsonutil.MustHaveTag(Build{}, "GithubCheckStatus")
	ActivatedKey           = bsonutil.MustHaveTag(Build{}, "Activated")
	ActivatedByKey         = bsonutil.MustHaveTag(Build{}, "ActivatedBy")
	ActivatedTimeKey       = bsonutil.MustHaveTag(Build{}, "ActivatedTime")
	RevisionOrderNumberKey = bsonutil.MustHaveTag(Build{}, "RevisionOrderNumber")
	TasksKey               = bsonutil.MustHaveTag(Build{}, "Tasks")
	TimeTakenKey           = bsonutil.MustHaveTag(Build{}, "TimeTaken")
	DisplayNameKey         = bsonutil.MustHaveTag(Build{}, "DisplayName")
	RequesterKey           = bsonutil.MustHaveTag(Build{}, "Requester")
	PredictedMakespanKey   = bsonutil.MustHaveTag(Build{}, "PredictedMakespan")
	ActualMakespanKey      = bsonutil.MustHaveTag(Build{}, "ActualMakespan")

	// bson fields for the task caches
	TaskCacheIdKey            = bsonutil.MustHaveTag(TaskCache{}, "Id")
	TaskCacheDisplayNameKey   = bsonutil.MustHaveTag(TaskCache{}, "DisplayName")
	TaskCacheStatusKey        = bsonutil.MustHaveTag(TaskCache{}, "Status")
	TaskCacheStatusDetailsKey = bsonutil.MustHaveTag(TaskCache{}, "StatusDetails")
	TaskCacheStartTimeKey     = bsonutil.MustHaveTag(TaskCache{}, "StartTime")
	TaskCacheTimeTakenKey     = bsonutil.MustHaveTag(TaskCache{}, "TimeTaken")
	TaskCacheActivatedKey     = bsonutil.MustHaveTag(TaskCache{}, "Activated")
	TaskCacheBlockedKey       = bsonutil.MustHaveTag(TaskCache{}, "Blocked")
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

// ByVariant creates a query that finds all builds for a given variant.
func ByVariant(bv string) db.Q {
	return db.Query(bson.M{BuildVariantKey: bv})
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

// ByRecentlyActivatedForProjectAndVariant builds a query that returns all
// builds before a given revision that were activated for a project + variant.
// Builds are sorted from most to least recent.
func ByRecentlyActivatedForProjectAndVariant(revision int, project, variant, requester string) db.Q {
	return db.Query(bson.M{
		RevisionOrderNumberKey: bson.M{"$lt": revision},
		ActivatedKey:           true,
		BuildVariantKey:        variant,
		ProjectKey:             project,
		RequesterKey:           requester,
	}).Sort([]string{"-" + RevisionOrderNumberKey})
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

// ByBetweenBuilds returns all builds that happened between
// the current and previous build.
func ByBetweenBuilds(current, previous *Build) db.Q {
	intermediateRevisions := bson.M{
		"$lt": current.RevisionOrderNumber,
		"$gt": previous.RevisionOrderNumber,
	}
	q := db.Query(bson.M{
		BuildVariantKey:        current.BuildVariant,
		RequesterKey:           current.Requester,
		RevisionOrderNumberKey: intermediateRevisions,
		ProjectKey:             current.Project,
	}).Sort([]string{RevisionOrderNumberKey})
	return q
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

// ByRecentlyFinished builds a query that returns all builds for a given project
// that are versions (not patches), that have finished and have non-zero
// makespans.
func ByRecentlyFinishedWithMakespans(limit int) db.Q {
	return db.Query(bson.M{
		RequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
		PredictedMakespanKey: bson.M{"$gt": 0},
		ActualMakespanKey:    bson.M{"$gt": 0},
		StatusKey:            bson.M{"$in": evergreen.CompletedStatuses},
	}).Sort([]string{RevisionOrderNumberKey}).Limit(limit)
}

// DB Boilerplate

// FindOne returns one build that satisfies the query.
func FindOne(query db.Q) (*Build, error) {
	build := &Build{}
	err := db.FindOneQ(Collection, query, build)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return build, err
}

// FindOneId returns one build by Id.
func FindOneId(id string) (*Build, error) {
	return FindOne(ById(id))
}

func FindBuildsByVersions(versionIds []string) ([]Build, error) {
	return Find(ByVersions(versionIds).
		WithFields(BuildVariantKey, DisplayNameKey, TasksKey, VersionKey, StatusKey, TimeTakenKey, PredictedMakespanKey, ActualMakespanKey))
}

// Find returns all builds that satisfy the query.
func Find(query db.Q) ([]Build, error) {
	builds := []Build{}
	err := db.FindAllQ(Collection, query, &builds)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return builds, err
}

// UpdateOne updates one build.
func UpdateOne(query interface{}, update interface{}) error {
	return db.Update(
		Collection,
		query,
		update,
	)
}

func UpdateAllBuilds(query interface{}, update interface{}) (*adb.ChangeInfo, error) {
	return db.UpdateAll(
		Collection,
		query,
		update,
	)
}

// Remove deletes the build of the given id from the database
func Remove(id string) error {
	return db.Remove(
		Collection,
		bson.M{IdKey: id},
	)
}

func FindProjectForBuild(buildID string) (string, error) {
	b, err := FindOne(ById(buildID).Project(bson.M{ProjectKey: 1}))
	if err != nil {
		return "", err
	}
	if b == nil {
		return "", errors.New("build not found")
	}
	return b.Project, nil
}

func SetBuildStartedForTasks(tasks []task.Task, caller string) error {
	buildIdSet := map[string]bool{}
	catcher := grip.NewBasicCatcher()
	for _, t := range tasks {
		buildIdSet[t.BuildId] = true
		toReset := t.Id
		if t.IsPartOfDisplay() {
			toReset = t.DisplayTask.Id
		}
		if err := SetCachedTaskActivated(t.BuildId, toReset, true); err != nil {
			catcher.Add(errors.Wrap(err, "unable to activate task in build cache"))
			continue
		}

		// update the cached version of the task, in its build document
		if err := ResetCachedTask(t.BuildId, toReset); err != nil {
			catcher.Add(errors.Wrap(err, "unable to reset task in build cache"))
		}
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	// reset the build statuses, once per build
	buildIdList := make([]string, 0, len(buildIdSet))
	for k := range buildIdSet {
		buildIdList = append(buildIdList, k)
	}
	// Set the build status for all the builds containing the tasks that we touched
	_, err := UpdateAllBuilds(
		bson.M{IdKey: bson.M{"$in": buildIdList}},
		bson.M{"$set": bson.M{StatusKey: evergreen.BuildStarted}},
	)
	if err != nil {
		return errors.Wrapf(err, "error updating builds to started")
	}
	// update activation for all the builds
	return errors.Wrap(UpdateActivation(buildIdList, true, caller), "can't activate builds")
}
