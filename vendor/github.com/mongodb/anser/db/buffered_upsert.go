package db

import (
	"context"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	mgo "gopkg.in/mgo.v2"
	mgobson "gopkg.in/mgo.v2/bson"
)

type anserBufUpsertImpl struct {
	opts    BufferedWriteOptions
	db      Database
	cancel  context.CancelFunc
	upserts chan upsertOp
	flusher chan chan error
	closer  chan chan error
	err     chan error
}

type upsertOp struct {
	query  interface{}
	record interface{}
}

func NewBufferedUpsertByID(ctx context.Context, db Database, opts BufferedWriteOptions) (BufferedWriter, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "cannot construct buffered insert handler")
	}

	if db == nil {
		return nil, errors.New("must not have a nil database")
	}

	bu := &anserBufUpsertImpl{
		upserts: make(chan upsertOp),
		err:     make(chan error),
		flusher: make(chan chan error),
		closer:  make(chan chan error),
		db:      db,
		opts:    opts,
	}

	ctx, bu.cancel = context.WithCancel(ctx)
	go bu.start(ctx)

	return bu, nil
}

func NewBufferedSessionUpsertByID(ctx context.Context, s *mgo.Session, opts BufferedWriteOptions) (BufferedWriter, error) {
	if opts.DB == "" {
		return nil, errors.New("must specify a database name when constructing a buffered upsert handler")
	}

	if s == nil {
		return nil, errors.New("must specify a non-nil database session")
	}

	session := WrapSession(s)

	return NewBufferedUpsertByID(ctx, session.DB(opts.DB), opts)

}

func (bu *anserBufUpsertImpl) start(ctx context.Context) {
	timer := time.NewTimer(bu.opts.Duration)
	defer timer.Stop()
	ops := 0
	bulk := bu.db.C(bu.opts.Collection).Bulk()
	catcher := grip.NewBasicCatcher()

bufferLoop:
	for {
		select {
		case <-ctx.Done():
			if ops > 0 {
				catcher.Add(errors.Errorf("buffered upsert has %d pending operations", ops))
			}

			bu.err <- catcher.Resolve()
			break bufferLoop
		case <-timer.C:
			if ops > 0 {
				_, err := bulk.Run()
				catcher.Add(err)
				if err == nil {
					bulk = bu.db.C(bu.opts.Collection).Bulk()
				}
				ops = 0
			}
			timer.Reset(bu.opts.Duration)
		case op := <-bu.upserts:
			bulk.Upsert(op.query, op.record)
			ops++

			if ops >= bu.opts.Count {
				_, err := bulk.Run()
				catcher.Add(err)
				ops = 0

				if err == nil {
					bulk = bu.db.C(bu.opts.Collection).Bulk()
				}
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(bu.opts.Duration)
			}
		case f := <-bu.flusher:
			select {
			case last := <-bu.upserts:
				bulk.Upsert(last.query, last.record)
				ops++
			default:
			}

			if ops > 0 {
				_, err := bulk.Run()
				catcher.Add(err)
				if err == nil {
					bulk = bu.db.C(bu.opts.Collection).Bulk()
				}
				ops = 0

				f <- err
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(bu.opts.Duration)
			}

			close(f)
		case c := <-bu.closer:
			close(bu.upserts)
			for last := range bu.upserts {
				bulk.Upsert(last.query, last.record)
			}

			if ops > 0 {
				_, err := bulk.Run()
				if err == nil {
					bulk = bu.db.C(bu.opts.Collection).Bulk() // nolint
				}
				catcher.Add(err)
			}

			c <- catcher.Resolve()
			close(c)
			bu.cancel = nil
			break bufferLoop
		}
	}

	close(bu.err)
}

func (bu *anserBufUpsertImpl) Close() error {
	if bu.cancel == nil {
		return nil
	}

	res := make(chan error)
	bu.closer <- res

	bu.cancel()
	bu.cancel = nil

	return <-res
}

func (bu *anserBufUpsertImpl) Append(doc interface{}) error {
	if doc == nil {
		return errors.New("cannot insert a nil document")
	}

	id, ok := getDocID(doc)
	if !ok {
		return errors.New("could not find document ID")
	}

	bu.upserts <- upsertOp{
		query:  Document{"_id": id},
		record: doc,
	}

	return nil
}

func getDocID(doc interface{}) (interface{}, bool) {
	switch d := doc.(type) {
	case mgobson.RawD:
		for _, raw := range d {
			if raw.Name == "_id" {
				return raw.Value, true
			}
		}
		return nil, false
	case bson.Raw:
		rv, err := d.LookupErr("_id")
		if err != nil {
			return nil, false
		}

		switch rv.Type {
		case bsontype.Double:
			return rv.DoubleOK()
		case bsontype.EmbeddedDocument:
			return rv.DocumentOK()
		case bsontype.String:
			return rv.StringValueOK()
		case bsontype.ObjectID:
			return rv.ObjectIDOK()
		case bsontype.Boolean:
			return rv.BooleanOK()
		case bsontype.DateTime:
			return rv.DateTimeOK()
		case bsontype.Int32:
			return rv.Int32OK()
		case bsontype.Int64:
			return rv.Int64OK()
		case bsontype.Decimal128:
			return rv.Decimal128OK()
		default:
			return nil, false
		}
	case map[string]interface{}:
		id, ok := d["_id"]
		return id, ok
	case Document:
		id, ok := d["_id"]
		return id, ok
	case bson.M:
		id, ok := d["_id"]
		return id, ok
	case mgobson.M:
		id, ok := d["_id"]
		return id, ok
	case map[string]string:
		id, ok := d["_id"]
		return id, ok
	default:
		return nil, false
	}
}

func (bu *anserBufUpsertImpl) Flush() error {
	res := make(chan error)
	bu.flusher <- res
	return <-res
}
