package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
)

const (
	ProjectConfigsCollection = "project_configs"
)

var (
	ProjectConfigsIdKey          = bsonutil.MustHaveTag(ProjectConfigs{}, "Id")
	ProjectConfigsEnabledKey     = bsonutil.MustHaveTag(ProjectConfigs{}, "Enabled")
	ProjectConfigsOwnerKey       = bsonutil.MustHaveTag(ProjectConfigs{}, "Owner")
	ProjectConfigsRepoKey        = bsonutil.MustHaveTag(ProjectConfigs{}, "Repo")
	ProjectConfigsRemotePathKey  = bsonutil.MustHaveTag(ProjectConfigs{}, "RemotePath")
	ProjectConfigsBranchKey      = bsonutil.MustHaveTag(ProjectConfigs{}, "Branch")
	ProjectConfigsIdentifierKey  = bsonutil.MustHaveTag(ProjectConfigs{}, "Identifier")
	ProjectConfigsDisplayNameKey = bsonutil.MustHaveTag(ProjectConfigs{}, "DisplayName")
	ProjectConfigsCreateTimeKey  = bsonutil.MustHaveTag(ProjectConfigs{}, "CreateTime")

	ProjectConfigsTaskAnnotationSettingsKey = bsonutil.MustHaveTag(ProjectConfigs{}, "TaskAnnotationSettings")
	ProjectConfigsBuildBaronSettingsKey     = bsonutil.MustHaveTag(ProjectConfigs{}, "BuildBaronSettings")
	ProjectConfigsPerfEnabledKey            = bsonutil.MustHaveTag(ProjectConfigs{}, "PerfEnabled")
	ProjectConfigsCommitQueueAliasesKey     = bsonutil.MustHaveTag(ProjectConfigs{}, "CommitQueueAliases")
	ProjectConfigsGitHubPRAliasesKey        = bsonutil.MustHaveTag(ProjectConfigs{}, "GitHubPRAliases")
	ProjectConfigsGitTagAliasesKey          = bsonutil.MustHaveTag(ProjectConfigs{}, "GitTagAliases")
	ProjectConfigsGitHubChecksAliasesKey    = bsonutil.MustHaveTag(ProjectConfigs{}, "GitHubChecksAliases")
	ProjectConfigsPatchAliasesKey           = bsonutil.MustHaveTag(ProjectConfigs{}, "PatchAliases")
	ProjectConfigsWorkstationConfigKey      = bsonutil.MustHaveTag(ProjectConfigs{}, "WorkstationConfig")
	ProjectConfigsCommitQueueKey            = bsonutil.MustHaveTag(ProjectConfigs{}, "CommitQueue")
)

// ProjectConfigsFindOneById returns the parser project for the version
func ProjectConfigsFindOneById(id string) (*ProjectConfigs, error) {
	return ProjectConfigsFindOne(ProjectConfigsById(id))
}

// ProjectConfigsFindOne finds a parser project with a given query.
func ProjectConfigsFindOne(query db.Q) (*ProjectConfigs, error) {
	project := &ProjectConfigs{}
	err := db.FindOneQ(ProjectConfigsCollection, query, project)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return project, err
}

// ProjectConfigsById returns a query to find a parser project by id.
func ProjectConfigsById(id string) db.Q {
	return db.Query(bson.M{ProjectConfigsIdKey: id})
}

// FindLastGoodConfig filters on versions with valid (i.e., have no errors) config for the given
// project. Does not apply a limit, so should generally be used with a findOneRepoRefQ.
func FindLastGoodConfig(projectId string, revisionOrderNumber int) (*Version, error) {
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

// ProjectConfigsUpsertOne updates one project
func ProjectConfigsUpsertOne(query interface{}, update interface{}) error {
	_, err := db.Upsert(
		ProjectConfigsCollection,
		query,
		update,
	)

	return err
}
