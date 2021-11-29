package model

import (
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
	ProjectConfigsTaskSyncKey               = bsonutil.MustHaveTag(ProjectConfigs{}, "TaskSync")
)

// FindProjectConfigToMerge returns a project config by id, or the most recent project config if id is empty
func FindProjectConfigToMerge(projectId, id string) (*ProjectConfigs, error) {
	if id == "" {
		return FindLastKnownGoodProjectConfig(projectId)
	} else {
		return ProjectConfigsById(id)
	}
}

// FindLastKnownGoodProjectConfig retrieves the most recent project config for the given project.
func FindLastKnownGoodProjectConfig(projectId string) (*ProjectConfigs, error) {
	q := bson.M{
		ProjectConfigsIdentifierKey: projectId,
	}
	pc, err := ProjectConfigsFindOne(db.Query(q).Sort([]string{"-" + ProjectConfigsCreateTimeKey}))
	if err != nil {
		return nil, errors.Wrapf(err, "Error finding recent valid project config for '%s'", projectId)
	}
	return pc, nil
}

func ProjectConfigsFindOne(query db.Q) (*ProjectConfigs, error) {
	projectConfig := &ProjectConfigs{}
	err := db.FindOneQ(ProjectConfigsCollection, query, projectConfig)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return projectConfig, err
}

// ProjectConfigsById returns a project config by id.
func ProjectConfigsById(id string) (*ProjectConfigs, error) {
	project := &ProjectConfigs{}
	query := db.Query(bson.M{ProjectConfigsIdKey: id})
	err := db.FindOneQ(ProjectConfigsCollection, query, project)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return project, err
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
