package model

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

// ParserProjectStorage is an interface for accessing the parser project.
type ParserProjectStorage interface {
	// FindOneByID finds a parser project using its ID. Implementations may or
	// may not respect the context.
	FindOneByID(ctx context.Context, id string) (*ParserProject, error)
	// FindOneByIDWithFields finds a parser project using its ID and returns the
	// parser project with only the requested fields populated.
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
func GetParserProjectStorage(settings *evergreen.Settings, method ParserProjectStorageMethod) (ParserProjectStorage, error) {
	switch method {
	case "", ProjectStorageMethodDB:
		return ParserProjectDBStorage{}, nil
	case ProjectStorageMethodS3:
		return nil, errors.New("TODO (EVG-17537): implement")
	default:
		return nil, errors.Errorf("unrecognized parser project storage method '%s'", method)
	}
}
