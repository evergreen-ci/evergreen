package db

// Q holds all information necessary to execute a query
type Q struct {
	filter     interface{} // should be bson.D or bson.M
	projection interface{} // should be bson.D or bson.M
	sort       []string
	skip       int
	limit      int
}

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
	return FindAll(
		collection,
		q.filter,
		q.projection,
		q.sort,
		q.skip,
		q.limit,
		out,
	)
}

// CountQ runs a Q count query against the given collection.
func CountQ(collection string, q Q) (int, error) {
	return Count(collection, q.filter)
}
