package model

import "context"

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
}

// GetParserProjectStorage returns the parser project storage mechanism to
// access the persistent copy of it.
func GetParserProjectStorage(method ParserProjectStorageMethod) ParserProjectStorage {
	return ParserProjectDBStorage{}
}
