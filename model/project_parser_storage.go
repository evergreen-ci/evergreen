package model

import (
	"context"

	"github.com/evergreen-ci/evergreen"
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
