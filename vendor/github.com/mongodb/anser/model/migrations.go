// Package model provides public data structures and interfaces to represent
// migration operations.
//
// The model pacakge contains no non-trivial implementations and has no
// dependencies outside of the standard library.
package model

// MigrationDefinitionSimple defines a single-document operation, performing a
// single document update that operates on one collection.
type Simple struct {
	// ID returns the _id field of the document that is the
	// subject of the migration.
	ID interface{} `bson:"id" json:"id" yaml:"id"`

	// Update is a specification for the update
	// operation. This is converted to a bson.M{} (but is typed as
	// a map to allow migration implementations to *not* depend upon
	// bson.M
	Update map[string]interface{} `bson:"update" json:"update" yaml:"update"`

	// Migration holds the ID of the migration operation,
	// typically the name of the class of migration and
	Migration string `bson:"migration_id" json:"migration_id" yaml:"migration_id"`

	// Namespace holds a struct that describes which database and
	// collection where the migration should run
	Namespace Namespace `bson:"namespace" json:"namespace" yaml:"namespace"`
}

// MigrationDefinitionManual defines an operations that runs an arbitrary
// function given the input of an mgo.Session pointer and
type Manual struct {
	// ID returns the _id field of the document that is the
	// input of the migration.
	ID interface{} `bson:"id" json:"id" yaml:"id"`

	// The name of the migration function to run. This function
	// must be defined in the migration registry
	OperationName string `bson:"op_name" json:"op_name" yaml:"op_name"`

	// Migration holds the ID of the migration operation,
	// typically the name of the class of migration and
	Migration string `bson:"migration_id" json:"migration_id" yaml:"migration_id"`

	// Namespace holds a struct that describes which database and
	// collection where the query for the input document should run.
	Namespace Namespace `bson:"namespace" json:"namespace" yaml:"namespace"`
}

// MigrationDefinitionStream is a migration definition form that has, that can
// processes a stream of documents, using an implementation of the
// DocumentProducer interface.
type Stream struct {
	Query map[string]interface{} `bson:"query" json:"query" yaml:"query"`

	// The name of a registered DocumentProcessor implementation.
	// Because the producer isn't versioned, changes to the
	// implementation of the Producer between migration runs, may
	// complicate idempotency requirements.
	ProcessorName string `bson:"producer" json:"producer" yaml:"producer"`

	// Migration holds the ID of the migration operation,
	// typically the name of the class of migration and
	Migration string `bson:"migration_id" json:"migration_id" yaml:"migration_id"`

	// Namespace holds a struct that describes which database and
	// collection where the query for the input document should run.
	Namespace Namespace `bson:"namespace" json:"namespace" yaml:"namespace"`
}
