package model

import (
	"context"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
)

const (
	ParserProjectCollection = "parser_projects"
)

var (
	ParserProjectIdKey                = bsonutil.MustHaveTag(ParserProject{}, "Id")
	ParserProjectStepbackKey          = bsonutil.MustHaveTag(ParserProject{}, "Stepback")
	ParserProjectPreErrorFailsTaskKey = bsonutil.MustHaveTag(ParserProject{}, "PreErrorFailsTask")
	ParserProjectOomTracker           = bsonutil.MustHaveTag(ParserProject{}, "OomTracker")
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
	ParserProjectTimeoutKey           = bsonutil.MustHaveTag(ParserProject{}, "Timeout")
	ParserProjectCallbackTimeoutKey   = bsonutil.MustHaveTag(ParserProject{}, "CallbackTimeout")
	ParserProjectModulesKey           = bsonutil.MustHaveTag(ParserProject{}, "Modules")
	ParserProjectBuildVariantsKey     = bsonutil.MustHaveTag(ParserProject{}, "BuildVariants")
)

// ParserProjectFindOneById returns the parser project from the DB for the
// given ID.
func parserProjectFindOneById(ctx context.Context, id string) (*ParserProject, error) {
	pp, err := parserProjectFindOne(ctx, parserProjectById(id))
	if err != nil {
		return nil, err
	}
	if pp != nil && pp.Functions == nil {
		pp.Functions = map[string]*YAMLCommandSet{}
	}
	return pp, nil
}

// parserProjectFindOne finds a parser project in the DB with a given query.
func parserProjectFindOne(ctx context.Context, query db.Q) (*ParserProject, error) {
	project := &ParserProject{}
	err := db.FindOneQ(ctx, ParserProjectCollection, query, project)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return project, err
}

// parserProjectById returns a DB query to find a parser project by ID.
func parserProjectById(id string) db.Q {
	return db.Query(bson.M{ParserProjectIdKey: id})
}

// parserProjectReplaceOne updates one parser project in the DB.
func parserProjectReplaceOne(ctx context.Context, query any, replacement any) error {
	_, err := db.Replace(
		ctx,
		ParserProjectCollection,
		query,
		replacement,
	)

	return err
}

// ParserProjectDBStorage implements the ParserProjectStorage interface to
// access parser projects stored in the DB.
type ParserProjectDBStorage struct{}

// FindOneByID finds a parser project from the DB by its ID. This ignores the
// context parameter.
func (s ParserProjectDBStorage) FindOneByID(ctx context.Context, id string) (*ParserProject, error) {
	return parserProjectFindOneById(ctx, id)
}

// FindOneByIDWithFields returns the parser project from the DB with only the
// requested fields populated. This may be more efficient than fetching the
// entire parser project. This ignores the context parameter.
func (s ParserProjectDBStorage) FindOneByIDWithFields(ctx context.Context, id string, fields ...string) (*ParserProject, error) {
	return parserProjectFindOne(ctx, parserProjectById(id).WithFields(fields...))
}

// UpsertOne replaces a parser project in the DB if one exists with the same ID.
// Otherwise, if it does not exist yet, it inserts a new parser project.
func (s ParserProjectDBStorage) UpsertOne(ctx context.Context, pp *ParserProject) error {
	return parserProjectReplaceOne(ctx, bson.M{ParserProjectIdKey: pp.Id}, pp)
}
