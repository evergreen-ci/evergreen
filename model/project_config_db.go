package model

import (
	"context"

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
	ProjectConfigProjectKey    = bsonutil.MustHaveTag(ProjectConfig{}, "Project")
	ProjectConfigCreateTimeKey = bsonutil.MustHaveTag(ProjectConfig{}, "CreateTime")
)

// FindProjectConfigForProjectOrVersion returns a project config by id, or the most recent project config if id is empty
func FindProjectConfigForProjectOrVersion(ctx context.Context, projectId, id string) (*ProjectConfig, error) {
	if id == "" {
		return FindLastKnownGoodProjectConfig(ctx, projectId)
	}
	return FindProjectConfigById(ctx, id)
}

// FindLastKnownGoodProjectConfig retrieves the most recent project config for the given project.
func FindLastKnownGoodProjectConfig(ctx context.Context, projectId string) (*ProjectConfig, error) {
	q := bson.M{
		ProjectConfigProjectKey: projectId,
	}
	pc, err := ProjectConfigFindOne(ctx, db.Query(q).Sort([]string{"-" + ProjectConfigCreateTimeKey}))
	if err != nil {
		return nil, errors.Wrapf(err, "finding recent valid project config for project '%s'", projectId)
	}
	return pc, nil
}

func ProjectConfigFindOne(ctx context.Context, query db.Q) (*ProjectConfig, error) {
	projectConfig := &ProjectConfig{}
	err := db.FindOneQ(ctx, ProjectConfigCollection, query, projectConfig)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return projectConfig, err
}

// FindProjectConfigById returns a project config by id.
func FindProjectConfigById(ctx context.Context, id string) (*ProjectConfig, error) {
	project := &ProjectConfig{}
	query := db.Query(bson.M{ProjectConfigIdKey: id})
	err := db.FindOneQ(ctx, ProjectConfigCollection, query, project)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return project, err
}
