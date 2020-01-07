package backup

import (
	"context"
	"io"
	"path/filepath"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// WriterCreator provides a way to create writers (e.g. for file or
// similar,) to support writing backup payloads without requiring
// this implementation to manage files or file interfaces.
type WriterCreator func(context.Context, string) (io.WriteCloser, error)

// Options describes the configuration of the backup, for a single
// collection. Query, Sort, and Limit are optional, but allow you to
// constrain the backup.
type Options struct {
	NS          model.Namespace `bson:"ns" json:"ns" yaml:"ns"`
	Target      WriterCreator   `bson:"-" json:"-" yaml:"-"`
	Query       interface{}     `bson:"query" json:"query" yaml:"query"`
	Sort        interface{}     `bson:"sort" json:"sort" yaml:"sort"`
	Limit       int64           `bson:"limit" json:"limit" yaml:"limit"`
	IndexesOnly bool            `bson:"indexes_only" json:"indexes_only" yaml:"indexes_only"`
}

// Collection creates a backup of a collection using the options to
// describe how to filter or constrain the backup. The option's Target
// value allows you to produce a writer where the backup will be collected.
func Collection(ctx context.Context, client *mongo.Client, opts Options) error {
	if err := opts.flushData(ctx, client); err != nil {
		return errors.WithStack(err)
	}

	idxes, err := opts.getIndexData(ctx, client)
	if err != nil {
		return errors.WithStack(err)
	}

	if err := opts.writeIndexData(ctx, idxes); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (opts *Options) getQueryOpts() *options.FindOptions {
	qopts := options.Find()
	if opts.Sort != nil {
		qopts.SetSort(opts.Sort)
	}
	if opts.Limit > 0 {
		qopts.SetLimit(opts.Limit)
	}
	if opts.Query == nil {
		opts.Query = struct{}{}
	}
	return qopts
}

func (opts *Options) getCursor(ctx context.Context, client *mongo.Client) (*mongo.Cursor, error) {
	cursor, err := client.Database(opts.NS.DB).Collection(opts.NS.Collection).Find(ctx, opts.Query, opts.getQueryOpts())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return cursor, nil
}

func (opts *Options) flushData(ctx context.Context, client *mongo.Client) error {
	if opts.IndexesOnly {
		return nil
	}

	catcher := grip.NewCatcher()

	cursor, err := opts.getCursor(ctx, client)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() { catcher.Add(cursor.Close(ctx)) }()

	target, err := opts.Target(ctx, filepath.Join(opts.NS.DB, opts.NS.Collection)+".bson")
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() { catcher.Add(target.Close()) }()

	for cursor.Next(ctx) {
		_, err := target.Write(cursor.Current)
		if err != nil {
			catcher.Add(err)
			break
		}
	}

	catcher.Add(cursor.Err())
	return catcher.Resolve()
}

func (opts *Options) getIndexData(ctx context.Context, client *mongo.Client) (*birch.Array, error) {
	catcher := grip.NewCatcher()

	cursor, err := client.Database(opts.NS.DB).Collection(opts.NS.Collection).Indexes().List(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer func() { catcher.Add(cursor.Close(ctx)) }()

	indexes := birch.NewArray()
	for cursor.Next(ctx) {
		doc, err := birch.DC.ReaderErr(birch.Reader(cursor.Current))
		if err != nil {
			catcher.Add(err)
			break
		}
		indexes.Append(birch.VC.Document(doc))
	}

	catcher.Add(cursor.Err())

	return indexes, catcher.Resolve()
}

func (opts *Options) writeIndexData(ctx context.Context, indexes *birch.Array) error {
	out, err := birch.DC.Elements(
		birch.EC.SubDocument("options", birch.DC.New()),
		birch.EC.Array("indexes", indexes),
		birch.EC.String("uuid", ""),
	).MarshalJSON()
	if err != nil {
		return errors.WithStack(err)
	}

	target, err := opts.Target(ctx, filepath.Join(opts.NS.DB, opts.NS.Collection)+".metadata.json")
	if err != nil {
		return errors.WithStack(err)
	}

	catcher := grip.NewCatcher()
	defer func() { catcher.Add(target.Close()) }()
	_, err = target.Write(out)
	if err != nil {
		catcher.Add(err)
		return catcher.Resolve()
	}

	return catcher.Resolve()
}
