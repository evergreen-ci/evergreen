package db

import (
	"context"
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

// FindOneQ runs a Q query against the given collection, applying the results to "out."
// Only reads one document from the DB.
func FindOneQ(collection string, q Q, out any) error {
	return FindOneQContext(context.Background(), collection, q, out)
}

// FindOneQContext runs a Q query against the given collection, applying the results to "out."
// Only reads one document from the DB.
func FindOneQContext(ctx context.Context, collection string, q Q, out any) error {
	if q.maxTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, q.maxTime)
		defer cancel()
	}

	session, db, err := GetGlobalSessionFactory().GetContextSession(ctx)
	if err != nil {
		return err
	}
	defer session.Close()

	return db.C(collection).
		Find(q.filter).
		Select(q.projection).
		Sort(q.sort...).
		Skip(q.skip).
		Limit(1).
		Hint(q.hint).
		One(out)
}

// FindAllQ runs a Q query against the given collection, applying the results to "out."
func FindAllQ(collection string, q Q, out any) error {
	return FindAllQContext(context.Background(), collection, q, out)
}

func FindAllQContext(ctx context.Context, collection string, q Q, out any) error {
	if q.maxTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, q.maxTime)
		defer cancel()
	}

	session, db, err := GetGlobalSessionFactory().GetContextSession(ctx)
	if err != nil {
		return err
	}
	defer session.Close()

	return db.C(collection).
		Find(q.filter).
		Select(q.projection).
		Sort(q.sort...).
		Skip(q.skip).
		Limit(q.limit).
		Hint(q.hint).
		All(out)
}

// CountQ runs a Q count query against the given collection.
func CountQ(collection string, q Q) (int, error) {
	return Count(collection, q.filter)
}

// CountQ runs a Q count query against the given collection.
func CountQContext(ctx context.Context, collection string, q Q) (int, error) {
	return CountContext(ctx, collection, q.filter)
}

// RemoveAllQ removes all docs that satisfy the query
func RemoveAllQ(ctx context.Context, collection string, q Q) error {
	return Remove(ctx, collection, q.filter)
}

// implement custom marshaller interfaces to prevent the bug where we
// pass a db.Q instead of a bson document, leading to great
// sadness. The real solution is to get rid of db.Q objects entirely.

func (q Q) SetBSON(_ bson.Raw) error     { panic("should never marshal db.Q objects to bson") }
func (q Q) GetBSON() (any, error)        { panic("should never marshal db.Q objects to bson") }
func (q Q) UnmarshalBSON(_ []byte) error { panic("should never marshal db.Q objects to bson") }
func (q Q) MarshalBSON() ([]byte, error) { panic("should never marshal db.Q objects to bson") }
