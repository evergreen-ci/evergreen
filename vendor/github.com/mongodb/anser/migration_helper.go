package anser

import (
	"context"

	"github.com/mongodb/amboy/job"
	"github.com/mongodb/anser/model"
)

// MigrationHelper is an interface embedded in all jobs as an
// "extended base" for migrations on top of the existing amboy.Base
// type which implements most job functionality.
//
// MigrationHelper implementations should not require construction:
// getter methods should initialize nil values at runtime.
type MigrationHelper interface {
	Env() Environment

	// Migrations need to record their state to help resolve
	// dependencies to the database.
	FinishMigration(context.Context, string, *job.Base)
	SaveMigrationEvent(context.Context, *model.MigrationMetadata) error

	// The migration helper provides a model/interface for
	// interacting with the database to check the state of a
	// migration operation, helpful in dependency approval.
	PendingMigrationOperations(context.Context, model.Namespace, map[string]interface{}) int
	GetMigrationEvents(context.Context, map[string]interface{}) MigrationMetadataIterator
}

// MigrationMetadataiterator wraps a query response for data about a migration.
type MigrationMetadataIterator interface {
	Next(context.Context) bool
	Item() *model.MigrationMetadata
	Err() error
	Close() error
}

func NewMigrationHelper(e Environment) MigrationHelper {
	if e.PreferClient() {
		return NewClientMigrationHelper(e)
	}

	return NewClientMigrationHelper(e)
	// return NewLegacyMigrationHelper(e)
}
