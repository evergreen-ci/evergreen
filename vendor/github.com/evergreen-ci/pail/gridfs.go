package pail

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
	Verbose      bool
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
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"dry_run":       b.opts.DryRun,
		"operation":     "writer",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"key":           name,
	})

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
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"operation":     "reader",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"key":           name,
	})

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
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"dry_run":       b.opts.DryRun,
		"operation":     "put",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"key":           name,
	})

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
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"operation":     "get",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"key":           name,
	})

	return b.Reader(ctx, name)
}

func (b *gridfsBucket) Upload(ctx context.Context, name, path string) error {
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"dry_run":       b.opts.DryRun,
		"operation":     "upload",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"key":           name,
		"path":          path,
	})

	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "problem opening file %s", name)
	}
	defer f.Close()

	return errors.WithStack(b.Put(ctx, name, f))
}

func (b *gridfsBucket) Download(ctx context.Context, name, path string) error {
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"operation":     "download",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"key":           name,
		"path":          path,
	})

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

func (b *gridfsBucket) Push(ctx context.Context, opts SyncOptions) error {
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"dry_run":       b.opts.DryRun,
		"operation":     "push",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"remote":        opts.Remote,
		"local":         opts.Local,
		"exclude":       opts.Exclude,
	})

	var re *regexp.Regexp
	var err error
	if opts.Exclude != "" {
		re, err = regexp.Compile(opts.Exclude)
		if err != nil {
			return errors.Wrap(err, "problem compiling exclude regex")
		}
	}

	localPaths, err := walkLocalTree(ctx, opts.Local)
	if err != nil {
		return errors.Wrap(err, "problem finding local paths")
	}

	for _, path := range localPaths {
		if re != nil && re.MatchString(path) {
			continue
		}

		target := consistentJoin(opts.Remote, path)
		_ = b.Remove(ctx, target)
		if err = b.Upload(ctx, target, filepath.Join(opts.Local, path)); err != nil {
			return errors.Wrapf(err, "problem uploading '%s' to '%s'", path, target)
		}
	}

	if b.opts.DeleteOnSync && !b.opts.DryRun {
		return errors.Wrap(deleteOnPush(ctx, localPaths, opts.Remote, b), "problem with delete on sync after push")
	}

	return nil
}

func (b *gridfsBucket) Pull(ctx context.Context, opts SyncOptions) error {
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"operation":     "pull",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"remote":        opts.Remote,
		"local":         opts.Local,
		"exclude":       opts.Exclude,
	})

	var re *regexp.Regexp
	var err error
	if opts.Exclude != "" {
		re, err = regexp.Compile(opts.Exclude)
		if err != nil {
			return errors.Wrap(err, "problem compiling exclude regex")
		}
	}

	iter, err := b.List(ctx, opts.Remote)
	if err != nil {
		return errors.WithStack(err)
	}

	keys := []string{}

	for iter.Next(ctx) {
		item := iter.Item()
		if re != nil && re.MatchString(item.Name()) {
			continue
		}

		fn := item.Name()[len(opts.Remote)+1:]
		name := filepath.Join(opts.Local, fn)
		keys = append(keys, fn)

		if err = b.Download(ctx, item.Name(), name); err != nil {
			return errors.WithStack(err)
		}
	}

	if err = iter.Err(); err != nil {
		return errors.WithStack(err)
	}

	if b.opts.DeleteOnSync && !b.opts.DryRun {
		return errors.Wrap(deleteOnPull(ctx, keys, opts.Local), "problem with delete on sync after pull")
	}

	return nil
}

func (b *gridfsBucket) Copy(ctx context.Context, opts CopyOptions) error {
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"operation":     "copy",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"source_key":    opts.SourceKey,
		"dest_key":      opts.DestinationKey,
	})

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
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"dry_run":       b.opts.DryRun,
		"operation":     "remove",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"key":           key,
	})

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
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"dry_run":       b.opts.DryRun,
		"operation":     "remove many",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"keys":          keys,
	})

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
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"dry_run":       b.opts.DryRun,
		"operation":     "remove prefix",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"prefix":        prefix,
	})

	return removePrefix(ctx, prefix, b)
}

func (b *gridfsBucket) RemoveMatching(ctx context.Context, expr string) error {
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"dry_run":       b.opts.DryRun,
		"operation":     "remove matching",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"expression":    expr,
	})

	return removeMatching(ctx, expr, b)
}

func (b *gridfsBucket) List(ctx context.Context, prefix string) (BucketIterator, error) {
	grip.DebugWhen(b.opts.Verbose, message.Fields{
		"type":          "gridfs",
		"operation":     "list",
		"bucket":        b.opts.Name,
		"bucket_prefix": b.opts.Prefix,
		"prefix":        prefix,
	})

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
