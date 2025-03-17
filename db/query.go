package db

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// Q holds all information necessary to execute a query
type Q struct {
	filter     any // should be bson.D or bson.M
	projection any // should be bson.D or bson.M
	sort       []string
	skip       int
	limit      int
	hint       any // should be bson.D of the index keys or string name of the hint
	maxTime    time.Duration
}

// Query creates a db.Q for the given MongoDB query. The filter
// can be a struct, bson.D, bson.M, nil, etc.
func Query(filter any) Q {
	return Q{filter: filter}
}

func (q Q) Filter(filter any) Q {
	q.filter = filter
	return q
}

func (q Q) Project(projection any) Q {
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

// MaxTime sets the maxTime for a query to time out the query on the db server.
func (q Q) MaxTime(maxTime time.Duration) Q {
	q.maxTime = maxTime
	return q
}

// Hint sets the hint for a query to determine what index will be used. The hint
// can be either the index as an ordered document of the keys or a
func (q Q) Hint(hint any) Q {
	q.hint = hint
	return q
}

// implement custom marshaller interfaces to prevent the bug where we
// pass a db.Q instead of a bson document, leading to great
// sadness. The real solution is to get rid of db.Q objects entirely.

func (q Q) SetBSON(_ bson.Raw) error     { panic("should never marshal db.Q objects to bson") }
func (q Q) GetBSON() (any, error)        { panic("should never marshal db.Q objects to bson") }
func (q Q) UnmarshalBSON(_ []byte) error { panic("should never marshal db.Q objects to bson") }
func (q Q) MarshalBSON() ([]byte, error) { panic("should never marshal db.Q objects to bson") }
