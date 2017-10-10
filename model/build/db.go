package build

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// The MongoDB collection for build documents.
const Collection = "builds"

var (
	// bson fields for the build struct
	IdKey                  = bsonutil.MustHaveTag(Build{}, "Id")
	CreateTimeKey          = bsonutil.MustHaveTag(Build{}, "CreateTime")
	StartTimeKey           = bsonutil.MustHaveTag(Build{}, "StartTime")
	FinishTimeKey          = bsonutil.MustHaveTag(Build{}, "FinishTime")
	PushTimeKey            = bsonutil.MustHaveTag(Build{}, "PushTime")
	VersionKey             = bsonutil.MustHaveTag(Build{}, "Version")
	ProjectKey             = bsonutil.MustHaveTag(Build{}, "Project")
	RevisionKey            = bsonutil.MustHaveTag(Build{}, "Revision")
	BuildVariantKey        = bsonutil.MustHaveTag(Build{}, "BuildVariant")
	BuildNumberKey         = bsonutil.MustHaveTag(Build{}, "BuildNumber")
	StatusKey              = bsonutil.MustHaveTag(Build{}, "Status")
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
)

// Queries

// All returns all builds.
var All = db.Query(nil)

// ById creates a query that finds a build by its _id.
func ById(id string) db.Q {
	return db.Query(bson.D{{IdKey, id}})
}

// ByIds creates a query that finds all builds with the given ids.
func ByIds(ids []string) db.Q {
	return db.Query(bson.D{{IdKey, bson.D{{"$in", ids}}}})
}

// ByVersion creates a query that returns all builds for a given version.
func ByVersion(version string) db.Q {
	return db.Query(bson.D{{VersionKey, version}})
}

// ByVersions creates a query that finds all builds with the given version ids.
func ByVersions(vIds []string) db.Q {
	return db.Query(bson.D{{VersionKey, bson.D{{"$in", vIds}}}})
}

// ByVariant creates a query that finds all builds for a given variant.
func ByVariant(bv string) db.Q {
	return db.Query(bson.D{{BuildVariantKey, bv}})
}

// ByProject creates a query that finds all builds for a given project id.
func ByProject(proj string) db.Q {
	return db.Query(bson.D{{ProjectKey, proj}})
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
// a revision + buildvariant combionation.
func ByRevisionAndVariant(revision, variant string) db.Q {
	return db.Query(bson.M{
		RevisionKey:     revision,
		RequesterKey:    evergreen.RepotrackerVersionRequester,
		BuildVariantKey: variant,
	})
}

// ByRevision creates a query that returns all builds for a revision.
func ByRevision(revision string) db.Q {
	return db.Query(bson.M{
		RevisionKey:  revision,
		RequesterKey: evergreen.RepotrackerVersionRequester,
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
		RequesterKey:           evergreen.RepotrackerVersionRequester,
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
		ProjectKey:             project,
		BuildVariantKey:        buildVariant,
		RequesterKey:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumberKey: bson.M{"$lt": revision},
	}).Sort([]string{"-" + RevisionOrderNumberKey})
}

// ByAfterRevision builds a query that returns all builds
// that happened at or after the given revision for the project/variant.
// Results are sorted by revision order, ascending.
func ByAfterRevision(project, buildVariant string, revision int) db.Q {
	return db.Query(bson.M{
		ProjectKey:             project,
		BuildVariantKey:        buildVariant,
		RequesterKey:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumberKey: bson.M{"$gte": revision},
	}).Sort([]string{RevisionOrderNumberKey})
}

// ByRecentlyFinished builds a query that returns all builds for a given project
// that are versions (not patches), that have finished.
func ByRecentlyFinished(limit int) db.Q {
	return db.Query(bson.M{
		RequesterKey: evergreen.RepotrackerVersionRequester,
		StatusKey:    bson.M{"$in": evergreen.CompletedStatuses},
	}).Sort([]string{RevisionOrderNumberKey}).Limit(limit)
}

// DB Boilerplate

// FindOne returns one build that satisfies the query.
func FindOne(query db.Q) (*Build, error) {
	build := &Build{}
	err := db.FindOneQ(Collection, query, build)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return build, err
}

// Find returns all builds that satisfy the query.
func Find(query db.Q) ([]Build, error) {
	builds := []Build{}
	err := db.FindAllQ(Collection, query, &builds)
	if err == mgo.ErrNotFound {
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

func UpdateAllBuilds(query interface{}, update interface{}) (*mgo.ChangeInfo, error) {
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
