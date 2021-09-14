package db

import (
	"time"

	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
)

// WrapSession takes an mgo.Session and returns an equivalent session object.
func WrapSession(s *mgo.Session) Session { return sessionLegacyWrapper{Session: s} }

type sessionLegacyWrapper struct {
	*mgo.Session
}

func (s sessionLegacyWrapper) Clone() Session       { return sessionLegacyWrapper{s.Session.Clone()} }
func (s sessionLegacyWrapper) Copy() Session        { return sessionLegacyWrapper{s.Session.Copy()} }
func (s sessionLegacyWrapper) DB(d string) Database { return databaseLegacyWrapper{s.Session.DB(d)} }
func (s sessionLegacyWrapper) Error() error         { return nil }

type databaseLegacyWrapper struct {
	*mgo.Database
}

func (d databaseLegacyWrapper) Name() string { return d.Database.Name }

func (d databaseLegacyWrapper) C(n string) Collection {
	return collectionLegacyWrapper{Collection: d.Database.C(n)}
}

type collectionLegacyWrapper struct {
	*mgo.Collection
}

func (c collectionLegacyWrapper) Bulk() Bulk { return bulkLegacyWrapper{c.Collection.Bulk()} }

func (c collectionLegacyWrapper) Pipe(p interface{}) Aggregation {
	return pipelineLegacyWrapper{c.Collection.Pipe(p)}
}

func (c collectionLegacyWrapper) Find(q interface{}) Query {
	return queryLegacyWrapper{c.Collection.Find(q)}
}

func (c collectionLegacyWrapper) FindId(id interface{}) Query {
	return queryLegacyWrapper{c.Collection.FindId(id)}
}

func (q queryLegacyWrapper) Sort(keys ...string) Query {
	return queryLegacyWrapper{q.Query.Sort(keys...)}
}

func (c collectionLegacyWrapper) RemoveAll(q interface{}) (*ChangeInfo, error) {
	i, err := c.Collection.RemoveAll(q)
	return buildChangeInfo(i), errors.WithStack(err)
}

func (c collectionLegacyWrapper) UpdateAll(q, u interface{}) (*ChangeInfo, error) {
	i, err := c.Collection.UpdateAll(q, u)
	return buildChangeInfo(i), errors.WithStack(err)
}

func (c collectionLegacyWrapper) Upsert(q, u interface{}) (*ChangeInfo, error) {
	i, err := c.Collection.Upsert(q, u)
	return buildChangeInfo(i), errors.WithStack(err)
}

func (c collectionLegacyWrapper) UpsertId(q, u interface{}) (*ChangeInfo, error) {
	i, err := c.Collection.UpsertId(q, u)
	return buildChangeInfo(i), errors.WithStack(err)
}

type queryLegacyWrapper struct {
	*mgo.Query
}

func (q queryLegacyWrapper) Iter() Iterator             { return q.Query.Iter() }
func (q queryLegacyWrapper) Limit(n int) Query          { return queryLegacyWrapper{q.Query.Limit(n)} }
func (q queryLegacyWrapper) Skip(n int) Query           { return queryLegacyWrapper{q.Query.Skip(n)} }
func (q queryLegacyWrapper) Select(p interface{}) Query { return queryLegacyWrapper{q.Query.Select(p)} }

// Hint is an unsupported no-op because the legacy driver's support for hints is
// limited.
func (q queryLegacyWrapper) Hint(h interface{}) Query { return queryLegacyWrapper{q.Query} }

func (q queryLegacyWrapper) Apply(ch Change, result interface{}) (*ChangeInfo, error) {
	i, err := q.Query.Apply(buildChange(ch), result)
	return buildChangeInfo(i), errors.WithStack(err)
}

func (q queryLegacyWrapper) MaxTime(d time.Duration) Query {
	return queryLegacyWrapper{q.Query.SetMaxTime(d)}
}

type pipelineLegacyWrapper struct {
	*mgo.Pipe
}

func (p pipelineLegacyWrapper) Iter() Iterator { return p.Pipe.Iter() }

// Hint and MaxTime are unsupported no-ops because the legacy driver's support for
// hints and max time is limited.
func (p pipelineLegacyWrapper) Hint(hint interface{}) Aggregation   { return p }
func (p pipelineLegacyWrapper) MaxTime(d time.Duration) Aggregation { return p }

type bulkLegacyWrapper struct {
	*mgo.Bulk
}

func (b bulkLegacyWrapper) Run() (*BulkResult, error) {
	r, err := b.Bulk.Run()
	return buildBulkResult(r), errors.WithStack(err)
}

func buildChangeInfo(i *mgo.ChangeInfo) *ChangeInfo {
	if i == nil {
		return nil
	}
	return &ChangeInfo{i.Updated, i.Removed, i.UpsertedId}
}

func buildChange(ch Change) mgo.Change {
	return mgo.Change{
		Update:    ch.Update,
		Upsert:    ch.Upsert,
		Remove:    ch.Remove,
		ReturnNew: ch.ReturnNew,
	}
}

func buildBulkResult(r *mgo.BulkResult) *BulkResult {
	if r == nil {
		return nil
	}

	return &BulkResult{
		Matched:  r.Matched,
		Modified: r.Modified,
	}
}
