package db

import (
	"context"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
)

// BufferedInserter provides a way to do application-buffered write operations
// to take advantage of the faster "vector" insert or bulk write api  when
// writing groups of documents into a single collection.
//
// Implementations differ in how they manage connections to the
// database, typically as well as providing different write
// operations. The interface
type BufferedWriter interface {
	// Adds a single document to the buffer, which will cause a
	// single insert after the configured duration
	// elapses. Implementations may do validation of the input,
	// and should generally return an error for nil input.
	Append(interface{}) error

	// Flush forces the buffered insert to write its current
	// buffer to the database regardless of the current time
	// remining. Implementations may reset timer.
	Flush() error

	// Returns any cached errors and flushes any pending documents
	// for insert.
	Close() error
}

// BufferedInsertOptions captures options used by any BufferedInserter
// implemenation.
type BufferedWriteOptions struct {
	DB         string
	Collection string
	Count      int
	Duration   time.Duration
}

// Validate returns an error if any of the required values are not set
// or have unreasonable values.
func (o BufferedWriteOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	// NOTE: db name is not required in all cases, implementations
	// should check on this separately.

	if o.Collection == "" {
		catcher.Add(errors.New("must specify a collection name"))
	}

	if o.Count < 2 {
		catcher.Add(errors.New("must specify a count of at least 2"))
	}

	if o.Duration < 10*time.Millisecond {
		catcher.Add(errors.New("must specify a duration of at least 10ms"))
	}

	return catcher.Resolve()
}

type anserBufInsertsImpl struct {
	opts    BufferedWriteOptions
	db      Database
	cancel  context.CancelFunc
	docs    chan interface{}
	flusher chan chan error
	closer  chan chan error
	err     chan error
}

// Constructs a buffered insert handler, wrapping an mgo-like
// interface for a database dervied from anser's database
// interface. You must specify the collection that the
// inserts target in the options. You can construct the same instance
// using an actual mgo session with the NewBufferedSession
func NewBufferedInserter(ctx context.Context, db Database, opts BufferedWriteOptions) (BufferedWriter, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "cannot construct buffered insert handler")
	}

	bi := &anserBufInsertsImpl{
		docs:    make(chan interface{}),
		err:     make(chan error),
		flusher: make(chan chan error),
		closer:  make(chan chan error),
		db:      db,
		opts:    opts,
	}
	ctx, bi.cancel = context.WithCancel(ctx)

	go bi.start(ctx)

	return bi, nil
}

func NewBufferedSessionInserter(ctx context.Context, s *mgo.Session, opts BufferedWriteOptions) (BufferedWriter, error) {
	if opts.DB == "" {
		return nil, errors.New("must specify a database name when constructing a buffered insert handler")
	}

	if s == nil {
		return nil, errors.New("must specify a non-nil database session")
	}

	session := WrapSession(s)

	return NewBufferedInserter(ctx, session.DB(opts.DB), opts)
}

func (bi *anserBufInsertsImpl) start(ctx context.Context) {
	timer := time.NewTimer(bi.opts.Duration)
	defer timer.Stop()
	buf := []interface{}{}
	catcher := grip.NewBasicCatcher()

bufferLoop:
	for {
		select {
		case <-ctx.Done():
			if len(buf) > 0 {
				catcher.Add(errors.Errorf("buffered insert has %d pending inserts", len(buf)))
			}

			bi.err <- catcher.Resolve()
			break bufferLoop
		case <-timer.C:
			if len(buf) > 0 {
				err := bi.db.C(bi.opts.Collection).Insert(buf...)
				catcher.Add(err)
				if err == nil {
					buf = []interface{}{}
				}
			}

			timer.Reset(bi.opts.Duration)
		case doc := <-bi.docs:
			buf = append(buf, doc)

			if len(buf) >= bi.opts.Count {
				err := bi.db.C(bi.opts.Collection).Insert(buf...)
				catcher.Add(err)
				if err == nil {
					buf = []interface{}{}
				}

				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(bi.opts.Duration)
			}
		case f := <-bi.flusher:
			select {
			case last := <-bi.docs:
				buf = append(buf, last)
			default:
			}

			if len(buf) > 0 {
				err := bi.db.C(bi.opts.Collection).Insert(buf...)
				catcher.Add(err)
				if err == nil {
					buf = []interface{}{}
				}
				f <- err
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(bi.opts.Duration)
			}

			close(f)
		case c := <-bi.closer:
			close(bi.docs)
			for last := range bi.docs {
				buf = append(buf, last)
			}

			if len(buf) > 0 {
				err := bi.db.C(bi.opts.Collection).Insert(buf...)
				catcher.Add(err)
			}
			c <- catcher.Resolve()
			close(c)
			bi.cancel = nil
			break bufferLoop
		}
	}

	close(bi.err)

}

func (bi *anserBufInsertsImpl) Close() error {
	if bi.cancel == nil {
		return nil
	}

	res := make(chan error)
	bi.closer <- res

	bi.cancel()
	bi.cancel = nil

	return <-res
}

func (bi *anserBufInsertsImpl) Append(doc interface{}) error {
	if doc == nil {
		return errors.New("cannot insert a nil document")
	}

	bi.docs <- doc
	return nil
}

func (bi *anserBufInsertsImpl) Flush() error {
	res := make(chan error)
	bi.flusher <- res
	return <-res
}
