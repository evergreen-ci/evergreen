// Package mock contains mocked implementations of the interfaces
// defined in the anser package.
//
// These implementations expose all internals and do not have external
// dependencies. Indeed they should have interface-definition-only
// dependencies on other anser packages.
package mock

import (
	"errors"
	"reflect"

	"github.com/mongodb/anser/db"
)

type Session struct {
	DBs    map[string]*Database
	URI    string
	Closed bool
}

func NewSession() *Session {
	return &Session{
		DBs: make(map[string]*Database),
	}
}

func (s *Session) Clone() db.Session { return s }
func (s *Session) Copy() db.Session  { return s }
func (s *Session) Close()            { s.Closed = true }
func (s *Session) DB(n string) db.Database {
	if _, ok := s.DBs[n]; !ok {
		s.DBs[n] = &Database{
			Collections: make(map[string]*Collection),
		}

	}
	return s.DBs[n]
}

type Database struct {
	Collections map[string]*Collection
	DBName      string
}

func (d *Database) Name() string { return d.DBName }
func (d *Database) C(n string) db.Collection {
	if _, ok := d.Collections[n]; !ok {
		d.Collections[n] = &Collection{}
	}

	return d.Collections[n]
}

type Collection struct {
	Name         string
	InsertedDocs []interface{}
	UpdatedIds   []interface{}
	FailWrites   bool
	Queries      []*Query
	Pipelines    []*Pipeline
	NumDocs      int
	QueryError   error
}

func (c *Collection) Pipe(p interface{}) db.Results {
	pm := &Pipeline{Pipe: p}
	c.Pipelines = append(c.Pipelines, pm)
	return pm
}
func (c *Collection) Find(q interface{}) db.Query {
	qm := &Query{Query: q, Error: c.QueryError}
	c.Queries = append(c.Queries, qm)
	return qm
}
func (c *Collection) FindId(q interface{}) db.Query {
	qm := &Query{Query: q, Error: c.QueryError}
	c.Queries = append(c.Queries, qm)
	return qm
}
func (c *Collection) Count() (int, error)                                { return c.NumDocs, nil }
func (c *Collection) Update(q, u interface{}) error                      { return nil }
func (c *Collection) UpdateAll(q, u interface{}) (*db.ChangeInfo, error) { return &db.ChangeInfo{}, nil }
func (c *Collection) Remove(q interface{}) error                         { return nil }
func (c *Collection) RemoveAll(q interface{}) (*db.ChangeInfo, error)    { return &db.ChangeInfo{}, nil }
func (c *Collection) RemoveId(id interface{}) error                      { return nil }
func (c *Collection) Insert(docs ...interface{}) error                   { c.InsertedDocs = docs; return nil }
func (c *Collection) Upsert(q, u interface{}) (*db.ChangeInfo, error)    { return &db.ChangeInfo{}, nil }
func (c *Collection) UpsertId(id, u interface{}) (*db.ChangeInfo, error) {
	if c.FailWrites {
		return nil, errors.New("writes fail")
	}
	c.InsertedDocs = append(c.InsertedDocs, u)
	return &db.ChangeInfo{0, 0, id}, nil
}

func (c *Collection) UpdateId(id, u interface{}) error {
	if c.FailWrites {
		return errors.New("writes fail")
	}
	c.UpdatedIds = append(c.UpdatedIds, id)
	return nil
}

type Query struct {
	Query    interface{}
	Project  interface{}
	SortKeys []string
	NumLimit int
	NumSkip  int
	Error    error
	CountNum int
}

func (q *Query) Count() (int, error)           { return q.CountNum, q.Error }
func (q *Query) Limit(n int) db.Query          { q.NumLimit = n; return q }
func (q *Query) Select(p interface{}) db.Query { q.Project = p; return q }
func (q *Query) Skip(n int) db.Query           { q.NumSkip = n; return q }
func (q *Query) Iter() db.Iterator             { return &Iterator{Error: q.Error, Query: q} }
func (q *Query) One(r interface{}) error       { return q.Error }
func (q *Query) All(r interface{}) error       { return q.Error }
func (q *Query) Sort(keys ...string) db.Query  { q.SortKeys = keys; return q }

type Iterator struct {
	Query       *Query
	Pipeline    *Pipeline
	ShouldIter  bool
	Error       error
	NumIterated int
	Results     []interface{}
}

func (i *Iterator) Close() error { return i.Error }
func (i *Iterator) Err() error   { return i.Error }
func (i *Iterator) Next(out interface{}) bool {
	if i.ShouldIter {
		outVal := reflect.ValueOf(out)
		if i.NumIterated >= len(i.Results) {
			return false
		}

		outVal.Elem().Set(reflect.ValueOf(i.Results[i.NumIterated]).Elem())
		i.NumIterated++
		return true
	}

	return false
}

type Pipeline struct {
	Pipe  interface{}
	Error error
}

func (p *Pipeline) Iter() db.Iterator       { return &Iterator{Pipeline: p} }
func (p *Pipeline) All(r interface{}) error { return p.Error }
func (p *Pipeline) One(r interface{}) error { return p.Error }
