package db

import (
	"github.com/mongodb/anser/model"
	"gopkg.in/mgo.v2/bson"
)

// Processor defines the process for processing a stream of
// documents using an Iterator, which resembles mgo's Iter
// operation.
type Processor interface {
	Load(model.Namespace, map[string]interface{}) Iterator
	Migrate(Iterator) error
}

// MigrationOperation defines the function object that performs
// the transformation in the manual migration migrations. Register
// these functions using RegisterMigrationOperation.
//
// Implementors of MigrationOperations are responsible for
// implementing idempotent operations.
type MigrationOperation func(Session, bson.RawD) error
