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
	ParserProjectIdKey                     = bsonutil.MustHaveTag(ParserProject{}, "Id")
	ParserProjectConfigNumberKey           = bsonutil.MustHaveTag(ParserProject{}, "ConfigUpdateNumber")
	ParserProjectEnabledKey                = bsonutil.MustHaveTag(ParserProject{}, "Enabled")
	ParserProjectStepbackKey               = bsonutil.MustHaveTag(ParserProject{}, "Stepback")
	ParserProjectPreErrorFailsTaskKey      = bsonutil.MustHaveTag(ParserProject{}, "PreErrorFailsTask")
	ParserProjectOomTracker                = bsonutil.MustHaveTag(ParserProject{}, "OomTracker")
	ParserProjectBatchTimeKey              = bsonutil.MustHaveTag(ParserProject{}, "BatchTime")
	ParserProjectOwnerKey                  = bsonutil.MustHaveTag(ParserProject{}, "Owner")
	ParserProjectRepoKey                   = bsonutil.MustHaveTag(ParserProject{}, "Repo")
	ParserProjectRemotePathKey             = bsonutil.MustHaveTag(ParserProject{}, "RemotePath")
	ParserProjectBranchKey                 = bsonutil.MustHaveTag(ParserProject{}, "Branch")
	ParserProjectIdentifierKey             = bsonutil.MustHaveTag(ParserProject{}, "Identifier")
	ParserProjectDisplayNameKey            = bsonutil.MustHaveTag(ParserProject{}, "DisplayName")
	ParserProjectCommandTypeKey            = bsonutil.MustHaveTag(ParserProject{}, "CommandType")
	ParserProjectIgnoreKey                 = bsonutil.MustHaveTag(ParserProject{}, "Ignore")
	ParserProjectParametersKey             = bsonutil.MustHaveTag(ParserProject{}, "Parameters")
	ParserProjectPreKey                    = bsonutil.MustHaveTag(ParserProject{}, "Pre")
	ParserProjectPostKey                   = bsonutil.MustHaveTag(ParserProject{}, "Post")
	ParserProjectEarlyTerminationKey       = bsonutil.MustHaveTag(ParserProject{}, "EarlyTermination")
	ParserProjectTimeoutKey                = bsonutil.MustHaveTag(ParserProject{}, "Timeout")
	ParserProjectCallbackTimeoutKey        = bsonutil.MustHaveTag(ParserProject{}, "CallbackTimeout")
	ParserProjectModulesKey                = bsonutil.MustHaveTag(ParserProject{}, "Modules")
	ParserProjectBuildVariantsKey          = bsonutil.MustHaveTag(ParserProject{}, "BuildVariants")
	ParserProjectFunctionsKey              = bsonutil.MustHaveTag(ParserProject{}, "Functions")
	ParserProjectTaskGroupsKey             = bsonutil.MustHaveTag(ParserProject{}, "TaskGroups")
	ParserProjectTasksKey                  = bsonutil.MustHaveTag(ParserProject{}, "Tasks")
	ParserProjectExecTimeoutSecsKey        = bsonutil.MustHaveTag(ParserProject{}, "ExecTimeoutSecs")
	ParserProjectLoggersKey                = bsonutil.MustHaveTag(ParserProject{}, "Loggers")
	ParserProjectAxesKey                   = bsonutil.MustHaveTag(ParserProject{}, "Axes")
	ParserProjectCreateTimeKey             = bsonutil.MustHaveTag(ParserProject{}, "CreateTime")
	ParserProjectTaskAnnotationSettingsKey = bsonutil.MustHaveTag(ParserProject{}, "TaskAnnotationSettings")
	ParserProjectBuildBaronSettingsKey     = bsonutil.MustHaveTag(ParserProject{}, "BuildBaronSettings")
	ParserProjectPerfEnabledKey            = bsonutil.MustHaveTag(ParserProject{}, "PerfEnabled")
	ParserProjectCommitQueueAliasesKey     = bsonutil.MustHaveTag(ParserProject{}, "CommitQueueAliases")
	ParserProjectGitHubPRAliasesKey        = bsonutil.MustHaveTag(ParserProject{}, "GitHubPRAliases")
	ParserProjectGitTagAliasesKey          = bsonutil.MustHaveTag(ParserProject{}, "GitTagAliases")
	ParserProjectGitHubChecksAliasesKey    = bsonutil.MustHaveTag(ParserProject{}, "GitHubChecksAliases")
	ParserProjectPatchAliasesKey           = bsonutil.MustHaveTag(ParserProject{}, "PatchAliases")
)

// ParserProjectFindOneById returns the parser project for the version
func ParserProjectFindOneById(id string) (*ParserProject, error) {
	return ParserProjectFindOne(ParserProjectById(id))
}

// ParserProjectFindOneIdWithFields returns a parser project with the given ID, projecting only
// the given fields.
func ParserProjectFindOneIdWithFields(id string, projected ...string) (*ParserProject, error) {
	pp := &ParserProject{}
	query := db.Query(bson.M{ParserProjectIdKey: id})

	if len(projected) > 0 {
		query = query.WithFields(projected...)
	}

	err := db.FindOneQ(ParserProjectCollection, query, pp)

	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return pp, nil
}

// ParserProjectFindOne finds a parser project with a given query.
func ParserProjectFindOne(query db.Q) (*ParserProject, error) {
	project := &ParserProject{}
	err := db.FindOneQ(ParserProjectCollection, query, project)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return project, err
}

// ParserProjectByVersion finds a parser project with a given version.
// If version is empty the last known good config will be returned
func ParserProjectByVersion(projectId string, version string) (*ParserProject, error) {
	lookupVersion := false
	if version == "" {
		lastGoodVersion, err := FindVersionByLastKnownGoodConfig(projectId, -1)
		if err != nil || lastGoodVersion == nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":    "Unable to retrieve last good version for project",
				"project_id": projectId,
				"version":    version,
			}))
			return nil, errors.Wrapf(err, "Unable to retrieve last good version for project '%s'", projectId)
		}
		version = lastGoodVersion.Id
		lookupVersion = true
	}
	parserProject, err := ParserProjectFindOneIdWithFields(version,
		ParserProjectPerfEnabledKey, ProjectRefDeactivatePreviousKey, ParserProjectTaskAnnotationSettingsKey, ParserProjectBuildBaronSettingsKey,
		ParserProjectCommitQueueAliasesKey, ParserProjectPatchAliasesKey, ParserProjectGitHubChecksAliasesKey, ParserProjectGitTagAliasesKey,
		ParserProjectGitHubPRAliasesKey)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message":        "Error retrieving parser project for version",
			"project_id":     projectId,
			"version":        version,
			"lookup_version": lookupVersion,
		}))
		return nil, errors.Wrapf(err, "Error retrieving parser project for version '%s'", version)
	}
	return parserProject, nil
}

// ParserProjectById returns a query to find a parser project by id.
func ParserProjectById(id string) db.Q {
	return db.Query(bson.M{ParserProjectIdKey: id})
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

func checkConfigNumberQuery(id string, configNum int) bson.M {
	q := bson.M{ParserProjectIdKey: id}
	if configNum == 0 {
		q["$or"] = []bson.M{
			bson.M{ParserProjectConfigNumberKey: bson.M{"$exists": false}},
			bson.M{ParserProjectConfigNumberKey: configNum},
		}
		return q
	}

	q[ParserProjectConfigNumberKey] = configNum
	return q
}

// TryUpsert suppresses the error of inserting if it's a duplicate key error
// and attempts to upsert if config number matches.
func (pp *ParserProject) TryUpsert() error {
	err := ParserProjectUpsertOne(checkConfigNumberQuery(pp.Id, pp.ConfigUpdateNumber), pp)
	if !db.IsDuplicateKey(err) {
		return errors.Wrapf(err, "database error upserting parser project")
	}

	// log this error but don't return it
	grip.Debug(message.WrapError(err, message.Fields{
		"message":                      "parser project not upserted",
		"operation":                    "TryUpsert",
		"version":                      pp.Id,
		"attempted_to_update_with_num": pp.ConfigUpdateNumber,
	}))

	return nil
}

// UpsertWithConfigNumber inserts project if it DNE. Otherwise, updates if the
// existing config number is less than or equal to the new config number.
// If shouldEqual, only update if the config update number matches.
func (pp *ParserProject) UpsertWithConfigNumber(updateNum int) error {
	if pp.Id == "" {
		return errors.New("no version ID given")
	}
	expectedNum := pp.ConfigUpdateNumber
	pp.ConfigUpdateNumber = updateNum
	if err := ParserProjectUpsertOne(checkConfigNumberQuery(pp.Id, expectedNum), pp); err != nil {
		// expose all errors to check duplicate key errors for a race
		return errors.Wrapf(err, "database error upserting parser project '%s'", pp.Id)
	}
	return nil
}
