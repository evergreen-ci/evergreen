package model

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
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
	VersionIdKey          = bsonutil.MustHaveTag(Version{}, "Id")
	VersionCreateTimeKey  = bsonutil.MustHaveTag(Version{}, "CreateTime")
	VersionStartTimeKey   = bsonutil.MustHaveTag(Version{}, "StartTime")
	VersionFinishTimeKey  = bsonutil.MustHaveTag(Version{}, "FinishTime")
	VersionRevisionKey    = bsonutil.MustHaveTag(Version{}, "Revision")
	VersionAuthorKey      = bsonutil.MustHaveTag(Version{}, "Author")
	VersionAuthorEmailKey = bsonutil.MustHaveTag(Version{}, "AuthorEmail")
	VersionMessageKey     = bsonutil.MustHaveTag(Version{}, "Message")
	VersionStatusKey      = bsonutil.MustHaveTag(Version{}, "Status")
	VersionParametersKey  = bsonutil.MustHaveTag(Version{}, "Parameters")
	VersionBuildIdsKey    = bsonutil.MustHaveTag(Version{}, "BuildIds")
	// VersionBuildVariantsKey can be a large array. Prefer to exclude it from
	// queries unless it's needed. Otherwise, use (Version).GetBuildVariants to
	// fetch the build variants lazily when needed.
	VersionBuildVariantsKey                     = bsonutil.MustHaveTag(Version{}, "BuildVariants")
	VersionRevisionOrderNumberKey               = bsonutil.MustHaveTag(Version{}, "RevisionOrderNumber")
	VersionRequesterKey                         = bsonutil.MustHaveTag(Version{}, "Requester")
	VersionGitTagsKey                           = bsonutil.MustHaveTag(Version{}, "GitTags")
	VersionIgnoredKey                           = bsonutil.MustHaveTag(Version{}, "Ignored")
	VersionOwnerNameKey                         = bsonutil.MustHaveTag(Version{}, "Owner")
	VersionRepoKey                              = bsonutil.MustHaveTag(Version{}, "Repo")
	VersionBranchKey                            = bsonutil.MustHaveTag(Version{}, "Branch")
	VersionErrorsKey                            = bsonutil.MustHaveTag(Version{}, "Errors")
	VersionWarningsKey                          = bsonutil.MustHaveTag(Version{}, "Warnings")
	VersionIdentifierKey                        = bsonutil.MustHaveTag(Version{}, "Identifier")
	VersionRemoteKey                            = bsonutil.MustHaveTag(Version{}, "Remote")
	VersionRemoteURLKey                         = bsonutil.MustHaveTag(Version{}, "RemotePath")
	VersionTriggerIDKey                         = bsonutil.MustHaveTag(Version{}, "TriggerID")
	VersionTriggerTypeKey                       = bsonutil.MustHaveTag(Version{}, "TriggerType")
	VersionSatisfiedTriggersKey                 = bsonutil.MustHaveTag(Version{}, "SatisfiedTriggers")
	VersionPeriodicBuildIDKey                   = bsonutil.MustHaveTag(Version{}, "PeriodicBuildID")
	VersionActivatedKey                         = bsonutil.MustHaveTag(Version{}, "Activated")
	VersionAbortedKey                           = bsonutil.MustHaveTag(Version{}, "Aborted")
	VersionAuthorIDKey                          = bsonutil.MustHaveTag(Version{}, "AuthorID")
	VersionProjectStorageMethodKey              = bsonutil.MustHaveTag(Version{}, "ProjectStorageMethod")
	VersionPreGenerationProjectStorageMethodKey = bsonutil.MustHaveTag(Version{}, "PreGenerationProjectStorageMethod")
	VersionGitTagsTagKey                        = bsonutil.MustHaveTag(GitTag{}, "Tag")
	VersionCostKey                              = bsonutil.MustHaveTag(Version{}, "Cost")
	VersionPredictedCostKey                     = bsonutil.MustHaveTag(Version{}, "PredictedCost")
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

// FindVersionByLastKnownGoodConfig filters on versions with valid (i.e., have no errors) config for the given project.
func FindVersionByLastKnownGoodConfig(ctx context.Context, projectId string, revisionOrderNumber int) (*Version, error) {
	const retryLimit = 50
	q := byLatestProjectVersion(projectId)
	if revisionOrderNumber >= 0 {
		q[VersionRevisionOrderNumberKey] = bson.M{"$lt": revisionOrderNumber}
	}
	for i := 0; i < retryLimit; i++ {
		v, err := VersionFindOne(ctx, db.Query(q).Sort([]string{"-" + VersionRevisionOrderNumberKey}))
		if err != nil {
			return nil, errors.Wrapf(err, "finding recent valid version for project '%s'", projectId)
		}
		if v == nil {
			// No version found - this can happen if all versions have expired due to TTL (365 days)
			// or if the project has never had any mainline commits.
			return nil, nil
		}
		if len(v.Errors) == 0 {
			return v, nil
		}
		// Try again with the new revision order number if error exists for version.
		// We don't include this in the query in order to use an index for identifier, requester, and order number.
		q[VersionRevisionOrderNumberKey] = bson.M{"$lt": v.RevisionOrderNumber}
	}
	return nil, errors.Errorf("couldn't finding version with good config in last %d commits", retryLimit)
}

// byLatestProjectVersion finds the latest commit for the given project.
func byLatestProjectVersion(projectId string) bson.M {
	return bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey:  evergreen.RepotrackerVersionRequester,
	}
}

// FindLatestRevisionAndAuthorForProject returns the latest revision and author ID for the project, and returns an error if it's not found.
func FindLatestRevisionAndAuthorForProject(ctx context.Context, projectId string) (string, string, error) {
	v, err := VersionFindOne(ctx, db.Query(byLatestProjectVersion(projectId)).
		Sort([]string{"-" + VersionRevisionOrderNumberKey}).WithFields(VersionRevisionKey, VersionAuthorIDKey))
	if err != nil {
		return "", "", errors.Wrapf(err, "finding most recent version for project '%s'", projectId)
	}
	if v == nil {
		return "", "", errors.Errorf("no recent version found for project '%s'", projectId)
	}
	if v.Revision == "" {
		return "", "", errors.Errorf("latest version '%s' has no revision", v.Id)
	}
	return v.Revision, v.AuthorID, nil
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
	lengthHash := 40 - len(revisionPrefix)
	return db.Query(
		bson.M{
			VersionIdentifierKey: projectId,
			VersionRevisionKey:   bson.M{"$regex": fmt.Sprintf("^%s[0-9a-f]{%d}$", revisionPrefix, lengthHash)},
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
		})
}

// VersionByProjectIdAndCreateTime finds the most recent system-requested version created on or before a specified createTime.
func VersionByProjectIdAndCreateTime(projectId string, createTime time.Time) db.Q {
	return db.Query(
		bson.M{
			VersionIdentifierKey: projectId,
			VersionCreateTimeKey: bson.M{
				"$lte": createTime,
			},
			VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
		}).Sort([]string{"-" + VersionRevisionOrderNumberKey})
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
		}).Sort([]string{"-" + VersionRevisionOrderNumberKey})
}

// VersionByLastVariantActivation finds the most recent non-patch, non-ignored
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

// VersionByLastTaskActivation finds the most recent non-patch, non-ignored
// versions in a project that have a particular task activated.
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

// VersionByMostRecentNonIgnored finds all non-ignored mainline commit versions
// within a project, ordered by most recently created to oldest, before a given
// time.
func VersionByMostRecentNonIgnored(projectId string, ts time.Time) db.Q {
	return db.Query(
		bson.M{
			VersionRequesterKey:  evergreen.RepotrackerVersionRequester,
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

// VersionFindOne returns a version matching the query.
func VersionFindOne(ctx context.Context, query db.Q) (*Version, error) {
	version := &Version{}
	err := db.FindOneQ(ctx, VersionCollection, query, version)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return version, err
}

// VersionFindOneId returns a version by ID, excluding its BuildVariants. If the
// version needs to load BuildVariants, use VersionFindOneIdWithBuildVariants.
func VersionFindOneId(ctx context.Context, id string) (*Version, error) {
	q := VersionById(id).WithoutFields(VersionBuildVariantsKey)
	return VersionFindOne(ctx, q)
}

// VersionFindOneIdWithBuildVariants returns a version by ID, including its
// BuildVariants. In general, prefer to use VersionFindOneId instead and use
// GetBuildVariants to load the field when necessary unless BuildVariants is
// needed because it is expensive to load.
func VersionFindOneIdWithBuildVariants(ctx context.Context, id string) (*Version, error) {
	q := VersionById(id)
	v := &Version{}
	err := db.FindOneQ(ctx, VersionCollection, q, v)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return v, err
}

// VersionFind returns versions matching the query.
func VersionFind(ctx context.Context, query db.Q) ([]Version, error) {
	versions := []Version{}
	err := db.FindAllQ(ctx, VersionCollection, query, &versions)
	return versions, err
}

// Count returns the number of hosts that satisfy the given query.
func VersionCount(ctx context.Context, query db.Q) (int, error) {
	return db.CountQ(ctx, VersionCollection, query)
}

// UpdateOne updates one version.
func VersionUpdateOne(ctx context.Context, query any, update any) error {
	return db.Update(
		ctx,
		VersionCollection,
		query,
		update,
	)
}

func ActivateVersions(ctx context.Context, versionIds []string) error {
	_, err := db.UpdateAll(
		ctx,
		VersionCollection,
		bson.M{
			VersionIdKey: bson.M{"$in": versionIds},
		},
		bson.M{
			"$set": bson.M{
				VersionActivatedKey: true,
			},
		})
	if err != nil {
		return errors.Wrap(err, "activating versions")
	}
	return nil
}

func UpdateVersionMessage(ctx context.Context, versionId, message string) error {
	return VersionUpdateOne(
		ctx,
		bson.M{VersionIdKey: versionId},
		bson.M{
			"$set": bson.M{
				VersionMessageKey: message,
			},
		},
	)
}

func AddGitTag(ctx context.Context, versionId string, tag GitTag) error {
	return VersionUpdateOne(
		ctx,
		bson.M{VersionIdKey: versionId},
		bson.M{
			"$push": bson.M{
				VersionGitTagsKey: tag,
			},
		},
	)
}

// RemoveGitTagFromVersions removes the git tag from all versions that have the tag.
// It filters by owner and repo to limit the scope of the operation so we don't accidentally
// remove same-named tags from other repositories.
func RemoveGitTagFromVersions(ctx context.Context, owner, repo string, tag GitTag) error {
	_, err := db.UpdateAll(
		ctx,
		VersionCollection,
		bson.M{
			VersionOwnerNameKey: owner,
			VersionRepoKey:      repo,
			VersionGitTagsKey:   bson.M{"$elemMatch": bson.M{VersionGitTagsTagKey: tag.Tag}},
		},
		bson.M{
			"$pull": bson.M{
				VersionGitTagsKey: bson.M{VersionGitTagsTagKey: tag.Tag},
			},
		},
	)
	return err
}

func AddSatisfiedTrigger(ctx context.Context, versionID, definitionID string) error {
	return VersionUpdateOne(ctx, bson.M{VersionIdKey: versionID},
		bson.M{
			"$push": bson.M{
				VersionSatisfiedTriggersKey: definitionID,
			},
		})
}

func GetVersionAuthorID(ctx context.Context, versionID string) (string, error) {
	v, err := VersionFindOne(ctx, VersionById(versionID).WithFields(VersionAuthorIDKey))
	if err != nil {
		return "", errors.Wrapf(err, "getting version '%s'", versionID)
	}
	if v == nil {
		return "", errors.Errorf("no version found for ID '%s'", versionID)
	}

	return v.AuthorID, nil
}

func FindLastPeriodicBuild(ctx context.Context, projectID, definitionID string) (*Version, error) {
	versions, err := VersionFind(ctx, db.Query(bson.M{
		VersionPeriodicBuildIDKey: definitionID,
		VersionIdentifierKey:      projectID,
	}).Sort([]string{"-" + VersionCreateTimeKey}).Limit(1))
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, nil
	}

	return &versions[0], nil
}

func FindProjectForVersion(ctx context.Context, versionID string) (string, error) {
	v, err := VersionFindOne(ctx, VersionById(versionID).Project(bson.M{VersionIdentifierKey: 1}))
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", errors.New("version not found")
	}
	return v.Identifier, nil
}

// FindBaseVersionForVersion finds the base version  for a given version ID. If the version is a patch, it will
// return the base version. If the version is a mainline commit, it will return the previously run mainline commit.
func FindBaseVersionForVersion(ctx context.Context, versionID string) (*Version, error) {
	v, err := VersionFindOne(ctx, VersionById(versionID))
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, errors.New("version not found")
	}
	if evergreen.IsPatchRequester(v.Requester) {
		baseVersion, err := VersionFindOne(ctx, BaseVersionByProjectIdAndRevision(v.Identifier, v.Revision))
		if err != nil {
			return nil, errors.Wrapf(err, "finding base version with id: '%s'", v.Id)
		}
		return baseVersion, nil
	} else {
		previousVersion, err := VersionFindOne(ctx, VersionByProjectIdAndOrder(utility.FromStringPtr(&v.Identifier), v.RevisionOrderNumber-1))
		if err != nil {
			return nil, errors.Wrapf(err, "finding base version with id: '%s'", v.Id)
		}
		return previousVersion, nil
	}
}
