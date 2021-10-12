package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	VersionCollection = "versions"
)

var (
	// bson fields for the version struct
	VersionIdKey                  = bsonutil.MustHaveTag(Version{}, "Id")
	VersionCreateTimeKey          = bsonutil.MustHaveTag(Version{}, "CreateTime")
	VersionStartTimeKey           = bsonutil.MustHaveTag(Version{}, "StartTime")
	VersionFinishTimeKey          = bsonutil.MustHaveTag(Version{}, "FinishTime")
	VersionRevisionKey            = bsonutil.MustHaveTag(Version{}, "Revision")
	VersionAuthorKey              = bsonutil.MustHaveTag(Version{}, "Author")
	VersionAuthorEmailKey         = bsonutil.MustHaveTag(Version{}, "AuthorEmail")
	VersionMessageKey             = bsonutil.MustHaveTag(Version{}, "Message")
	VersionStatusKey              = bsonutil.MustHaveTag(Version{}, "Status")
	VersionParametersKey          = bsonutil.MustHaveTag(Version{}, "Parameters")
	VersionBuildIdsKey            = bsonutil.MustHaveTag(Version{}, "BuildIds")
	VersionBuildVariantsKey       = bsonutil.MustHaveTag(Version{}, "BuildVariants")
	VersionRevisionOrderNumberKey = bsonutil.MustHaveTag(Version{}, "RevisionOrderNumber")
	VersionRequesterKey           = bsonutil.MustHaveTag(Version{}, "Requester")
	VersionGitTagsKey             = bsonutil.MustHaveTag(Version{}, "GitTags")
	VersionConfigKey              = bsonutil.MustHaveTag(Version{}, "Config")
	VersionConfigNumberKey        = bsonutil.MustHaveTag(Version{}, "ConfigUpdateNumber")
	VersionIgnoredKey             = bsonutil.MustHaveTag(Version{}, "Ignored")
	VersionOwnerNameKey           = bsonutil.MustHaveTag(Version{}, "Owner")
	VersionRepoKey                = bsonutil.MustHaveTag(Version{}, "Repo")
	VersionProjectNameKey         = bsonutil.MustHaveTag(Version{}, "Branch")
	VersionErrorsKey              = bsonutil.MustHaveTag(Version{}, "Errors")
	VersionWarningsKey            = bsonutil.MustHaveTag(Version{}, "Warnings")
	VersionIdentifierKey          = bsonutil.MustHaveTag(Version{}, "Identifier")
	VersionRemoteKey              = bsonutil.MustHaveTag(Version{}, "Remote")
	VersionRemoteURLKey           = bsonutil.MustHaveTag(Version{}, "RemotePath")
	VersionTriggerIDKey           = bsonutil.MustHaveTag(Version{}, "TriggerID")
	VersionTriggerTypeKey         = bsonutil.MustHaveTag(Version{}, "TriggerType")
	VersionSatisfiedTriggersKey   = bsonutil.MustHaveTag(Version{}, "SatisfiedTriggers")
	VersionPeriodicBuildIDKey     = bsonutil.MustHaveTag(Version{}, "PeriodicBuildID")
	VersionActivatedKey           = bsonutil.MustHaveTag(Version{}, "Activated")
)

// ById returns a db.Q object which will filter on {_id : <the id param>}
func VersionById(id string) db.Q {
	return db.Query(bson.M{VersionIdKey: id})
}

// ByIds returns a db.Q object which will find any versions whose _id appears in the given list.
func VersionByIds(ids []string) db.Q {
	return db.Query(bson.M{VersionIdKey: bson.M{"$in": ids}})
}

// All is a query for all versions.
var VersionAll = db.Query(bson.D{})

// ByLastKnownGoodConfig filters on versions with valid (i.e., have no errors) config for the given
// project. Does not apply a limit, so should generally be used with a findOneRepoRefQ.
func FindVersionByLastKnownGoodConfig(projectId string, revisionOrderNumber int) (*Version, error) {
	q := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
		VersionErrorsKey: bson.M{
			"$exists": false,
		},
	}

	if revisionOrderNumber >= 0 {
		q[VersionRevisionOrderNumberKey] = bson.M{"$lt": revisionOrderNumber}
	}
	v, err := VersionFindOne(db.Query(q).Sort([]string{"-" + VersionRevisionOrderNumberKey}))
	if err != nil {
		return nil, errors.Wrapf(err, "Error finding recent valid version for '%s'", projectId)
	}
	return v, nil
}

// BaseVersionByProjectIdAndRevision finds a base version for the given project and revision.
func BaseVersionByProjectIdAndRevision(projectId, revision string) db.Q {
	return db.Query(
		bson.M{
			VersionIdentifierKey: projectId,
			VersionRevisionKey:   revision,
			VersionRequesterKey: bson.M{
				"$in": []string{
					evergreen.RepotrackerVersionRequester,
					evergreen.TriggerRequester,
				},
			},
		})
}

func VersionByProjectIdAndRevisionPrefix(projectId, revisionPrefix string) db.Q {
	lengthHash := (40 - len(revisionPrefix))
	return db.Query(
		bson.M{
			VersionIdentifierKey: projectId,
			VersionRevisionKey:   bson.M{"$regex": fmt.Sprintf("^%s[0-9a-f]{%d}$", revisionPrefix, lengthHash)},
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
		})
}

// ByProjectIdAndOrder finds non-patch versions for the given project with revision
// order numbers less than or equal to revisionOrderNumber.
func VersionByProjectIdAndOrder(projectId string, revisionOrderNumber int) db.Q {
	return db.Query(
		bson.M{
			VersionIdentifierKey:          projectId,
			VersionRevisionOrderNumberKey: bson.M{"$lte": revisionOrderNumber},
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
		})
}

// ByLastVariantActivation finds the most recent non-patch, non-ignored
// versions in a project that have a particular variant activated.
func VersionByLastVariantActivation(projectId, variant string) db.Q {
	return db.Query(
		bson.M{
			VersionIdentifierKey: projectId,
			VersionIgnoredKey:    bson.M{"$ne": true},
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			VersionBuildVariantsKey: bson.M{
				"$elemMatch": bson.M{
					VersionBuildStatusActivatedKey: true,
					VersionBuildStatusVariantKey:   variant,
				},
			},
		},
	).Sort([]string{"-" + VersionRevisionOrderNumberKey})
}

func VersionByLastTaskActivation(projectId, variant, taskName string) db.Q {
	return db.Query(
		bson.M{
			VersionIdentifierKey: projectId,
			VersionIgnoredKey:    bson.M{"$ne": true},
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			VersionBuildVariantsKey: bson.M{
				"$elemMatch": bson.M{
					VersionBuildStatusVariantKey: variant,
					VersionBuildStatusBatchTimeTasksKey: bson.M{
						"$elemMatch": bson.M{
							BatchTimeTaskStatusActivatedKey: true,
							BatchTimeTaskStatusTaskNameKey:  taskName,
						},
					},
				},
			},
		},
	).Sort([]string{"-" + VersionRevisionOrderNumberKey})
}

// ByProjectId finds all non-patch versions within a project.
func VersionByProjectId(projectId string) db.Q {
	return db.Query(
		bson.M{
			VersionIdentifierKey: projectId,
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
		})
}

func VersionByProjectAndTrigger(projectID string, includeTriggered bool) db.Q {
	q := bson.M{
		VersionIdentifierKey: projectID,
		VersionRequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
	}
	if !includeTriggered {
		q[VersionTriggerIDKey] = bson.M{
			"$exists": false,
		}
	}
	return db.Query(q)
}

// VersionByMostRecentSystemRequester finds all mainline versions within a project,
// ordered by most recently created to oldest.
func VersionByMostRecentSystemRequester(projectId string) db.Q {
	return db.Query(
		bson.M{
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			VersionIdentifierKey: projectId,
		},
	).Sort([]string{"-" + VersionRevisionOrderNumberKey})
}

// if startOrder is specified, only returns older versions (i.e. with a smaller revision number)
func VersionBySystemRequesterOrdered(projectId string, startOrder int) db.Q {
	q := bson.M{
		VersionRequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
		VersionIdentifierKey: projectId,
	}
	if startOrder > 0 {
		q[VersionRevisionOrderNumberKey] = bson.M{
			"$lt": startOrder,
		}
	}
	return db.Query(q).Sort([]string{"-" + VersionRevisionOrderNumberKey})
}

// ByMostRecentNonIgnored finds all non-ignored versions within a project,
// ordered by most recently created to oldest, before a given time.
func VersionByMostRecentNonIgnored(projectId string, ts time.Time) db.Q {
	return db.Query(
		bson.M{
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			VersionIdentifierKey: projectId,
			VersionIgnoredKey:    bson.M{"$ne": true},
			VersionCreateTimeKey: bson.M{"$lte": ts},
		},
	).Sort([]string{"-" + VersionRevisionOrderNumberKey})
}

func VersionBySuccessfulBeforeRevision(project string, beforeRevision int) db.Q {
	return db.Query(
		bson.M{
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			VersionIdentifierKey: project,
			VersionStatusKey:     evergreen.VersionSucceeded,
			VersionRevisionOrderNumberKey: bson.M{
				"$lt": beforeRevision,
			},
		},
	)
}

func VersionsByRequesterOrdered(project, requester string, limit, startOrder int) db.Q {
	q := bson.M{
		VersionIdentifierKey: project,
		VersionRequesterKey:  requester,
	}

	if startOrder > 0 {
		q[VersionRevisionOrderNumberKey] = bson.M{
			"$lt": startOrder,
		}
	}
	return db.Query(q).Limit(limit).Sort([]string{"-" + VersionRevisionOrderNumberKey})
}

func VersionFindOne(query db.Q) (*Version, error) {
	version := &Version{}
	err := db.FindOneQ(VersionCollection, query, version)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return version, err
}

func VersionFindOneId(id string) (*Version, error) {
	return VersionFindOne(VersionById(id))
}

func VersionFindByIds(ids []string) ([]Version, error) {
	return VersionFind(db.Query(bson.M{
		VersionIdKey: bson.M{
			"$in": ids,
		}}))
}

func VersionFind(query db.Q) ([]Version, error) {
	versions := []Version{}
	err := db.FindAllQ(VersionCollection, query, &versions)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return versions, err
}

// Count returns the number of hosts that satisfy the given query.
func VersionCount(query db.Q) (int, error) {
	return db.CountQ(VersionCollection, query)
}

// UpdateOne updates one version.
func VersionUpdateOne(query interface{}, update interface{}) error {
	return db.Update(
		VersionCollection,
		query,
		update,
	)
}

func AddGitTag(versionId string, tag GitTag) error {
	return VersionUpdateOne(
		bson.M{VersionIdKey: versionId},
		bson.M{
			"$push": bson.M{
				VersionGitTagsKey: tag,
			},
		},
	)
}

func AddSatisfiedTrigger(versionID, definitionID string) error {
	return VersionUpdateOne(bson.M{VersionIdKey: versionID},
		bson.M{
			"$push": bson.M{
				VersionSatisfiedTriggersKey: definitionID,
			},
		})
}

func FindLastPeriodicBuild(projectID, definitionID string) (*Version, error) {
	versions, err := VersionFind(db.Query(bson.M{
		VersionPeriodicBuildIDKey: definitionID,
		VersionIdentifierKey:      projectID,
	}).Sort([]string{"-" + VersionCreateTimeKey}).Limit(1))
	if err != nil {
		return nil, err
	}
	if versions == nil || len(versions) == 0 {
		return nil, nil
	}

	return &versions[0], nil
}

func FindProjectForVersion(versionID string) (string, error) {
	v, err := VersionFindOne(VersionById(versionID).Project(bson.M{VersionIdentifierKey: 1}))
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", errors.New("version not found")
	}
	return v.Identifier, nil
}
