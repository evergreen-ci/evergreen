package db

import (
	"fmt"
	"io"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/mongo/driver"
)

var (
	NoProjection             = bson.M{}
	NoSort                   = []string{}
	NoSkip                   = 0
	NoLimit                  = 0
	NoHint       interface{} = nil
)

type SessionFactory interface {
	GetSession() (db.Session, db.Database, error)
}

type shimFactoryImpl struct {
	env evergreen.Environment
	db  string
}

func GetGlobalSessionFactory() SessionFactory {
	env := evergreen.GetEnvironment()
	return &shimFactoryImpl{
		env: env,
		db:  env.Settings().Database.DB,
	}
}

func (s *shimFactoryImpl) GetSession() (db.Session, db.Database, error) {
	if s.env == nil {
		return nil, nil, errors.New("undefined environment")
	}

	session := s.env.Session()
	if session == nil {
		return nil, nil, errors.New("session is not defined")
	}

	return session, session.DB(s.db), nil
}

// Insert inserts the specified item into the specified collection.
func Insert(collection string, item interface{}) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return errors.WithStack(err)
	}
	defer session.Close()

	return db.C(collection).Insert(item)
}

func InsertMany(collection string, items ...interface{}) error {
	if len(items) == 0 {
		return nil
	}
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return errors.WithStack(err)
	}
	defer session.Close()

	return db.C(collection).Insert(items...)
}

func InsertManyUnordered(c string, items ...interface{}) error {
	if len(items) == 0 {
		return nil
	}
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	_, err := env.DB().Collection(c).InsertMany(ctx, items, options.InsertMany().SetOrdered(false))

	return errors.WithStack(err)
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

// ClearCollections clears all documents from all the specified collections,
// returning an error immediately if clearing any one of them fails.
func ClearCollections(collections ...string) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()
	for _, collection := range collections {
		_, err := db.C(collection).RemoveAll(bson.M{})

		if err != nil {
			return errors.Wrapf(err, "Couldn't clear collection '%v'", collection)
		}
	}
	return nil
}

// DropCollections drops the specified collections, returning an error
// immediately if dropping any one of them fails.
func DropCollections(collections ...string) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()
	for _, coll := range collections {
		if err := db.C(coll).DropCollection(); err != nil {
			return errors.Wrapf(err, "dropping collection '%s'", coll)
		}
	}
	return nil
}

// EnsureIndex takes in a collection and ensures that the index is created if it
// does not already exist.
func EnsureIndex(collection string, index mongo.IndexModel) error {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	_, err := env.DB().Collection(collection).Indexes().CreateOne(ctx, index)

	return errors.WithStack(err)
}

const errorCodeNamespaceNotFound = 26

// DropAllIndexes drops all indexes in the specified collections, returning an
// error immediately if dropping the indexes in any one of them fails.
func DropAllIndexes(collections ...string) error {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	for _, coll := range collections {
		if _, err := env.DB().Collection(coll).Indexes().DropAll(ctx); err != nil {
			// DropAll errors if the collection does not exist, so make this
			// idempotent by ignoring the error for the case of a nonexistent
			// collection.
			if mongoErr, ok := err.(driver.Error); ok && mongoErr.NamespaceNotFound() {
				continue
			}
			if cmdErr, ok := err.(mongo.CommandError); ok && cmdErr.HasErrorCode(errorCodeNamespaceNotFound) {
				continue
			}
			return errors.Wrapf(err, "dropping indexes in collection '%s'", coll)
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

// Update updates one matching document in the collection.
func Update(collection string, query interface{}, update interface{}) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		grip.Errorf("error establishing db connection: %+v", err)

		return err
	}
	defer session.Close()

	return db.C(collection).Update(query, update)
}

// UpdateId updates one _id-matching document in the collection.
func UpdateId(collection string, id, update interface{}) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		grip.Errorf("error establishing db connection: %+v", err)

		return err
	}
	defer session.Close()

	return db.C(collection).UpdateId(id, update)
}

// UpdateAll updates all matching documents in the collection.
func UpdateAll(collection string, query interface{}, update interface{}) (*db.ChangeInfo, error) {
	switch query.(type) {
	case *Q, Q:
		grip.EmergencyPanic(message.Fields{
			"message":    "invalid query passed to update all",
			"cause":      "programmer error",
			"query":      query,
			"collection": collection,
		})
	case nil:
		grip.EmergencyPanic(message.Fields{
			"message":    "nil query passed to update all",
			"query":      query,
			"collection": collection,
		})
	}

	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		grip.Errorf("error establishing db connection: %+v", err)

		return nil, err
	}
	defer session.Close()

	return db.C(collection).UpdateAll(query, update)
}

// Upsert run the specified update against the collection as an upsert operation.
func Upsert(collection string, query interface{}, update interface{}) (*db.ChangeInfo, error) {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		grip.Errorf("error establishing db connection: %+v", err)

		return nil, err
	}
	defer session.Close()

	return db.C(collection).Upsert(query, update)
}

// Count run a count command with the specified query against the collection.
func Count(collection string, query interface{}) (int, error) {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		grip.Errorf("error establishing db connection: %+v", err)

		return 0, err
	}
	defer session.Close()

	return db.C(collection).Find(query).Count()
}

// FindAndModify runs the specified query and change against the collection,
// unmarshaling the result into the specified interface.
func FindAndModify(collection string, query interface{}, sort []string, change db.Change, out interface{}) (*db.ChangeInfo, error) {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		grip.Errorf("error establishing db connection: %+v", err)

		return nil, err
	}
	defer session.Close()
	return db.C(collection).Find(query).Sort(sort...).Apply(change, out)
}

// WriteGridFile writes the data in the source Reader to a GridFS collection with
// the given prefix and filename.
func WriteGridFile(fsPrefix, name string, source io.Reader) error {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	bucket, err := pail.NewGridFSBucketWithClient(ctx, env.Client(), pail.GridFSOptions{
		Database: env.DB().Name(),
		Name:     fsPrefix,
	})

	if err != nil {
		return errors.Wrap(err, "problem constructing bucket access")
	}
	return errors.Wrap(bucket.Put(ctx, name, source), "problem writing file")
}

// GetGridFile returns a ReadCloser for a file stored with the given name under the GridFS prefix.
func GetGridFile(fsPrefix, name string) (io.ReadCloser, error) {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	bucket, err := pail.NewGridFSBucketWithClient(ctx, env.Client(), pail.GridFSOptions{
		Database: env.DB().Name(),
		Name:     fsPrefix,
	})

	if err != nil {
		return nil, errors.Wrap(err, "problem constructing bucket access")
	}

	return bucket.Get(ctx, name)
}

func ClearGridCollections(fsPrefix string) error {
	return ClearCollections(fmt.Sprintf("%s.files", fsPrefix), fmt.Sprintf("%s.chunks", fsPrefix))
}

// Aggregate runs an aggregation pipeline on a collection and unmarshals
// the results to the given "out" interface (usually a pointer
// to an array of structs/bson.M)
func Aggregate(collection string, pipeline interface{}, out interface{}) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		err = errors.Wrap(err, "error establishing db connection")
		grip.Error(err)
		return err
	}
	defer session.Close()

	// NOTE: with the legacy driver, this function unset the
	// socket timeout, which isn't really an option here. (other
	// operations had a 90s timeout, which is no longer specified)

	pipe := db.C(collection).Pipe(pipeline)

	return errors.WithStack(pipe.All(out))
}
