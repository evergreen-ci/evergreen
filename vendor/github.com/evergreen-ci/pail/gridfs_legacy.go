package pail

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type gridfsLegacyBucket struct {
	opts    GridFSOptions
	session *mgo.Session
}

// GridFSOptions support the use and creation of GridFS backed
// buckets.
type GridFSOptions struct {
	Prefix     string
	Database   string
	MongoDBURI string
	DryRun     bool
}

// NewLegacyGridFSBucket creates a Bucket implementation backed by
// GridFS as implemented by the legacy "mgo" MongoDB driver. This
// constructor creates a new connection and mgo session.
//
// Mgo in general does not offer rich support for contexts, so
// cancellation may not be robust.
func NewLegacyGridFSBucket(opts GridFSOptions) (Bucket, error) {
	if opts.MongoDBURI == "" {
		return nil, errors.New("cannot create a new bucket without a URI")
	}

	ses, err := mgo.DialWithTimeout(opts.MongoDBURI, time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "problem connecting to MongoDB")
	}

	return &gridfsLegacyBucket{
		opts:    opts,
		session: ses,
	}, nil
}

// NewLegacyGridFSBucketWithSession creates a Bucket implementation
// baked by GridFS as implemented by the legacy "mgo" MongoDB driver,
// but allows you to reuse an existing session.
//
// Mgo in general does not offer rich support for contexts, so
// cancellation may not be robust.
func NewLegacyGridFSBucketWithSession(s *mgo.Session, opts GridFSOptions) (Bucket, error) {
	if s == nil {
		b, err := NewLegacyGridFSBucket(opts)
		return b, errors.WithStack(err)
	}

	return &gridfsLegacyBucket{
		opts:    opts,
		session: s,
	}, nil
}

func (b *gridfsLegacyBucket) Check(_ context.Context) error {
	if b.session == nil {
		return errors.New("no session defined")
	}

	return errors.Wrap(b.session.Ping(), "problem contacting mongodb")
}

func (b *gridfsLegacyBucket) gridFS() *mgo.GridFS {
	return b.session.DB(b.opts.Database).GridFS(b.opts.Prefix)
}

func (b *gridfsLegacyBucket) openFile(ctx context.Context, name string, create bool) (io.ReadWriteCloser, error) {
	ses := b.session.Clone()
	out := &legacyGridFSFile{}
	ctx, out.cancel = context.WithCancel(ctx)

	gridfs := b.gridFS()

	var (
		err  error
		file *mgo.GridFile
	)

	if create {
		file, err = gridfs.Create(name)
	} else {
		file, err = gridfs.Open(name)
	}
	if err != nil {
		ses.Close()
		return nil, errors.Wrapf(err, "couldn't open %s/%s", b.opts.Prefix, name)
	}

	out.GridFile = file
	go func() {
		<-ctx.Done()
		ses.Close()
	}()

	return out, nil
}

type legacyGridFSFile struct {
	*mgo.GridFile
	cancel context.CancelFunc
}

func (f *legacyGridFSFile) Close() error { f.cancel(); return errors.WithStack(f.GridFile.Close()) }

func (b *gridfsLegacyBucket) Writer(ctx context.Context, name string) (io.WriteCloser, error) {
	if b.opts.DryRun {
		return &mockWriteCloser{}, nil
	}
	return b.openFile(ctx, name, true)
}

func (b *gridfsLegacyBucket) Reader(ctx context.Context, name string) (io.ReadCloser, error) {
	return b.openFile(ctx, name, false)
}

func (b *gridfsLegacyBucket) Put(ctx context.Context, name string, input io.Reader) error {
	var file io.WriteCloser
	var err error
	if b.opts.DryRun {
		file = &mockWriteCloser{}
	} else {
		file, err = b.openFile(ctx, name, true)
		if err != nil {
			return errors.Wrap(err, "problem creating file")
		}
	}

	_, err = io.Copy(file, input)
	if err != nil {
		return errors.Wrap(err, "problem copying data")
	}

	return errors.Wrap(file.Close(), "problem flushing data to file")
}

func (b *gridfsLegacyBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	return b.Reader(ctx, name)
}

func (b *gridfsLegacyBucket) Upload(ctx context.Context, name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "problem opening file %s", name)
	}
	defer f.Close()

	return errors.WithStack(b.Put(ctx, name, f))
}

func (b *gridfsLegacyBucket) Download(ctx context.Context, name, path string) error {
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

func (b *gridfsLegacyBucket) Push(ctx context.Context, local, remote string) error {
	localPaths, err := walkLocalTree(ctx, local)
	if err != nil {
		return errors.Wrap(err, "problem finding local paths")
	}

	gridfs := b.gridFS()
	for _, path := range localPaths {
		target := filepath.Join(remote, path)
		file, err := gridfs.Open(target)
		if err == mgo.ErrNotFound {
			if err = b.Upload(ctx, target, filepath.Join(local, path)); err != nil {
				return errors.Wrapf(err, "problem uploading '%s' to '%s'", path, target)
			}
			continue
		} else if err != nil {
			return errors.Wrapf(err, "problem finding '%s'", target)
		}

		localmd5, err := md5sum(filepath.Join(local, path))
		if err != nil {
			return errors.Wrapf(err, "problem checksumming '%s'", path)
		}

		if file.MD5() != localmd5 {
			if err = b.Upload(ctx, target, filepath.Join(local, path)); err != nil {
				return errors.Wrapf(err, "problem uploading '%s' to '%s'", path, target)
			}
		}
	}

	return nil
}

func (b *gridfsLegacyBucket) Pull(ctx context.Context, local, remote string) error {
	iter, err := b.List(ctx, remote)
	if err != nil {
		return errors.WithStack(err)
	}

	iterimpl, ok := iter.(*legacyGridFSIterator)
	if !ok {
		return errors.New("programmer error")
	}

	gridfs := b.gridFS()
	var f *mgo.GridFile
	var checksum string
	for gridfs.OpenNext(iterimpl.iter, &f) {
		name := filepath.Join(local, f.Name()[len(remote)+1:])
		checksum, err = md5sum(name)
		if os.IsNotExist(errors.Cause(err)) {
			if err = b.Download(ctx, f.Name(), name); err != nil {
				return errors.WithStack(err)
			}
			continue
		} else if err != nil {
			return errors.WithStack(err)
		}

		// NOTE: it doesn't seem like the md5 sums are being
		// populated, so this always happens
		if f.MD5() != checksum {
			if err = b.Download(ctx, f.Name(), name); err != nil {
				return errors.WithStack(err)
			}
		}
	}

	if err = iterimpl.iter.Err(); err != nil {
		return errors.Wrap(err, "problem iterating bucket")
	}

	return nil
}

func (b *gridfsLegacyBucket) Copy(ctx context.Context, options CopyOptions) error {
	from, err := b.Reader(ctx, options.SourceKey)
	if err != nil {
		return errors.Wrap(err, "problem getting reader for source")
	}

	to, err := options.DestinationBucket.Writer(ctx, options.DestinationKey)
	if err != nil {
		return errors.Wrap(err, "problem getting writer for dst")
	}

	_, err = io.Copy(to, from)
	if err != nil {
		return errors.Wrap(err, "problem copying data")
	}

	return errors.WithStack(to.Close())
}

func (b *gridfsLegacyBucket) Remove(ctx context.Context, key string) error {
	if b.opts.DryRun {
		return nil
	}
	return errors.Wrapf(b.gridFS().Remove(key), "problem removing file %s", key)
}

func (b *gridfsLegacyBucket) RemoveMany(ctx context.Context, keys ...string) error {
	catcher := grip.NewBasicCatcher()
	for _, key := range keys {
		catcher.Add(b.Remove(ctx, key))
	}
	return catcher.Resolve()
}

func (b *gridfsLegacyBucket) RemovePrefix(ctx context.Context, prefix string) error {
	return removePrefix(ctx, prefix, b)
}

func (b *gridfsLegacyBucket) RemoveMatching(ctx context.Context, expression string) error {
	return removeMatching(ctx, expression, b)
}

func (b *gridfsLegacyBucket) List(ctx context.Context, prefix string) (BucketIterator, error) {
	if ctx.Err() != nil {
		return nil, errors.New("operation canceled")
	}

	if prefix == "" {
		return &legacyGridFSIterator{
			ctx:    ctx,
			iter:   b.gridFS().Find(nil).Iter(),
			bucket: b,
		}, nil
	}

	return &legacyGridFSIterator{
		ctx:    ctx,
		iter:   b.gridFS().Find(bson.M{"filename": bson.RegEx{Pattern: fmt.Sprintf("^%s.*", prefix)}}).Iter(),
		bucket: b,
	}, nil
}

type legacyGridFSIterator struct {
	ctx    context.Context
	err    error
	item   *bucketItemImpl
	bucket *gridfsLegacyBucket
	iter   *mgo.Iter
}

func (iter *legacyGridFSIterator) Err() error       { return iter.err }
func (iter *legacyGridFSIterator) Item() BucketItem { return iter.item }

func (iter *legacyGridFSIterator) Next(ctx context.Context) bool {
	if iter.ctx.Err() != nil {
		return false
	}
	if ctx.Err() != nil {
		return false
	}

	var f *mgo.GridFile

	gridfs := iter.bucket.gridFS()

	if !gridfs.OpenNext(iter.iter, &f) {
		return false
	}

	iter.item = &bucketItemImpl{
		bucket: iter.bucket.opts.Prefix,
		key:    f.Name(),
		b:      iter.bucket,
	}

	return true
}
