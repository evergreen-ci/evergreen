package pail

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GridFSOptions support the use and creation of GridFS backed
// buckets.
type GridFSOptions struct {
	Name         string
	Prefix       string
	Database     string
	MongoDBURI   string
	DryRun       bool
	DeleteOnSync bool
}

type gridfsBucket struct {
	opts   GridFSOptions
	client *mongo.Client
}

func (b *gridfsBucket) normalizeKey(key string) string {
	if key == "" {
		return b.opts.Prefix
	}
	return consistentJoin(b.opts.Prefix, key)
}

func (b *gridfsBucket) denormalizeKey(key string) string {
	if b.opts.Prefix != "" && len(key) > len(b.opts.Prefix)+1 {
		key = key[len(b.opts.Prefix)+1:]
	}
	return key
}

// NewGridFSBucketWithClient constructs a Bucket implementation using
// GridFS and the new MongoDB driver. If client is nil, then this
// method falls back to the behavior of NewGridFS bucket. Use the
// Check method to verify that this bucket ise operationsal.
func NewGridFSBucketWithClient(ctx context.Context, client *mongo.Client, opts GridFSOptions) (Bucket, error) {
	if client == nil {
		return NewGridFSBucket(ctx, opts)
	}

	return &gridfsBucket{opts: opts, client: client}, nil
}

// NewGridFSBucket creates a Bucket instance backed by the new MongoDb
// driver, creating a new client and connecting to the URI.
// Use the Check method to verify that this bucket ise operationsal.
func NewGridFSBucket(ctx context.Context, opts GridFSOptions) (Bucket, error) {
	client, err := mongo.NewClient(options.Client().ApplyURI(opts.MongoDBURI))
	if err != nil {
		return nil, errors.Wrap(err, "problem constructing client")
	}

	connctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	err = client.Connect(connctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem connecting")
	}

	return &gridfsBucket{opts: opts, client: client}, nil
}

func (b *gridfsBucket) Check(ctx context.Context) error {
	if b.client == nil {
		return errors.New("no client defined")
	}

	return errors.Wrap(b.client.Ping(ctx, nil), "problem contacting mongodb")
}

func (b *gridfsBucket) bucket(ctx context.Context) (*gridfs.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "cannot fetch bucket with canceled context")
	}

	gfs, err := gridfs.NewBucket(b.client.Database(b.opts.Database), options.GridFSBucket().SetName(b.opts.Name))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dl, ok := ctx.Deadline()
	if ok {
		_ = gfs.SetReadDeadline(dl)
		_ = gfs.SetWriteDeadline(dl)
	}

	return gfs, nil
}

func (b *gridfsBucket) Writer(ctx context.Context, name string) (io.WriteCloser, error) {
	grid, err := b.bucket(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem resolving bucket")
	}

	if b.opts.DryRun {
		return &mockWriteCloser{}, nil
	}

	writer, err := grid.OpenUploadStream(b.normalizeKey(name))
	if err != nil {
		return nil, errors.Wrap(err, "problem opening stream")
	}

	return writer, nil
}

func (b *gridfsBucket) Reader(ctx context.Context, name string) (io.ReadCloser, error) {
	grid, err := b.bucket(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem resolving bucket")
	}

	reader, err := grid.OpenDownloadStreamByName(b.normalizeKey(name))
	if err != nil {
		return nil, errors.Wrap(err, "problem opening stream")
	}

	return reader, nil
}

func (b *gridfsBucket) Put(ctx context.Context, name string, input io.Reader) error {
	grid, err := b.bucket(ctx)
	if err != nil {
		return errors.Wrap(err, "problem resolving bucket")
	}

	if b.opts.DryRun {
		return nil
	}

	if _, err = grid.UploadFromStream(b.normalizeKey(name), input); err != nil {
		return errors.Wrap(err, "problem uploading file")
	}

	return nil
}

func (b *gridfsBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	return b.Reader(ctx, name)
}

func (b *gridfsBucket) Upload(ctx context.Context, name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "problem opening file %s", name)
	}
	defer f.Close()

	return errors.WithStack(b.Put(ctx, name, f))
}

func (b *gridfsBucket) Download(ctx context.Context, name, path string) error {
	reader, err := b.Reader(ctx, name)
	if err != nil {
		return errors.WithStack(err)
	}

	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return errors.Wrapf(err, "problem creating enclosing directory for '%s'", path)
	}

	f, err := os.Create(path)
	if err != nil {
		return errors.Wrapf(err, "problem creating file '%s'", path)
	}
	_, err = io.Copy(f, reader)
	if err != nil {
		_ = f.Close()
		return errors.Wrap(err, "problem copying data")
	}

	return errors.WithStack(f.Close())
}

func (b *gridfsBucket) Push(ctx context.Context, local, remote string) error {
	localPaths, err := walkLocalTree(ctx, local)
	if err != nil {
		return errors.Wrap(err, "problem finding local paths")
	}

	for _, path := range localPaths {
		target := consistentJoin(remote, path)
		_ = b.Remove(ctx, target)
		if err = b.Upload(ctx, target, filepath.Join(local, path)); err != nil {
			return errors.Wrapf(err, "problem uploading '%s' to '%s'", path, target)
		}
	}

	if b.opts.DeleteOnSync && !b.opts.DryRun {
		return errors.Wrapf(os.RemoveAll(local), "problem removing '%s' after push", local)
	}

	return nil
}

func (b *gridfsBucket) Pull(ctx context.Context, local, remote string) error {
	iter, err := b.List(ctx, remote)
	if err != nil {
		return errors.WithStack(err)
	}

	keys := []string{}

	for iter.Next(ctx) {
		item := iter.Item()
		name := filepath.Join(local, item.Name()[len(remote)+1:])
		keys = append(keys, item.Name())

		if err = b.Download(ctx, item.Name(), name); err != nil {
			return errors.WithStack(err)
		}
	}

	if err = iter.Err(); err != nil {
		return errors.WithStack(err)
	}

	if b.opts.DeleteOnSync && !b.opts.DryRun {
		return errors.Wrapf(b.RemoveMany(ctx, keys...), "problem removing '%s' after pull", remote)
	}

	return nil
}

func (b *gridfsBucket) Copy(ctx context.Context, opts CopyOptions) error {
	from, err := b.Reader(ctx, opts.SourceKey)
	if err != nil {
		return errors.Wrap(err, "problem getting reader for source")
	}

	to, err := opts.DestinationBucket.Writer(ctx, opts.DestinationKey)
	if err != nil {
		return errors.Wrap(err, "problem getting writer for destination")
	}

	if _, err = io.Copy(to, from); err != nil {
		return errors.Wrap(err, "problem copying data")
	}

	return errors.WithStack(to.Close())
}

func (b *gridfsBucket) Remove(ctx context.Context, key string) error {
	grid, err := b.bucket(ctx)
	if err != nil {
		return errors.Wrap(err, "problem resolving bucket")
	}

	cursor, err := grid.Find(bson.M{"filename": b.normalizeKey(key)})
	if err == mongo.ErrNoDocuments {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "problem finding file")
	}

	document := struct {
		ID interface{} `bson:"_id"`
	}{}

	for cursor.Next(ctx) {
		err = cursor.Decode(&document)
		if err == mongo.ErrNoDocuments {
			continue
		}

		if err != nil {
			_ = cursor.Close(ctx)
			return errors.Wrap(err, "problem decoding gridfs metadata")
		}

		if b.opts.DryRun {
			continue
		}

		if err = grid.Delete(document.ID); err != nil {
			return errors.Wrap(err, "problem deleting gridfs file")
		}
	}
	if err = cursor.Err(); err != nil {
		return errors.Wrap(err, "problem iterating gridfs metadata")
	}
	if err = cursor.Close(ctx); err != nil {
		return errors.Wrap(err, "problem closing ")
	}

	return nil
}

func (b *gridfsBucket) RemoveMany(ctx context.Context, keys ...string) error {
	grid, err := b.bucket(ctx)
	if err != nil {
		return errors.Wrap(err, "problem resolving bucket")
	}

	normalizedKeys := make([]string, len(keys))
	for i, key := range keys {
		normalizedKeys[i] = b.normalizeKey(key)
	}

	cursor, err := grid.Find(bson.M{"filename": bson.M{"$in": normalizedKeys}})
	if err != nil {
		return errors.Wrap(err, "problem finding file")
	}

	document := struct {
		ID interface{} `bson:"_id"`
	}{}

	for cursor.Next(ctx) {
		err = cursor.Decode(&document)
		if err == mongo.ErrNoDocuments {
			continue
		}

		if err != nil {
			return errors.Wrap(err, "problem decoding gridfs metadata")
		}

		if b.opts.DryRun {
			continue
		}

		if err = grid.Delete(document.ID); err != nil {
			return errors.Wrap(err, "problem deleting gridfs file")
		}
	}

	if err = cursor.Err(); err != nil {
		return errors.Wrap(err, "problem iterating gridfs metadata")
	}

	if err = cursor.Close(ctx); err != nil {
		return errors.Wrap(err, "problem closing cursor")
	}

	return nil
}

func (b *gridfsBucket) RemovePrefix(ctx context.Context, prefix string) error {
	return removePrefix(ctx, prefix, b)
}

func (b *gridfsBucket) RemoveMatching(ctx context.Context, expr string) error {
	return removeMatching(ctx, expr, b)
}

func (b *gridfsBucket) List(ctx context.Context, prefix string) (BucketIterator, error) {
	filter := bson.M{}
	if prefix != "" {
		filter = bson.M{"filename": primitive.Regex{Pattern: fmt.Sprintf("^%s.*", b.normalizeKey(prefix))}}
	}

	grid, err := b.bucket(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem resolving bucket")
	}

	cursor, err := grid.Find(filter)
	if err != nil {
		return nil, errors.Wrap(err, "problem finding file")
	}

	return &gridfsIterator{bucket: b, iter: cursor}, nil
}

type gridfsIterator struct {
	err    error
	bucket *gridfsBucket
	iter   *mongo.Cursor
	item   *bucketItemImpl
}

func (iter *gridfsIterator) Err() error       { return iter.err }
func (iter *gridfsIterator) Item() BucketItem { return iter.item }
func (iter *gridfsIterator) Next(ctx context.Context) bool {
	if !iter.iter.Next(ctx) {
		iter.err = iter.iter.Err()
		return false
	}

	document := struct {
		ID       interface{} `bson:"_id"`
		Filename string      `bson:"filename"`
	}{}

	err := iter.iter.Decode(&document)
	if err != nil {
		iter.err = err
		return false
	}

	iter.item = &bucketItemImpl{
		bucket: iter.bucket.opts.Prefix,
		b:      iter.bucket,
		key:    iter.bucket.denormalizeKey(document.Filename),
	}
	return true
}
