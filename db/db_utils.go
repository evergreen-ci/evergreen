package db

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"go.mongodb.org/mongo-driver/bson/bsontype"

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
	NoProjection = bson.M{}
	NoSort       = []string{}
	NoSkip       = 0
	NoLimit      = 0
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

// ClearCollections clears all documents from all the specified collections, returning an error
// immediately if clearing any one of them fails.
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

// EnsureIndex takes in a collection and ensures that the
func EnsureIndex(collection string, index mongo.IndexModel) error {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	_, err := env.DB().Collection(collection).Indexes().CreateOne(ctx, index)

	return errors.WithStack(err)
}

// DropIndex takes in a collection and a slice of keys and drops those indexes
func DropAllIndexes(collection string) error {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	_, err := env.DB().Collection(collection).Indexes().DropAll(ctx)
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
		grip.Errorf("error establishing db connection: %+v", err)
		return err
	}
	defer session.Close()

	q := db.C(collection).Find(query).Select(projection).Limit(1)
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
		grip.Errorf("error establishing db connection: %+v", err)
		return errors.WithStack(err)
	}
	defer session.Close()

	q := db.C(collection).Find(query)
	if projection != nil {
		q = q.Select(projection)
	}

	if len(sort) != 0 {
		q = q.Sort(sort...)
	}

	return errors.WithStack(q.Skip(skip).Limit(limit).All(out))
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

type Index struct {
	Name                    string   `bson:"name"`
	Key                     IndexKey `bson:"key"`
	Unique                  bool     `bson:"unique"`
	ExpireAfter             int      `bson:"expireAfterSeconds,omitempty"`
	PartialFilterExpression bson.D   `bson:"partialFilterExpression,omitempty"`
}

type IndexKey struct {
	Fields []IndexFieldSpec
}

type IndexFieldSpec struct {
	Name  string
	Value interface{}
}

func (ik IndexKey) MarshalBSON() ([]byte, error) {
	d := make(bson.D, 0, len(ik.Fields))
	for _, field := range ik.Fields {
		d = append(d, bson.E{field.Name, field.Value})
	}
	return bson.Marshal(d)
}

func (ik *IndexKey) UnmarshalBSONValue(t bsontype.Type, data []byte) error {
	var d bson.D
	if err := (bson.RawValue{Type: t, Value: data}).Unmarshal(&d); err != nil {
		return err
	}
	ik.Fields = make([]IndexFieldSpec, 0, len(d))
	for _, elem := range d {
		ik.Fields = append(ik.Fields, IndexFieldSpec{Name: elem.Key, Value: elem.Value})
	}
	return nil
}

func CreateIndexes(ctx context.Context, collName string, indexSpecs ...Index) error {
	newSpecs := make([]Index, 0, len(indexSpecs))
	for _, spec := range indexSpecs {
		if spec.Name == "" {
			var buff bytes.Buffer
			for i, field := range spec.Key.Fields {
				if i > 0 {
					buff.WriteRune('_')
				}
				buff.WriteString(field.Name)
				buff.WriteRune('_')

				buff.WriteString(fmt.Sprintf("%v", field.Value))
			}
			spec.Name = buff.String()
		}
		newSpecs = append(newSpecs, spec)
	}

	createIndexCommand := bson.D{
		{"createIndexes", collName},
		{"indexes", newSpecs},
	}

	db := evergreen.GetEnvironment().DB()
	result := db.RunCommand(ctx, createIndexCommand)
	err := result.Err()
	if err != nil {
		return err
	}
	return nil
}
