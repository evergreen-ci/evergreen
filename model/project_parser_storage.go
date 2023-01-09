package model

import "context"

// ParserProjectStorage is an interface for accessing the parser project.
type ParserProjectStorage interface {
	// FindOneByID finds a parser project using its unique identifier.
	// Implementations may or may not respect the context.
	FindOneByID(ctx context.Context, id string) (*ParserProject, error)
	// FindOneByIDWithFields finds a parser project using its unique identifier
	// and returns the parser project with only the requested fields populated.
	// Implementations may or may not respect the context.
	FindOneByIDWithFields(ctx context.Context, id string, fields ...string) (*ParserProject, error)
	// UpsertOneByID replaces a parser project by ID if the parser project given
	// by the ID already exists. If it does not exist yet, it inserts a new
	// parser project.
	UpsertOneByID(ctx context.Context, id string, pp *ParserProject) error
}

// kim: TODO: implement
func GetParserProjectStorage(method ParserProjectStorageMethod) ParserProjectStorage {
	return nil
}
