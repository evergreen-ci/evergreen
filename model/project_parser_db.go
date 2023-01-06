package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
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
	ParserProjectOomTracker           = bsonutil.MustHaveTag(ParserProject{}, "OomTracker")
	ParserProjectBatchTimeKey         = bsonutil.MustHaveTag(ParserProject{}, "BatchTime")
	ParserProjectOwnerKey             = bsonutil.MustHaveTag(ParserProject{}, "Owner")
	ParserProjectRepoKey              = bsonutil.MustHaveTag(ParserProject{}, "Repo")
	ParserProjectRemotePathKey        = bsonutil.MustHaveTag(ParserProject{}, "RemotePath")
	ParserProjectBranchKey            = bsonutil.MustHaveTag(ParserProject{}, "Branch")
	ParserProjectIdentifierKey        = bsonutil.MustHaveTag(ParserProject{}, "Identifier")
	ParserProjectDisplayNameKey       = bsonutil.MustHaveTag(ParserProject{}, "DisplayName")
	ParserProjectCommandTypeKey       = bsonutil.MustHaveTag(ParserProject{}, "CommandType")
	ParserProjectIgnoreKey            = bsonutil.MustHaveTag(ParserProject{}, "Ignore")
	ParserProjectParametersKey        = bsonutil.MustHaveTag(ParserProject{}, "Parameters")
	ParserProjectPreKey               = bsonutil.MustHaveTag(ParserProject{}, "Pre")
	ParserProjectPostKey              = bsonutil.MustHaveTag(ParserProject{}, "Post")
	ParserProjectEarlyTerminationKey  = bsonutil.MustHaveTag(ParserProject{}, "EarlyTermination")
	ParserProjectTimeoutKey           = bsonutil.MustHaveTag(ParserProject{}, "Timeout")
	ParserProjectCallbackTimeoutKey   = bsonutil.MustHaveTag(ParserProject{}, "CallbackTimeout")
	ParserProjectModulesKey           = bsonutil.MustHaveTag(ParserProject{}, "Modules")
	ParserProjectContainersKey        = bsonutil.MustHaveTag(ParserProject{}, "Containers")
	ParserProjectBuildVariantsKey     = bsonutil.MustHaveTag(ParserProject{}, "BuildVariants")
	ParserProjectFunctionsKey         = bsonutil.MustHaveTag(ParserProject{}, "Functions")
	ParserProjectTaskGroupsKey        = bsonutil.MustHaveTag(ParserProject{}, "TaskGroups")
	ParserProjectTasksKey             = bsonutil.MustHaveTag(ParserProject{}, "Tasks")
	ParserProjectExecTimeoutSecsKey   = bsonutil.MustHaveTag(ParserProject{}, "ExecTimeoutSecs")
	ParserProjectLoggersKey           = bsonutil.MustHaveTag(ParserProject{}, "Loggers")
	ParserProjectAxesKey              = bsonutil.MustHaveTag(ParserProject{}, "Axes")
	ParserProjectCreateTimeKey        = bsonutil.MustHaveTag(ParserProject{}, "CreateTime")
)

// ParserProjectFindOneById returns the parser project for the version
func ParserProjectFindOneById(id string) (*ParserProject, error) {
	pp, err := ParserProjectFindOne(ParserProjectById(id))
	if err != nil {
		return nil, err
	}
	if pp != nil && pp.Functions == nil {
		pp.Functions = map[string]*YAMLCommandSet{}
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

// ParserProjectById returns a query to find a parser project by id.
func ParserProjectById(id string) db.Q {
	return db.Query(bson.M{ParserProjectIdKey: id})
}

// ParserProjectUpsertOne updates one project
func ParserProjectUpsertOne(query interface{}, update interface{}) error {
	_, err := db.Upsert(
		ParserProjectCollection,
		query,
		update,
	)

	return err
}

func FindParametersForVersion(v *Version) ([]patch.Parameter, error) {
	pp, err := ParserProjectFindOne(ParserProjectById(v.Id).WithFields(ParserProjectParametersKey))
	if err != nil {
		return nil, errors.Wrap(err, "finding parser project")
	}
	return pp.GetParameters(), nil
}

func FindExpansionsForVariant(v *Version, variant string) (util.Expansions, error) {
	pp, err := ParserProjectFindOne(ParserProjectById(v.Id).WithFields(ParserProjectBuildVariantsKey, ParserProjectAxesKey))
	if err != nil {
		return nil, errors.Wrap(err, "finding parser project")
	}

	bvs, errs := GetVariantsWithMatrices(nil, pp.Axes, pp.BuildVariants)
	if len(errs) > 0 {
		catcher := grip.NewBasicCatcher()
		catcher.Extend(errs)
		return nil, catcher.Resolve()
	}
	for _, bv := range bvs {
		if bv.Name == variant {
			return bv.Expansions, nil
		}
	}
	return nil, errors.New("could not find variant")
}

func checkConfigNumberQuery(id string, configNum int) bson.M {
	q := bson.M{ParserProjectIdKey: id}
	if configNum == 0 {
		q["$or"] = []bson.M{
			{ParserProjectConfigNumberKey: bson.M{"$exists": false}},
			{ParserProjectConfigNumberKey: configNum},
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
		return err
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
		return errors.Wrapf(err, "upserting parser project '%s'", pp.Id)
	}
	return nil
}

func (pp *ParserProject) GetParameters() []patch.Parameter {
	res := []patch.Parameter{}
	for _, param := range pp.Parameters {
		res = append(res, param.Parameter)
	}
	return res
}
