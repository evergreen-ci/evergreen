package model

import (
	"strings"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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

func setAllFieldsUpdate(pp *ParserProject) interface{} {
	return bson.M{
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
			//ParserProjectAxesKey:              pp.Axes,
			ParserProjectConfigNumberKey: pp.ConfigUpdateNumber,
		},
	}
}

// TryInsert suppresses the error of inserting if it's a duplicate key error and attempts to
// upsert if config number matches
func (pp *ParserProject) TryUpsert() error {
	err := pp.Insert()
	if err == nil || !strings.Contains(err.Error(), "duplicate key error") {
		return errors.Wrapf(err, "database error inserting parser project")
	}

	q := bson.M{
		ParserProjectConfigNumberKey: bson.M{"$exists": false},
		ParserProjectConfigNumberKey: pp.ConfigUpdateNumber,
	}
	err = ParserProjectUpdateOne(q, setAllFieldsUpdate(pp))
	if err == nil || adb.ResultsNotFound(err) {
		return nil
	}

	return errors.Wrapf(err, "database error updating parser project")
}

// UpsertWithConfigNumber inserts project if it DNE. Otherwise, updates if the
// existing config number is less than or equal to the new config number.
// If shouldEqual, only update if the config update number matches.
func (pp *ParserProject) UpsertWithConfigNumber(num int, shouldEqual bool) error {
	if pp.Id == "" {
		return errors.New("no version ID given")
	}
	pp.ConfigUpdateNumber = num + 1
	found, err := ParserProjectFindOneById(pp.Id)
	if err != nil {
		return errors.Wrapf(err, "error finding parser project '%s'", pp.Id)
	}
	// the document doesn't exist yet, so just insert it
	if found == nil {
		if err = pp.Insert(); err == nil {
			return nil
		}
		grip.Debug(message.WrapError(err, message.Fields{
			"message":                         "new parser project could not be inserted",
			"version":                         pp.Id,
			"attempted_to_update_with_number": pp.ConfigUpdateNumber,
		}))
		// if the insert didn't succeed, likely this ID has now already been inserted, so we should overwrite it
	}

	q := bson.M{ParserProjectIdKey: pp.Id}
	if shouldEqual { // guarantee that the config number hasn't changed
		q["$or"] = []bson.M{
			bson.M{ParserProjectConfigNumberKey: bson.M{"$exists": false}},
			bson.M{ParserProjectConfigNumberKey: num},
		}
	}

	err = ParserProjectUpdateOne(q, setAllFieldsUpdate(pp))
	if adb.ResultsNotFound(err) {
		grip.Debug(message.WrapError(err, message.Fields{
			"message":                         "parser project not updated",
			"version":                         pp.Id,
			"found_update_num":                found.ConfigUpdateNumber,
			"attempted_to_update_with_number": num + 1,
		}))
		return nil
	}
	return errors.Wrapf(err, "error updating parser project '%s'", pp.Id)
}
