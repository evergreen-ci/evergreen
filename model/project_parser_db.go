package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	ParserProjectCollection = "parser_projects"
)

var (
	ParserProjectIdKey                = bsonutil.MustHaveTag(ParserProject{}, "Id")
	ParserProjectConfigNumberKey      = bsonutil.MustHaveTag(ParserProject{}, "ConfigUpdateNumber")
	ParserProjectEnabledKey           = bsonutil.MustHaveTag(ParserProject{}, "Enabled")
	ParserProjectStepbackKey          = bsonutil.MustHaveTag(ParserProject{}, "Stepback")
	ParserProjectPreErrorFailsTaskKey = bsonutil.MustHaveTag(ParserProject{}, "PreErrorFailsTask")
	ParserProjectBatchTimeKey         = bsonutil.MustHaveTag(ParserProject{}, "BatchTime")
	ParserProjectOwnerKey             = bsonutil.MustHaveTag(ParserProject{}, "Owner")
	ParserProjectRepoKey              = bsonutil.MustHaveTag(ParserProject{}, "Repo")
	ParserProjectRepoKindKey          = bsonutil.MustHaveTag(ParserProject{}, "RepoKind")
	ParserProjectRemotePathKey        = bsonutil.MustHaveTag(ParserProject{}, "RemotePath")
	ParserProjectBranchKey            = bsonutil.MustHaveTag(ParserProject{}, "Branch")
	ParserProjectIdentifierKey        = bsonutil.MustHaveTag(ParserProject{}, "Identifier")
	ParserProjectDisplayNameKey       = bsonutil.MustHaveTag(ParserProject{}, "DisplayName")
	ParserProjectCommandTypeKey       = bsonutil.MustHaveTag(ParserProject{}, "CommandType")
	ParserProjectIgnoreKey            = bsonutil.MustHaveTag(ParserProject{}, "Ignore")
	ParserProjectPreKey               = bsonutil.MustHaveTag(ParserProject{}, "Pre")
	ParserProjectPostKey              = bsonutil.MustHaveTag(ParserProject{}, "Post")
	ParserProjectTimeoutKey           = bsonutil.MustHaveTag(ParserProject{}, "Timeout")
	ParserProjectCallbackTimeoutKey   = bsonutil.MustHaveTag(ParserProject{}, "CallbackTimeout")
	ParserProjectModulesKey           = bsonutil.MustHaveTag(ParserProject{}, "Modules")
	ParserProjectBuildVariantsKey     = bsonutil.MustHaveTag(ParserProject{}, "BuildVariants")
	ParserProjectFunctionsKey         = bsonutil.MustHaveTag(ParserProject{}, "Functions")
	ParserProjectTaskGroupsKey        = bsonutil.MustHaveTag(ParserProject{}, "TaskGroups")
	ParserProjectTasksKey             = bsonutil.MustHaveTag(ParserProject{}, "Tasks")
	ParserProjectExecTimeoutSecsKey   = bsonutil.MustHaveTag(ParserProject{}, "ExecTimeoutSecs")
	ParserProjectLoggersKey           = bsonutil.MustHaveTag(ParserProject{}, "Loggers")
	ParserProjectAxesKey              = bsonutil.MustHaveTag(ParserProject{}, "Axes")
)

// ProjectById returns the parser project for the version
func ParserProjectFindOneById(id string) (*ParserProject, error) {
	project := &ParserProject{}
	err := db.FindOneQ(ParserProjectCollection, db.Query(bson.M{ParserProjectIdKey: id}), project)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return project, err
}

// UpdateOne updates one project
func ParserProjectUpdateOne(query interface{}, update interface{}) error {
	return db.Update(
		ParserProjectCollection,
		query,
		update,
	)
}

// UpsertWithConfigNumber inserts project if it DNE, otherwise updates if the config number is correct
func (pp *ParserProject) UpsertWithConfigNumber(num int) error {
	if pp.Id == "" {
		return errors.New("no version ID given")
	}

	found, err := ParserProjectFindOneById(pp.Id)
	if err != nil {
		return errors.Wrapf(err, "error finding parser project '%s'", pp.Id)
	}
	if found == nil {
		fmt.Println("Inserting")
		pp.ConfigUpdateNumber = num
		return errors.Wrap(pp.Insert(), "error inserting parser project")
	}

	fmt.Println("Upserting")
	fmt.Println("Does config number exist?", pp.ConfigUpdateNumber)
	return errors.Wrapf(ParserProjectUpdateOne(
		bson.M{
			ParserProjectIdKey: pp.Id,
			"$or": []bson.M{
				bson.M{ParserProjectConfigNumberKey: bson.M{"$exists": false}},
				bson.M{ParserProjectConfigNumberKey: pp.ConfigUpdateNumber},
			},
		},
		bson.M{
			"$set": bson.M{
				ParserProjectEnabledKey:           pp.Enabled,
				ParserProjectStepbackKey:          pp.Stepback,
				ParserProjectPreErrorFailsTaskKey: pp.PreErrorFailsTask,
				ParserProjectBatchTimeKey:         pp.BatchTime,
				ParserProjectOwnerKey:             pp.Owner,
				ParserProjectRepoKey:              pp.Repo,
				ParserProjectRepoKindKey:          pp.RepoKind,
				ParserProjectRemotePathKey:        pp.RemotePath,
				ParserProjectBranchKey:            pp.Branch,
				ParserProjectIdentifierKey:        pp.Identifier,
				ParserProjectDisplayNameKey:       pp.DisplayName,
				ParserProjectCommandTypeKey:       pp.CommandType,
				ParserProjectIgnoreKey:            pp.Ignore,
				ParserProjectPreKey:               pp.Pre,
				ParserProjectPostKey:              pp.Post,
				ParserProjectTimeoutKey:           pp.Timeout,
				ParserProjectCallbackTimeoutKey:   pp.CallbackTimeout,
				ParserProjectModulesKey:           pp.Modules,
				ParserProjectBuildVariantsKey:     pp.BuildVariants,
				ParserProjectFunctionsKey:         pp.Functions,
				ParserProjectTaskGroupsKey:        pp.TaskGroups,
				ParserProjectTasksKey:             pp.Tasks,
				ParserProjectExecTimeoutSecsKey:   pp.ExecTimeoutSecs,
				ParserProjectLoggersKey:           pp.Loggers,
				ParserProjectConfigNumberKey:      num,
			},
		}), "error updating parser project '%s'", pp.Id)
}
