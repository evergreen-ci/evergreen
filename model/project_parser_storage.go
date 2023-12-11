package model

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// ParserProjectStorage is an interface for accessing the parser project.
type ParserProjectStorage interface {
	// FindOneByID finds a parser project using its ID. If the parser project
	// does not exist in the underlying storage, implementations must return a
	// nil parser project and nil error. Implementations may or may not respect
	// the context.
	FindOneByID(ctx context.Context, id string) (*ParserProject, error)
	// FindOneByIDWithFields finds a parser project using its ID and returns the
	// parser project with at least the requested fields populated.
	// Implementations may choose to return more fields than those explicitly
	// requested. If the parser project does not exist in the underlying
	// storage, implementations must return a nil parser project and nil error.
	// Implementations may or may not respect the context.
	FindOneByIDWithFields(ctx context.Context, id string, fields ...string) (*ParserProject, error)
	// UpsertOne replaces a parser project if the parser project with the
	// same ID already exists. If it does not exist yet, it inserts a new parser
	// project.
	UpsertOne(ctx context.Context, pp *ParserProject) error
	// Close cleans up the accessor to the parser project storage. Users of
	// ParserProjectStorage implementations must call Close once they are
	// finished using it.
	Close(ctx context.Context) error
}

// GetParserProjectStorage returns the parser project storage mechanism to
// access the persistent copy of it. Users of the returned ParserProjectStorage
// must call Close once they are finished using it.
func GetParserProjectStorage(settings *evergreen.Settings, method evergreen.ParserProjectStorageMethod) (ParserProjectStorage, error) {
	switch method {
	case "", evergreen.ProjectStorageMethodDB:
		return ParserProjectDBStorage{}, nil
	case evergreen.ProjectStorageMethodS3:
		return NewParserProjectS3Storage(settings.Providers.AWS.ParserProject)
	default:
		return nil, errors.Errorf("unrecognized parser project storage method '%s'", method)
	}
}

// ParserProjectFindOneByID is a convenience wrapper to find one parser project
// by ID from persistent storage.
func ParserProjectFindOneByID(ctx context.Context, settings *evergreen.Settings, method evergreen.ParserProjectStorageMethod, id string) (*ParserProject, error) {
	ppStorage, err := GetParserProjectStorage(settings, method)
	if err != nil {
		return nil, errors.Wrap(err, "getting parser project storage")
	}
	defer ppStorage.Close(ctx)
	return ppStorage.FindOneByID(ctx, id)
}

// ParserProjectFindOneByIDWithFields is a convenience wrapper to find one
// parser project by ID from persistent storage with the given fields populated.
func ParserProjectFindOneByIDWithFields(ctx context.Context, settings *evergreen.Settings, method evergreen.ParserProjectStorageMethod, id string, fields ...string) (*ParserProject, error) {
	ppStorage, err := GetParserProjectStorage(settings, method)
	if err != nil {
		return nil, errors.Wrap(err, "getting parser project storage")
	}
	defer ppStorage.Close(ctx)
	return ppStorage.FindOneByIDWithFields(ctx, id, fields...)
}

// ParserProjectUpsertOne is a convenience wrapper to upsert one parser project
// to persistent storage.
func ParserProjectUpsertOne(ctx context.Context, settings *evergreen.Settings, method evergreen.ParserProjectStorageMethod, pp *ParserProject) error {
	ppStorage, err := GetParserProjectStorage(settings, method)
	if err != nil {
		return errors.Wrap(err, "getting parser project storage")
	}
	defer ppStorage.Close(ctx)
	return ppStorage.UpsertOne(ctx, pp)
}

// ParserProjectUpsertOneWithS3Fallback attempts to upsert the parser project
// into persistent storage using the given storage method. If it fails due to
// DB document size limitations, it will attempt to fall back to using S3
// to store it. If it succeeds, this returns the actual project storage method
// used to persist the parser project; otherwise, it returns the
// originally-requested storage method.
func ParserProjectUpsertOneWithS3Fallback(ctx context.Context, settings *evergreen.Settings, method evergreen.ParserProjectStorageMethod, pp *ParserProject) (evergreen.ParserProjectStorageMethod, error) {
	err := ParserProjectUpsertOne(ctx, settings, method, pp)
	if method == evergreen.ProjectStorageMethodS3 {
		return method, errors.Wrap(err, "upserting parser project into S3")
	}

	if !db.IsDocumentLimit(err) {
		return method, errors.Wrap(err, "upserting parser project into the DB")
	}

	newMethod := evergreen.ProjectStorageMethodS3
	if err := ParserProjectUpsertOne(ctx, settings, newMethod, pp); err != nil {
		return method, errors.Wrap(err, "falling back to upserting parser project into S3")
	}

	grip.Info(message.Fields{
		"message":            "successfully upserted parser project into S3 as fallback due to document size limitation",
		"parser_project":     pp.Id,
		"old_storage_method": method,
		"new_storage_method": newMethod,
	})

	return newMethod, nil
}
