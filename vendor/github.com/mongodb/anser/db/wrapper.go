package db

import (
	mgo "gopkg.in/mgo.v2"
)

type sessionWrapper struct {
	*mgo.Session
}

type databaseWrapper struct {
	*mgo.Database
}

type collectionWrapper struct {
	*mgo.Collection
}

type queryWrapper struct {
	*mgo.Query
}

type pipelineWrapper struct {
	*mgo.Pipe
}

// WrapSession takes an mgo.Session and returns an equivalent session object.
func WrapSession(s *mgo.Session) Session                { return sessionWrapper{Session: s} }
func (s sessionWrapper) Clone() Session                 { return sessionWrapper{s.Session.Clone()} }
func (s sessionWrapper) Copy() Session                  { return sessionWrapper{s.Session.Copy()} }
func (s sessionWrapper) DB(d string) Database           { return databaseWrapper{s.Session.DB(d)} }
func (d databaseWrapper) Name() string                  { return d.Database.Name }
func (d databaseWrapper) C(n string) Collection         { return collectionWrapper{d.Database.C(n)} }
func (c collectionWrapper) Pipe(p interface{}) Results  { return pipelineWrapper{c.Collection.Pipe(p)} }
func (c collectionWrapper) Find(q interface{}) Query    { return queryWrapper{c.Collection.Find(q)} }
func (c collectionWrapper) FindId(id interface{}) Query { return queryWrapper{c.Collection.FindId(id)} }
func (p pipelineWrapper) Iter() Iterator                { return p.Pipe.Iter() }
func (q queryWrapper) Iter() Iterator                   { return q.Query.Iter() }
func (q queryWrapper) Limit(n int) Query                { return queryWrapper{q.Query.Limit(n)} }
func (q queryWrapper) Skip(n int) Query                 { return queryWrapper{q.Query.Skip(n)} }
func (q queryWrapper) Sort(keys ...string) Query        { return queryWrapper{q.Query.Sort(keys...)} }
func (q queryWrapper) Select(p interface{}) Query       { return queryWrapper{q.Query.Select(p)} }

func (c collectionWrapper) RemoveAll(q interface{}) (*ChangeInfo, error) {
	i, err := c.Collection.RemoveAll(q)
	return &ChangeInfo{Updated: i.Updated, Removed: i.Removed}, err
}

func (c collectionWrapper) UpdateAll(q, u interface{}) (*ChangeInfo, error) {
	i, err := c.Collection.UpdateAll(q, u)
	return &ChangeInfo{Updated: i.Updated, Removed: i.Removed}, err
}

func (c collectionWrapper) Upsert(q, u interface{}) (*ChangeInfo, error) {
	i, err := c.Collection.Upsert(q, u)
	return &ChangeInfo{i.Updated, i.Removed, i.UpsertedId}, err
}

func (c collectionWrapper) UpsertId(q, u interface{}) (*ChangeInfo, error) {
	i, err := c.Collection.UpsertId(q, u)
	return &ChangeInfo{i.Updated, i.Removed, i.UpsertedId}, err
}
