package db

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// Q holds all information necessary to execute a query
type Q struct {
	filter     interface{} // should be bson.D or bson.M
	projection interface{} // should be bson.D or bson.M
	sort       []string
	skip       int
	limit      int
}

// Query creates a db.Q for the given MongoDB query. The filter
// can be a struct, bson.D, bson.M, nil, etc.
func Query(filter interface{}) Q {
	return Q{filter: filter}
}

func (q Q) Filter(filter interface{}) Q {
	q.filter = filter
	return q
}

func (q Q) Project(projection interface{}) Q {
	q.projection = projection
	return q
}

func (q Q) WithFields(fields ...string) Q {
	projection := map[string]int{}
	for _, f := range fields {
		projection[f] = 1
	}
	q.projection = projection
	return q
}

func (q Q) WithoutFields(fields ...string) Q {
	projection := map[string]int{}
	for _, f := range fields {
		projection[f] = 0
	}
	q.projection = projection
	return q
}

func (q Q) Sort(sort []string) Q {
	q.sort = sort
	return q
}

func (q Q) Skip(skip int) Q {
	q.skip = skip
	return q
}

func (q Q) Limit(limit int) Q {
	q.limit = limit
	return q
}

// FindOneQ runs a Q query against the given collection, applying the results to "out."
// Only reads one document from the DB.
func FindOneQ(collection string, q Q, out interface{}) error {
	return FindOne(
		collection,
		q.filter,
		q.projection,
		q.sort,
		out,
	)
}

// FindAllQ runs a Q query against the given collection, applying the results to "out."
func FindAllQ(collection string, q Q, out interface{}) error {
	return errors.WithStack(FindAll(
		collection,
		q.filter,
		q.projection,
		q.sort,
		q.skip,
		q.limit,
		out,
	))
}

// CountQ runs a Q count query against the given collection.
func CountQ(collection string, q Q) (int, error) {
	return Count(collection, q.filter)
}

//RemoveAllQ removes all docs that satisfy the query
func RemoveAllQ(collection string, q Q) error {
	return Remove(collection, q.filter)
}

// implement custom marshaller interfaces to prevent the bug where we
// pass a db.Q instead of a bson document, leading to great
// sadness. The real solution is to get rid of db.Q objects entirely.

func (q Q) SetBSON(_ bson.Raw) error      { panic("should never marshal db.Q objects to bson") }
func (q Q) GetBSON() (interface{}, error) { panic("should never marshal db.Q objects to bson") }
func (q Q) UnmarshalBSON(_ []byte) error  { panic("should never marshal db.Q objects to bson") }
func (q Q) MarshalBSON() ([]byte, error)  { panic("should never marshal db.Q objects to bson") }
