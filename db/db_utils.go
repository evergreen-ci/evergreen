package db

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	NoProjection             = bson.M{}
	NoSort                   = []string{}
	NoSkip                   = 0
	NoLimit                  = 0
	NoHint       interface{} = nil
)

type SessionFactory interface {
	// GetSession uses the global environment's context to get a session and database.
	GetSession() (db.Session, db.Database, error)
	// GetContextSession uses the provided context to get a session and database.
	// This is needed for operations that need control over the passed-in context.
	// TODO DEVPROD-11824 Use this method instead of GetSession.
	GetContextSession(ctx context.Context) (db.Session, db.Database, error)
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

// GetSession creates a database connection using the global environment's
// session (and context through the session).
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

// GetContextSession creates a database session and connection that uses the associated
// context in its operations.
func (s *shimFactoryImpl) GetContextSession(ctx context.Context) (db.Session, db.Database, error) {
	if s.env == nil {
		return nil, nil, errors.New("undefined environment")
	}

	session := s.env.ContextSession(ctx)
	if session == nil {
		return nil, nil, errors.New("context session is not defined")
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

// CreateCollections ensures that all the given collections are created,
// returning an error immediately if creating any one of them fails.
func CreateCollections(collections ...string) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()

	const namespaceExistsErrCode = 48
	for _, collection := range collections {
		_, err := db.CreateCollection(collection)
		if err == nil {
			continue
		}
		// If the collection already exists, this does not count as an error.
		if mongoErr, ok := errors.Cause(err).(mongo.CommandError); ok && mongoErr.HasErrorCode(namespaceExistsErrCode) {
			continue
		}
		if err != nil {
			return errors.Wrapf(err, "creating collection '%s'", collection)
		}
	}
	return nil
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

// Remove removes one item matching the query from the specified collection.
func Remove(collection string, query interface{}) error {
	session, db, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()

	return db.C(collection).Remove(query)
}

func RemoveContext(ctx context.Context, collection string, query interface{}) error {
	session, db, err := GetGlobalSessionFactory().GetContextSession(ctx)
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

// Update updates one matching document in the collection.
func UpdateContext(ctx context.Context, collection string, query interface{}, update interface{}) error {
	res, err := evergreen.GetEnvironment().DB().Collection(collection).UpdateOne(ctx,
		query,
		update,
	)
	if err != nil {
		return errors.Wrapf(err, "updating task")
	}
	if res.MatchedCount == 0 {
		return db.ErrNotFound
	}

	return nil
}

func UpdateAllContext(ctx context.Context, collection string, query interface{}, update interface{}) (*db.ChangeInfo, error) {
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

	res, err := evergreen.GetEnvironment().DB().Collection(collection).UpdateMany(ctx,
		query,
		update,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "updating task")
	}

	return &db.ChangeInfo{Updated: int(res.ModifiedCount)}, nil
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

// Count run a count command with the specified query against the collection.
func CountContext(ctx context.Context, collection string, query interface{}) (int, error) {
	session, db, err := GetGlobalSessionFactory().GetContextSession(ctx)
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
		err = errors.Wrap(err, "establishing db connection")
		grip.Error(err)
		return err
	}
	defer session.Close()

	pipe := db.C(collection).Pipe(pipeline)

	return errors.WithStack(pipe.All(out))
}

// AggregateContext runs an aggregation pipeline on a collection and unmarshals
// the results to the given "out" interface (usually a pointer
// to an array of structs/bson.M)
func AggregateContext(ctx context.Context, collection string, pipeline interface{}, out interface{}) error {
	session, db, err := GetGlobalSessionFactory().GetContextSession(ctx)
	if err != nil {
		err = errors.Wrap(err, "establishing db connection")
		grip.Error(err)
		return err
	}
	defer session.Close()

	pipe := db.C(collection).Pipe(pipeline)

	return errors.WithStack(pipe.All(out))
}

// AggregateWithMaxTime runs aggregate and specifies a max query time which
// ensures the query won't go on indefinitely when the request is cancelled.
func AggregateWithMaxTime(collection string, pipeline interface{}, out interface{}, maxTime time.Duration) error {
	session, database, err := GetGlobalSessionFactory().GetSession()
	if err != nil {
		err = errors.Wrap(err, "establishing DB connection")
		grip.Error(err)
		return err
	}
	defer session.Close()

	return database.C(collection).Pipe(pipeline).MaxTime(maxTime).All(out)
}
