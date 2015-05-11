package db

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

var (
	NoProjection = bson.M{}
	NoSort       = []string{}
	NoSkip       = 0
	NoLimit      = 0
)

// Insert inserts the specified item into the specified collection.
func Insert(collection string, item interface{}) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return nil
	}
	defer session.Close()

	return db.C(collection).Insert(item)
}

// Clear removes all documents from a specified collection.
func Clear(collection string) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()
	_, err = db.C(collection).RemoveAll(bson.M{})
	return err
}

// ClearCollections clears all documents from all the specified collections, returning an error
// immediately if clearing any one of them fails.
func ClearCollections(collections ...string) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()
	for _, collection := range collections {
		_, err = db.C(collection).RemoveAll(bson.M{})
		if err != nil {
			return fmt.Errorf("Couldn't clear collection '%v': %v", collection, err)
		}
	}
	return nil
}

// Remove removes one item matching the query from the specified collection.
func Remove(collection string, query interface{}) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()

	return db.C(collection).Remove(query)
}

// RemoveAll removes all items matching the query from the specified collection.
func RemoveAll(collection string, query interface{}) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()

	_, err = db.C(collection).RemoveAll(query)
	return err
}

// FindOne finds one item from the specified collection and unmarshals it into the
// provided interface, which must be a pointer.
func FindOne(collection string, query interface{},
	projection interface{}, sort []string, out interface{}) error {

	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error establishing db connection: %v",
			err)
		return err
	}
	defer session.Close()

	q := db.C(collection).Find(query).Select(projection)
	if len(sort) != 0 {
		q = q.Sort(sort...)
	}
	return q.One(out)
}

// FindAll finds the items from the specified collection and unmarshals them into the
// provided interface, which must be a slice.
func FindAll(collection string, query interface{},
	projection interface{}, sort []string, skip int, limit int,
	out interface{}) error {

	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error establishing db connection: %v",
			err)
		return err
	}
	defer session.Close()

	q := db.C(collection).Find(query).Select(projection)
	if len(sort) != 0 {
		q = q.Sort(sort...)
	}
	return q.Skip(skip).Limit(limit).All(out)
}

// Update updates one matching document in the collection.
func Update(collection string, query interface{},
	update interface{}) error {

	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error establishing db connection: %v",
			err)
		return err
	}
	defer session.Close()

	return db.C(collection).Update(query, update)
}

// UpdateId updates one _id-matching document in the collection.
func UpdateId(collection string, id, update interface{}) error {

	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error establishing db connection: %v",
			err)
		return err
	}
	defer session.Close()

	return db.C(collection).UpdateId(id, update)
}

// UpdateAll updates all matching documents in the collection.
func UpdateAll(collection string, query interface{},
	update interface{}) (*mgo.ChangeInfo, error) {

	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error establishing db connection: %v",
			err)
		return nil, err
	}
	defer session.Close()

	return db.C(collection).UpdateAll(query, update)
}

// Upsert run the specified update against the collection as an upsert operation.
func Upsert(collection string, query interface{},
	update interface{}) (*mgo.ChangeInfo, error) {

	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error establishing db connection: %v",
			err)
		return nil, err
	}
	defer session.Close()

	return db.C(collection).Upsert(query, update)
}

// Count run a count command with the specified query against the collection.
func Count(collection string, query interface{}) (int, error) {

	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error establishing db connection: %v",
			err)
		return 0, err
	}
	defer session.Close()

	return db.C(collection).Find(query).Count()
}

// FindAndModify runs the specified query and change against the collection,
// unmarshaling the result into the specified interface.
func FindAndModify(collection string, query interface{},
	change mgo.Change, out interface{}) (*mgo.ChangeInfo, error) {

	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error establishing db connection: %v",
			err)
		return nil, err
	}
	defer session.Close()

	return db.C(collection).Find(query).Apply(change, out)

}

// Aggregate runs an aggregation pipeline on a collection and unmarshals
// the results to the given "out" interface (usually a pointer
// to an array of structs/bson.M)
func Aggregate(collection string, pipeline interface{}, out interface{}) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error establishing db connection: %v", err)
		return err
	}
	defer session.Close()

	pipe := db.C(collection).Pipe(pipeline)
	return pipe.All(out)
}
