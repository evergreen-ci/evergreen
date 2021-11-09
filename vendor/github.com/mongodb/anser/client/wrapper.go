package client

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type clientWrapper struct {
	cl *mongo.Client
}

func WrapClient(c *mongo.Client) Client                       { return &clientWrapper{cl: c} }
func (c *clientWrapper) Connect(ctx context.Context) error    { return c.cl.Connect(ctx) }
func (c *clientWrapper) Disconnect(ctx context.Context) error { return c.cl.Disconnect(ctx) }
func (c *clientWrapper) Database(name string) Database {
	return &databaseWrapper{db: c.cl.Database(name)}
}
func (c *clientWrapper) ListDatabaseNames(ctx context.Context, filter interface{}) ([]string, error) {
	return c.cl.ListDatabaseNames(ctx, filter)
}

type databaseWrapper struct {
	db *mongo.Database
}

func (d *databaseWrapper) Client() Client { return WrapClient(d.db.Client()) }
func (d *databaseWrapper) Name() string   { return d.db.Name() }
func (d *databaseWrapper) RunCommand(ctx context.Context, cmd interface{}) SingleResult {
	return &singleResultWrapper{d.db.RunCommand(ctx, cmd)}
}
func (d *databaseWrapper) RunCommandCursor(ctx context.Context, cmd interface{}) (Cursor, error) {
	cur, err := d.db.RunCommandCursor(ctx, cmd)
	return &cursorWrapper{cur}, errors.WithStack(err)
}

func (d *databaseWrapper) Collection(coll string) Collection {
	return &collectionWrapper{Collection: d.db.Collection(coll)}
}

type collectionWrapper struct {
	*mongo.Collection
}

func (c *collectionWrapper) Aggregate(ctx context.Context, pipe interface{}, opts ...*options.AggregateOptions) (Cursor, error) {
	cur, err := c.Collection.Aggregate(ctx, pipe, opts...)
	return &cursorWrapper{cur}, errors.WithStack(err)
}

func (c *collectionWrapper) Find(ctx context.Context, query interface{}, opts ...*options.FindOptions) (Cursor, error) {
	cur, err := c.Collection.Find(ctx, query, opts...)
	return &cursorWrapper{cur}, errors.WithStack(err)
}

func (c *collectionWrapper) FindOne(ctx context.Context, query interface{}, opts ...*options.FindOneOptions) SingleResult {
	return &singleResultWrapper{c.Collection.FindOne(ctx, query, opts...)}
}

func (c *collectionWrapper) InsertMany(ctx context.Context, docs []interface{}) (*InsertManyResult, error) {
	return c.Collection.InsertMany(ctx, docs)
}
func (c *collectionWrapper) InsertOne(ctx context.Context, doc interface{}) (*InsertOneResult, error) {
	return c.Collection.InsertOne(ctx, doc)
}
func (c *collectionWrapper) ReplaceOne(ctx context.Context, query, doc interface{}, opts ...*options.ReplaceOptions) (*UpdateResult, error) {
	return c.Collection.ReplaceOne(ctx, query, doc, opts...)
}

func (c *collectionWrapper) UpdateMany(ctx context.Context, query, update interface{}, opts ...*options.UpdateOptions) (*UpdateResult, error) {
	return c.Collection.UpdateMany(ctx, query, update, opts...)
}

func (c *collectionWrapper) UpdateOne(ctx context.Context, query, update interface{}, opts ...*options.UpdateOptions) (*UpdateResult, error) {
	return c.Collection.UpdateOne(ctx, query, update, opts...)
}

type singleResultWrapper struct {
	*mongo.SingleResult
}

func (sr *singleResultWrapper) DecodeBytes() ([]byte, error) { return sr.SingleResult.DecodeBytes() }

type cursorWrapper struct {
	*mongo.Cursor
}

func (cr cursorWrapper) Current() []byte { return cr.Cursor.Current }
