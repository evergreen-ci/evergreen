package model

import (
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

// UpsertWithConfigNumber inserts project if it DNE
// otherwise, updates if the existing config number is less than or equal to the new config number
func (pp *ParserProject) UpsertWithConfigNumber(num int) error {
	if pp.Id == "" {
		return errors.New("no version ID given")
	}

	found, err := ParserProjectFindOneById(pp.Id)
	if err != nil {
		return errors.Wrapf(err, "error finding parser project '%s'", pp.Id)
	}
	if found == nil {
		pp.ConfigUpdateNumber = num
		// this could error but it seems unlikely
		return errors.Wrap(pp.Insert(), "error inserting parser project")
	}

	// NOTE: if $lt: num is true this will error which we don't want ($lt bc we may want to update other things still if equal)
	// either we case here on ErrorNotFound and continue (it's fine if LoadProjectForVersion is using an older version bc that's what it did before so)
	// or we check before updating if $lte but potentially racey
	err = ParserProjectUpdateOne(
		bson.M{
			"$and": []bson.M{
				{ParserProjectIdKey: pp.Id},
				{"$or": []bson.M{
					bson.M{ParserProjectConfigNumberKey: bson.M{"$exists": false}},
					bson.M{ParserProjectConfigNumberKey: bson.M{"$lte": num}},
				}},
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
				ParserProjectAxesKey:              pp.Axes,
				ParserProjectConfigNumberKey:      num,
			},
		})
	if adb.ResultsNotFound(err) {
		grip.Debug(message.Fields{
			"message":                 "parser project not updated",
			"version":                 pp.Id,
			"attempted_update_number": pp.ConfigUpdateNumber,
			"current_update_num":      num,
		})
		return nil
	}
	return errors.Wrapf(err, "error updating parser project '%s'", pp.Id)
}
