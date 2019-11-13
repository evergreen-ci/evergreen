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
	"sync"
	"time"

	"github.com/mongodb/anser/db"
)

type Session struct {
	DBs           map[string]*LegacyDatabase
	URI           string
	Closed        bool
	SocketTimeout time.Duration
	Mutex         sync.Mutex
}

func NewSession() *Session {
	return &Session{
		DBs: make(map[string]*LegacyDatabase),
	}
}

func (s *Session) Clone() db.Session { return s }
func (s *Session) Copy() db.Session  { return s }
func (s *Session) Error() error      { return nil }
func (s *Session) DB(n string) db.Database {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	if _, ok := s.DBs[n]; !ok {
		s.DBs[n] = &LegacyDatabase{
			Collections: make(map[string]*LegacyCollection),
		}

	}
	return s.DBs[n]
}

func (s *Session) SetSocketTimeout(d time.Duration) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	s.SocketTimeout = d
}

func (s *Session) Close() {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	s.Closed = true
}

type LegacyDatabase struct {
	Collections map[string]*LegacyCollection
	DBName      string
	Mutex       sync.Mutex
}

func (d *LegacyDatabase) Name() string {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()

	return d.DBName
}

func (d *LegacyDatabase) C(n string) db.Collection {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()

	if _, ok := d.Collections[n]; !ok {
		d.Collections[n] = &LegacyCollection{}
	}

	return d.Collections[n]
}

func (d *LegacyDatabase) DropDatabase() error {
	return nil
}

type LegacyCollection struct {
	Name         string
	InsertedDocs []interface{}
	UpdatedIds   []interface{}
	FailWrites   bool
	Queries      []*Query
	Pipelines    []*Pipeline
	NumDocs      int
	QueryError   error
	Mutex        sync.Mutex
}

func (c *LegacyCollection) Pipe(p interface{}) db.Results {
	pm := &Pipeline{Pipe: p}

	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	c.Pipelines = append(c.Pipelines, pm)
	return pm
}
func (c *LegacyCollection) Find(q interface{}) db.Query {
	qm := &Query{Query: q, Error: c.QueryError}

	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	c.Queries = append(c.Queries, qm)
	return qm
}
func (c *LegacyCollection) FindId(q interface{}) db.Query {
	qm := &Query{Query: q, Error: c.QueryError}

	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	c.Queries = append(c.Queries, qm)
	return qm
}

func (c *LegacyCollection) DropCollection() error { return nil }
func (c *LegacyCollection) Bulk() db.Bulk         { return nil }
func (c *LegacyCollection) Count() (int, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	return c.NumDocs, nil
}
func (c *LegacyCollection) Update(q, u interface{}) error { return nil }
func (c *LegacyCollection) UpdateAll(q, u interface{}) (*db.ChangeInfo, error) {
	return &db.ChangeInfo{}, nil
}
func (c *LegacyCollection) Remove(q interface{}) error { return nil }
func (c *LegacyCollection) RemoveAll(q interface{}) (*db.ChangeInfo, error) {
	return &db.ChangeInfo{}, nil
}
func (c *LegacyCollection) RemoveId(id interface{}) error { return nil }
func (c *LegacyCollection) Insert(docs ...interface{}) error {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	c.InsertedDocs = docs
	return nil
}
func (c *LegacyCollection) Upsert(q, u interface{}) (*db.ChangeInfo, error) {
	return &db.ChangeInfo{}, nil
}
func (c *LegacyCollection) UpsertId(id, u interface{}) (*db.ChangeInfo, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	if c.FailWrites {
		return nil, errors.New("writes fail")
	}
	c.InsertedDocs = append(c.InsertedDocs, u)
	return &db.ChangeInfo{Updated: 0, Removed: 0, UpsertedId: id}, nil
}

func (c *LegacyCollection) UpdateId(id, u interface{}) error {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	if c.FailWrites {
		return errors.New("writes fail")
	}
	c.UpdatedIds = append(c.UpdatedIds, id)
	return nil
}

type Query struct {
	Query           interface{}
	Project         interface{}
	SortKeys        []string
	NumLimit        int
	NumSkip         int
	Error           error
	CountNum        int
	ApplyChangeSpec db.Change
	ApplyChangeInfo *db.ChangeInfo
}

func (q *Query) Count() (int, error)           { return q.CountNum, q.Error }
func (q *Query) Limit(n int) db.Query          { q.NumLimit = n; return q }
func (q *Query) Select(p interface{}) db.Query { q.Project = p; return q }
func (q *Query) Skip(n int) db.Query           { q.NumSkip = n; return q }
func (q *Query) Iter() db.Iterator             { return &Iterator{Error: q.Error, Query: q} }
func (q *Query) One(r interface{}) error       { return q.Error }
func (q *Query) All(r interface{}) error       { return q.Error }
func (q *Query) Sort(keys ...string) db.Query  { q.SortKeys = keys; return q }

func (q *Query) Apply(ch db.Change, r interface{}) (*db.ChangeInfo, error) {
	q.ApplyChangeSpec = ch
	return q.ApplyChangeInfo, q.Error
}

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
		if i.NumIterated >= len(i.Results) {
			return false
		}

		reflect.ValueOf(out).Elem().Set(reflect.ValueOf(i.Results[i.NumIterated]).Elem())
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
