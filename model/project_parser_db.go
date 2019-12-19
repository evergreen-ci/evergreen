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
func ParserProjectUpsertOne(query interface{}, update interface{}) error {
	_, err := db.Upsert(
		ParserProjectCollection,
		query,
		update,
	)

	return err
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
			ParserProjectAxesKey:              pp.Axes,
			ParserProjectConfigNumberKey:      pp.ConfigUpdateNumber,
		},
	}
}

func checkConfigNumberQuery(id string, configNum int) bson.M {
	q := bson.M{ParserProjectIdKey: id}
	if configNum == 0 {
		q[ParserProjectConfigNumberKey] = bson.M{"$exists": false}
		return q
	}

	q[ParserProjectConfigNumberKey] = configNum
	return q
}

// TryInsert suppresses the error of inserting if it's a duplicate key error and attempts to
// upsert if config number matches
func (pp *ParserProject) TryUpsert() error {
	err := ParserProjectUpsertOne(checkConfigNumberQuery(pp.Id, pp.ConfigUpdateNumber), setAllFieldsUpdate(pp))
	if isValidErr(err) {
		return errors.Wrapf(err, "database error upserting parser project")
	}

	// log this error but don't return it
	grip.Debug(message.WrapError(err, message.Fields{
		"message":                      "parser project not upserted",
		"version":                      pp.Id,
		"attempted_to_update_with_num": pp.ConfigUpdateNumber,
	}))

	return nil
}

// UpsertWithConfigNumber inserts project if it DNE. Otherwise, updates if the
// existing config number is less than or equal to the new config number.
// If shouldEqual, only update if the config update number matches.
func (pp *ParserProject) UpsertWithConfigNumber(num int, shouldEqual bool) error {
	if pp.Id == "" {
		return errors.New("no version ID given")
	}
	pp.ConfigUpdateNumber = num + 1
	q := bson.M{ParserProjectIdKey: pp.Id}
	if shouldEqual { // guarantee that the config number hasn't changed
		q = checkConfigNumberQuery(pp.Id, num)
	}

	err := ParserProjectUpsertOne(q, setAllFieldsUpdate(pp))
	if isValidErr(err) {
		return errors.Wrapf(err, "database error upserting parser project '%s'", pp.Id)
	}
	// log this error but don't return it
	grip.Debug(message.WrapError(err, message.Fields{
		"message":                      "parser project not upserted",
		"version":                      pp.Id,
		"attempted_to_update_with_num": num + 1,
	}))
	return nil
}

func isValidErr(err error) bool {
	// suppress results not found errors and dup key errors
	if err == nil || adb.ResultsNotFound(err) || strings.Contains(err.Error(), "duplicate key error") {
		return false
	}

	return true
}
