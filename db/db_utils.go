package db

import (
	"context"
	"fmt"
	"io"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	NoProjection     = bson.M{}
	NoSort           = []string{}
	NoSkip           = 0
	NoLimit          = 0
	NoHint       any = nil
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

// GetGlobalSessionFactory initializes a session factory to connect to the Evergreen database.
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
func Insert(ctx context.Context, collection string, item any) error {
	_, err := evergreen.GetEnvironment().DB().Collection(collection).InsertOne(ctx,
		item,
	)
	return errors.Wrapf(errors.WithStack(err), "inserting document")
}

func InsertMany(ctx context.Context, collection string, items ...any) error {
	if len(items) == 0 {
		return nil
	}

	_, err := evergreen.GetEnvironment().DB().Collection(collection).InsertMany(ctx,
		items,
	)
	return errors.Wrapf(errors.WithStack(err), "inserting documents")
}

func InsertManyUnordered(ctx context.Context, collection string, items ...any) error {
	if len(items) == 0 {
		return nil
	}

	_, err := evergreen.GetEnvironment().DB().Collection(collection).InsertMany(ctx,
		items,
		options.InsertMany().SetOrdered(false),
	)
	return errors.Wrapf(errors.WithStack(err), "inserting unordered documents")
}

// Remove removes one item matching the query from the specified collection.
func Remove(ctx context.Context, collection string, query any) error {
	_, err := evergreen.GetEnvironment().DB().Collection(collection).DeleteOne(ctx,
		query,
	)
	return errors.Wrapf(errors.WithStack(err), "deleting document")
}

// RemoveAll removes all items matching the query from the specified collection.
func RemoveAll(ctx context.Context, collection string, query any) error {
	_, err := evergreen.GetEnvironment().DB().Collection(collection).DeleteMany(ctx,
		query,
	)
	return errors.Wrapf(errors.WithStack(err), "deleting documents")
}

// UpdateContext updates one matching document in the collection.
func UpdateContext(ctx context.Context, collection string, query any, update any) error {
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

// ReplaceContext replaces one matching document in the collection. If a matching
// document is not found, it will be upserted. It returns the upserted ID if
// one was created.
func ReplaceContext(ctx context.Context, collection string, query any, replacement any) (*db.ChangeInfo, error) {
	res, err := evergreen.GetEnvironment().DB().Collection(collection).ReplaceOne(ctx,
		query,
		replacement,
		options.Replace().SetUpsert(true),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "replacing document")
	}

	return &db.ChangeInfo{Updated: int(res.UpsertedCount) + int(res.ModifiedCount), UpsertedId: res.UpsertedID}, nil
}

// UpdateAllContext updates all matching documents in the collection.
func UpdateAllContext(ctx context.Context, collection string, query any, update any) (*db.ChangeInfo, error) {
	res, err := evergreen.GetEnvironment().DB().Collection(collection).UpdateMany(ctx,
		query,
		update,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "updating task")
	}

	return &db.ChangeInfo{Updated: int(res.ModifiedCount)}, nil
}

// UpdateIdContext updates one _id-matching document in the collection.
func UpdateIdContext(ctx context.Context, collection string, id, update any) error {
	res, err := evergreen.GetEnvironment().DB().Collection(collection).UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}},
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

// Upsert run the specified update against the collection as an upsert operation.
func Upsert(ctx context.Context, collection string, query any, update any) (*db.ChangeInfo, error) {
	res, err := evergreen.GetEnvironment().DB().Collection(collection).UpdateOne(
		ctx,
		query,
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "upserting")
	}

	return &db.ChangeInfo{Updated: int(res.UpsertedCount) + int(res.ModifiedCount), UpsertedId: res.UpsertedID}, nil
}

// Count run a count command with the specified query against the collection.
func Count(ctx context.Context, collection string, query any) (int, error) {
	res, err := evergreen.GetEnvironment().DB().Collection(collection).CountDocuments(
		ctx,
		query,
	)
	return int(res), errors.WithStack(err)
}

// FindOneQ runs a Q query against the given collection, applying the results to "out."
// Only reads one document from the DB.
// DEPRECATED (DEVPROD-15398): This is only here to support a cache
// with Gimlet, use FindOneQContext instead.
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
func FindAllQ(ctx context.Context, collection string, q Q, out any) error {
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
func CountQ(ctx context.Context, collection string, q Q) (int, error) {
	return Count(ctx, collection, q.filter)
}

// RemoveAllQ removes all docs that satisfy the query
func RemoveAllQ(ctx context.Context, collection string, q Q) error {
	return Remove(ctx, collection, q.filter)
}

// FindAndModify runs the specified query and change against the collection,
// unmarshaling the result into the specified interface.
func FindAndModify(ctx context.Context, collection string, query any, sort []string, change db.Change, out any) (*db.ChangeInfo, error) {
	session, db, err := GetGlobalSessionFactory().GetContextSession(ctx)
	if err != nil {
		grip.Errorf("error establishing db connection: %+v", err)

		return nil, err
	}
	defer session.Close()
	return db.C(collection).Find(query).Sort(sort...).Apply(change, out)
}

// WriteGridFile writes the data in the source Reader to a GridFS collection with
// the given prefix and filename.
func WriteGridFile(ctx context.Context, fsPrefix, name string, source io.Reader) error {
	env := evergreen.GetEnvironment()
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
func GetGridFile(ctx context.Context, fsPrefix, name string) (io.ReadCloser, error) {
	env := evergreen.GetEnvironment()
	bucket, err := pail.NewGridFSBucketWithClient(ctx, env.Client(), pail.GridFSOptions{
		Database: env.DB().Name(),
		Name:     fsPrefix,
	})

	if err != nil {
		return nil, errors.Wrap(err, "problem constructing bucket access")
	}

	return bucket.Get(ctx, name)
}

// Aggregate runs an aggregation pipeline on a collection and unmarshals
// the results to the given "out" interface (usually a pointer
// to an array of structs/bson.M)
func Aggregate(ctx context.Context, collection string, pipeline any, out any) error {
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

// =============================================
// ============ Test only functions ============
// =============================================

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

func ClearGridCollections(fsPrefix string) error {
	return ClearCollections(fmt.Sprintf("%s.files", fsPrefix), fmt.Sprintf("%s.chunks", fsPrefix))
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
