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
	ProjectConfigProjectKey    = bsonutil.MustHaveTag(ProjectConfig{}, "Project")
	ProjectConfigCreateTimeKey = bsonutil.MustHaveTag(ProjectConfig{}, "CreateTime")
)

// FindProjectConfigForProjectOrVersion returns a project config by id, or the most recent project config if id is empty
func FindProjectConfigForProjectOrVersion(projectId, id string) (*ProjectConfig, error) {
	if id == "" {
		return FindLastKnownGoodProjectConfig(projectId)
	}
	return FindProjectConfigById(id)
}

// FindLastKnownGoodProjectConfig retrieves the most recent project config for the given project.
func FindLastKnownGoodProjectConfig(projectId string) (*ProjectConfig, error) {
	q := bson.M{
		ProjectConfigProjectKey: projectId,
	}
	pc, err := ProjectConfigFindOne(db.Query(q).Sort([]string{"-" + ProjectConfigCreateTimeKey}))
	if err != nil {
		return nil, errors.Wrapf(err, "finding recent valid project config for project '%s'", projectId)
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
