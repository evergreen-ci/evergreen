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
	ProjectConfigsIdKey         = bsonutil.MustHaveTag(ProjectConfig{}, "Id")
	ProjectConfigsIdentifierKey = bsonutil.MustHaveTag(ProjectConfig{}, "Identifier")
	ProjectConfigsCreateTimeKey = bsonutil.MustHaveTag(ProjectConfig{}, "CreateTime")

	ProjectConfigsTaskAnnotationSettingsKey = bsonutil.MustHaveTag(ProjectConfig{}, "TaskAnnotationSettings")
	ProjectConfigsBuildBaronSettingsKey     = bsonutil.MustHaveTag(ProjectConfig{}, "BuildBaronSettings")
	ProjectConfigsPerfEnabledKey            = bsonutil.MustHaveTag(ProjectConfig{}, "PerfEnabled")
	ProjectConfigsCommitQueueAliasesKey     = bsonutil.MustHaveTag(ProjectConfig{}, "CommitQueueAliases")
	ProjectConfigsGitHubPRAliasesKey        = bsonutil.MustHaveTag(ProjectConfig{}, "GitHubPRAliases")
	ProjectConfigsGitTagAliasesKey          = bsonutil.MustHaveTag(ProjectConfig{}, "GitTagAliases")
	ProjectConfigsGitHubChecksAliasesKey    = bsonutil.MustHaveTag(ProjectConfig{}, "GitHubChecksAliases")
	ProjectConfigsPatchAliasesKey           = bsonutil.MustHaveTag(ProjectConfig{}, "PatchAliases")
	ProjectConfigsWorkstationConfigKey      = bsonutil.MustHaveTag(ProjectConfig{}, "WorkstationConfig")
	ProjectConfigsCommitQueueKey            = bsonutil.MustHaveTag(ProjectConfig{}, "CommitQueue")
	ProjectConfigsTaskSyncKey               = bsonutil.MustHaveTag(ProjectConfig{}, "TaskSync")
)

// FindProjectConfigToMerge returns a project config by id, or the most recent project config if id is empty
func FindProjectConfigToMerge(projectId, id string) (*ProjectConfig, error) {
	if id == "" {
		return FindLastKnownGoodProjectConfig(projectId)
	}
	return FindProjectConfigById(id)
}

// FindLastKnownGoodProjectConfig retrieves the most recent project config for the given project.
func FindLastKnownGoodProjectConfig(projectId string) (*ProjectConfig, error) {
	q := bson.M{
		ProjectConfigsIdentifierKey: projectId,
	}
	pc, err := ProjectConfigsFindOne(db.Query(q).Sort([]string{"-" + ProjectConfigsCreateTimeKey}))
	if err != nil {
		return nil, errors.Wrapf(err, "Error finding recent valid project config for '%s'", projectId)
	}
	return pc, nil
}

func ProjectConfigsFindOne(query db.Q) (*ProjectConfig, error) {
	projectConfig := &ProjectConfig{}
	err := db.FindOneQ(ProjectConfigsCollection, query, projectConfig)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return projectConfig, err
}

// FindProjectConfigById returns a project config by id.
func FindProjectConfigById(id string) (*ProjectConfig, error) {
	project := &ProjectConfig{}
	query := db.Query(bson.M{ProjectConfigsIdKey: id})
	err := db.FindOneQ(ProjectConfigsCollection, query, project)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return project, err
}

// ProjectConfigsUpsertOne updates one project config
func ProjectConfigsUpsertOne(query interface{}, update interface{}) error {
	_, err := db.Upsert(
		ProjectConfigsCollection,
		query,
		update,
	)

	return err
}
