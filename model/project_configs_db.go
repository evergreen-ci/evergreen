package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
)

const (
	ProjectConfigCollection = "project_configs"
)

var (
	ProjectConfigIdKey         = bsonutil.MustHaveTag(ProjectConfig{}, "Id")
	ProjectConfigIdentifierKey = bsonutil.MustHaveTag(ProjectConfig{}, "Identifier")
	ProjectConfigCreateTimeKey = bsonutil.MustHaveTag(ProjectConfig{}, "CreateTime")

	ProjectConfigTaskAnnotationSettingsKey = bsonutil.MustHaveTag(ProjectConfig{}, "TaskAnnotationSettings")
	ProjectConfigBuildBaronSettingsKey     = bsonutil.MustHaveTag(ProjectConfig{}, "BuildBaronSettings")
	ProjectConfigPerfEnabledKey            = bsonutil.MustHaveTag(ProjectConfig{}, "PerfEnabled")
	ProjectConfigCommitQueueAliasesKey     = bsonutil.MustHaveTag(ProjectConfig{}, "CommitQueueAliases")
	ProjectConfigGitHubPRAliasesKey        = bsonutil.MustHaveTag(ProjectConfig{}, "GitHubPRAliases")
	ProjectConfigGitTagAliasesKey          = bsonutil.MustHaveTag(ProjectConfig{}, "GitTagAliases")
	ProjectConfigGitHubChecksAliasesKey    = bsonutil.MustHaveTag(ProjectConfig{}, "GitHubChecksAliases")
	ProjectConfigPatchAliasesKey           = bsonutil.MustHaveTag(ProjectConfig{}, "PatchAliases")
	ProjectConfigWorkstationConfigKey      = bsonutil.MustHaveTag(ProjectConfig{}, "WorkstationConfig")
	ProjectConfigCommitQueueKey            = bsonutil.MustHaveTag(ProjectConfig{}, "CommitQueue")
	ProjectConfigTaskSyncKey               = bsonutil.MustHaveTag(ProjectConfig{}, "TaskSync")
	ProjectConfigPRTestingEnabledKey       = bsonutil.MustHaveTag(ProjectConfig{}, "PRTestingEnabled")
	ProjectConfigGithubChecksEnabledKey    = bsonutil.MustHaveTag(ProjectConfig{}, "GithubChecksEnabled")
	ProjectConfigGitTagVersionsEnabledKey  = bsonutil.MustHaveTag(ProjectConfig{}, "GitTagVersionsEnabled")
	ProjectConfigGithubTriggerAliasesKey   = bsonutil.MustHaveTag(ProjectConfig{}, "GithubTriggerAliases")
	ProjectConfigPeriodicBuildsKey         = bsonutil.MustHaveTag(ProjectConfig{}, "PeriodicBuilds")
	ProjectConfigDeactivatePreviousKey     = bsonutil.MustHaveTag(ProjectConfig{}, "DeactivatePrevious")
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
		ProjectConfigIdentifierKey: projectId,
	}
	pc, err := ProjectConfigFindOne(db.Query(q).Sort([]string{"-" + ProjectConfigCreateTimeKey}))
	if err != nil {
		return nil, errors.Wrapf(err, "Error finding recent valid project config for '%s'", projectId)
	}
	return pc, nil
}

func ProjectConfigFindOne(query db.Q) (*ProjectConfig, error) {
	projectConfig := &ProjectConfig{}
	err := db.FindOneQ(ProjectConfigCollection, query, projectConfig)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return projectConfig, err
}

// FindProjectConfigById returns a project config by id.
func FindProjectConfigById(id string) (*ProjectConfig, error) {
	project := &ProjectConfig{}
	query := db.Query(bson.M{ProjectConfigIdKey: id})
	err := db.FindOneQ(ProjectConfigCollection, query, project)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return project, err
}

// ProjectConfigUpsertOne updates one project config
func ProjectConfigUpsertOne(query interface{}, update interface{}) error {
	_, err := db.Upsert(
		ProjectConfigCollection,
		query,
		update,
	)

	return err
}
