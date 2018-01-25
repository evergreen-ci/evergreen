package db

import "time"

// Session provides a subset of the functionality of the *mgo.Session
// type.
type Session interface {
	Clone() Session
	Copy() Session
	Close()
	DB(string) Database
	SetSocketTimeout(time.Duration)
}

// Database provides a very limited subset of the mgo.DB type.
type Database interface {
	Name() string
	C(string) Collection
}

// Collection provides access to the common query functionality of the
// mgo.Collection type.
type Collection interface {
	Pipe(interface{}) Results
	Find(interface{}) Query
	FindId(interface{}) Query
	Count() (int, error)
	Insert(...interface{}) error
	Upsert(interface{}, interface{}) (*ChangeInfo, error)
	UpsertId(interface{}, interface{}) (*ChangeInfo, error)
	Update(interface{}, interface{}) error
	UpdateId(interface{}, interface{}) error
	UpdateAll(interface{}, interface{}) (*ChangeInfo, error)
	Remove(interface{}) error
	RemoveId(interface{}) error
	RemoveAll(interface{}) (*ChangeInfo, error)
}

type Query interface {
	Count() (int, error)
	Limit(int) Query
	Select(interface{}) Query
	Skip(n int) Query
	Sort(...string) Query

	Results
}

// Iterator is a more narrow subset of mgo's Iter type that
// provides the opportunity to mock results, and avoids a strict
// dependency between mgo and migrations definitions.
type Iterator interface {
	Next(interface{}) bool
	Close() error
	Err() error
}

// Results reflect the output of a database operation and is part of
// the query interface, and is returned by the pipeline (e.g
// aggregation operation.)
type Results interface {
	All(interface{}) error
	One(interface{}) error
	Iter() Iterator
}

// Document is, like bson.M, a wrapper for an un-ordered map type
type Document map[string]interface{}
