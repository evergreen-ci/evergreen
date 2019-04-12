package db

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// WrapClient provides the anser database Session interface, which is
// modeled on mgo's interface but based on new mongo.Client fundamentals.
func WrapClient(ctx context.Context, client *mongo.Client) Session {
	return &sessionWrapper{
		ctx:     ctx,
		client:  client,
		catcher: grip.NewCatcher(),
	}
}

type sessionWrapper struct {
	ctx     context.Context
	client  *mongo.Client
	catcher grip.Catcher
	isClone bool
}

func (s *sessionWrapper) Clone() Session                   { s.isClone = true; return s }
func (s *sessionWrapper) Copy() Session                    { s.isClone = true; return s }
func (s *sessionWrapper) Error() error                     { return s.catcher.Resolve() }
func (s *sessionWrapper) SetSocketTimeout(d time.Duration) {}

func (s *sessionWrapper) DB(name string) Database {
	return &databaseWrapper{
		ctx:      s.ctx,
		database: s.client.Database(name),
	}
}

func (s *sessionWrapper) Close() {
	if s.isClone {
		return
	}
	s.catcher.Add(s.client.Disconnect(s.ctx))
}

type databaseWrapper struct {
	ctx      context.Context
	database *mongo.Database
}

func (d *databaseWrapper) Name() string        { return d.database.Name() }
func (d *databaseWrapper) DropDatabase() error { return errors.WithStack(d.database.Drop(d.ctx)) }
func (d *databaseWrapper) C(coll string) Collection {
	return &collectionWrapper{
		ctx:  d.ctx,
		coll: d.database.Collection(coll),
	}
}

type collectionWrapper struct {
	ctx  context.Context
	coll *mongo.Collection
}

func (c *collectionWrapper) DropCollection() error { return errors.WithStack(c.coll.Drop(c.ctx)) }

func (c *collectionWrapper) Pipe(p interface{}) Results {
	cursor, err := c.coll.Aggregate(c.ctx, p, options.Aggregate().SetAllowDiskUse(true))

	return &resultsWrapper{
		err:    err,
		cursor: cursor,
		ctx:    c.ctx,
	}
}

func (c *collectionWrapper) Find(q interface{}) Query {
	return &queryWrapper{
		ctx:    c.ctx,
		coll:   c.coll,
		filter: q,
	}
}

func (c *collectionWrapper) FindId(id interface{}) Query {
	return &queryWrapper{
		ctx:    c.ctx,
		coll:   c.coll,
		filter: bson.D{{Key: "_id", Value: id}},
	}
}

func (c *collectionWrapper) Count() (int, error) {
	num, err := c.coll.CountDocuments(c.ctx, struct{}{})
	return int(num), errors.WithStack(err)
}

func (c *collectionWrapper) Insert(d ...interface{}) error {
	var err error
	if len(d) == 1 {
		_, err = c.coll.InsertOne(c.ctx, d[0])
	} else {
		_, err = c.coll.InsertMany(c.ctx, d)
	}
	return errors.WithStack(err)
}

func (c *collectionWrapper) Remove(q interface{}) error {
	_, err := c.coll.DeleteOne(c.ctx, q)
	return errors.WithStack(err)
}

func (c *collectionWrapper) RemoveId(id interface{}) error {
	_, err := c.coll.DeleteOne(c.ctx, bson.D{{Key: "_id", Value: id}})
	return errors.WithStack(err)
}

func (c *collectionWrapper) RemoveAll(q interface{}) (*ChangeInfo, error) {
	res, err := c.coll.DeleteMany(c.ctx, q)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &ChangeInfo{Removed: int(res.DeletedCount)}, nil
}

func (c *collectionWrapper) Upsert(q interface{}, u interface{}) (*ChangeInfo, error) {
	doc, err := transformDocument(u)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var res *mongo.UpdateResult
	if hasDollarKey(doc) {
		res, err = c.coll.UpdateOne(c.ctx, q, u, options.Update().SetUpsert(true))
	} else {
		res, err = c.coll.ReplaceOne(c.ctx, q, u, options.Replace().SetUpsert(true))
	}

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &ChangeInfo{Updated: int(res.UpsertedCount) + int(res.ModifiedCount), UpsertedId: res.UpsertedID}, nil
}

func (c *collectionWrapper) UpsertId(id interface{}, u interface{}) (*ChangeInfo, error) {
	doc, err := transformDocument(u)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	query := bson.D{{Key: "_id", Value: id}}

	var res *mongo.UpdateResult
	if hasDollarKey(doc) {
		res, err = c.coll.UpdateOne(c.ctx, query, u, options.Update().SetUpsert(true))
	} else {
		res, err = c.coll.ReplaceOne(c.ctx, query, u, options.Replace().SetUpsert(true))
	}

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &ChangeInfo{Updated: int(res.UpsertedCount) + int(res.ModifiedCount), UpsertedId: res.UpsertedID}, nil
}

func (c *collectionWrapper) Update(q interface{}, u interface{}) error {
	doc, err := transformDocument(u)
	if err != nil {
		return errors.WithStack(err)
	}

	var res *mongo.UpdateResult
	if hasDollarKey(doc) {
		res, err = c.coll.UpdateOne(c.ctx, q, u)
	} else {
		res, err = c.coll.ReplaceOne(c.ctx, q, u)
	}

	if err != nil {
		return errors.WithStack(err)
	}

	if res.MatchedCount == 0 {
		return errors.WithStack(errNotFound)
	}

	return nil
}
func (c *collectionWrapper) UpdateId(q interface{}, u interface{}) error {
	doc, err := transformDocument(u)
	if err != nil {
		return errors.WithStack(err)
	}

	query := bson.D{{"_id", q}}

	var res *mongo.UpdateResult
	if hasDollarKey(doc) {
		res, err = c.coll.UpdateOne(c.ctx, query, doc)
	} else {
		res, err = c.coll.ReplaceOne(c.ctx, query, doc)
	}

	if err != nil {
		return errors.WithStack(err)
	}

	if res.MatchedCount == 0 {
		return errors.WithStack(errNotFound)
	}

	return nil
}

func (c *collectionWrapper) UpdateAll(q interface{}, u interface{}) (*ChangeInfo, error) {
	res, err := c.coll.UpdateMany(c.ctx, q, u)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &ChangeInfo{Updated: int(res.ModifiedCount)}, nil
}

func (c *collectionWrapper) Bulk() Bulk {
	return &bulkWrapper{
		ctx:  c.ctx,
		coll: c.coll,
	}
}

type bulkWrapper struct {
	ctx         context.Context
	models      []mongo.WriteModel
	coll        *mongo.Collection
	isUnordered bool
}

func (b *bulkWrapper) Insert(docs ...interface{}) {
	for _, d := range docs {
		b.models = append(b.models, &mongo.InsertOneModel{
			Document: d,
		})
	}
}

func (b *bulkWrapper) Remove(docs ...interface{}) {
	for _, d := range docs {
		b.models = append(b.models, &mongo.DeleteOneModel{
			Filter: d,
		})
	}
}

func (b *bulkWrapper) RemoveAll(docs ...interface{}) {
	for _, d := range docs {
		b.models = append(b.models, &mongo.DeleteManyModel{
			Filter: d,
		})
	}
}

func (b *bulkWrapper) Update(pairs ...interface{}) {
	if len(pairs)%2 != 0 {
		panic("bulk update requires an even number of parameters")
	}

	for i := 0; i < len(pairs); i += 2 {
		selector := pairs[i]
		if selector == nil {
			selector = bson.D{}
		}
		b.models = append(b.models, &mongo.UpdateOneModel{
			Filter: selector,
			Update: pairs[i+1],
		})
	}
}

func (b *bulkWrapper) UpdateAll(pairs ...interface{}) {
	if len(pairs)%2 != 0 {
		panic("bulk update requires an even number of parameters")
	}

	for i := 0; i < len(pairs); i += 2 {
		selector := pairs[i]
		if selector == nil {
			selector = bson.D{}
		}
		b.models = append(b.models, &mongo.UpdateManyModel{
			Filter: selector,
			Update: pairs[i+1],
		})
	}
}

func (b *bulkWrapper) Upsert(pairs ...interface{}) {
	if len(pairs)%2 != 0 {
		panic("Bulk.Update requires an even number of parameters")
	}

	for i := 0; i < len(pairs); i += 2 {
		selector := pairs[i]
		if selector == nil {
			selector = bson.D{}
		}
		b.models = append(b.models, (&mongo.UpdateOneModel{
			Filter: selector,
			Update: pairs[i+1],
		}).SetUpsert(true))
	}
}

func (b *bulkWrapper) Unordered() { b.isUnordered = true }

func (b *bulkWrapper) Run() (*BulkResult, error) {
	res, err := b.coll.BulkWrite(b.ctx, b.models, options.BulkWrite().SetOrdered(!b.isUnordered))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &BulkResult{Matched: int(res.MatchedCount), Modified: int(res.ModifiedCount)}, nil
}

type resultsWrapper struct {
	ctx    context.Context
	cursor *mongo.Cursor
	err    error
}

func (r *resultsWrapper) All(result interface{}) error {
	if r.err != nil {
		return errors.WithStack(r.err)
	}

	return errors.WithStack(ResolveCursorAll(r.ctx, r.cursor, result))
}

func (r *resultsWrapper) One(result interface{}) error {
	if r.err != nil {
		return errors.WithStack(r.err)
	}

	return errors.WithStack(ResolveCursorOne(r.ctx, r.cursor, result))
}

func (r *resultsWrapper) Iter() Iterator {
	catcher := grip.NewCatcher()
	catcher.Add(r.err)
	return &iteratorWrapper{
		ctx:     r.ctx,
		cursor:  r.cursor,
		catcher: catcher,
	}
}

type iteratorWrapper struct {
	ctx        context.Context
	cursor     *mongo.Cursor
	catcher    grip.Catcher
	errChecked bool
}

func (iter *iteratorWrapper) Close() error { return errors.WithStack(iter.cursor.Close(iter.ctx)) }

func (iter *iteratorWrapper) Err() error {
	if !iter.errChecked {
		iter.catcher.Add(iter.cursor.Err())
		iter.errChecked = true
	}

	return iter.catcher.Resolve()
}

func (iter *iteratorWrapper) Next(val interface{}) bool {
	if !iter.cursor.Next(iter.ctx) {
		return false
	}

	iter.catcher.Add(iter.cursor.Decode(val))
	return true
}

type queryWrapper struct {
	ctx        context.Context
	coll       *mongo.Collection
	cursor     *mongo.Cursor
	filter     interface{}
	projection interface{}
	limit      int
	skip       int
	sort       []string
}

func (q *queryWrapper) Limit(l int) Query             { q.limit = l; return q }
func (q *queryWrapper) Select(proj interface{}) Query { q.projection = proj; return q }
func (q *queryWrapper) Sort(keys ...string) Query     { q.sort = append(q.sort, keys...); return q }
func (q *queryWrapper) Skip(s int) Query              { q.skip = s; return q }
func (q *queryWrapper) Count() (int, error) {
	v, err := q.coll.CountDocuments(q.ctx, q.filter)
	return int(v), errors.WithStack(err)
}

func (q *queryWrapper) Apply(ch Change, result interface{}) (*ChangeInfo, error) {
	if ch.Remove && ch.Update != nil {
		return nil, errors.New("cannot delete and update in a findAndUpdate")
	}

	var res *mongo.SingleResult
	out := &ChangeInfo{}
	if ch.Remove {
		if ch.ReturnNew {
			return nil, errors.New("cannot return new with a delete operation")
		}

		opts := options.FindOneAndDelete().SetProjection(q.projection)
		if q.sort != nil {
			opts.SetSort(getSort(q.sort))
		}

		res = q.coll.FindOneAndDelete(q.ctx, q.filter, opts)

		out.Removed++
	} else if ch.Update != nil {
		doc, err := transformDocument(ch.Update)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		if hasDollarKey(doc) {

			opts := options.FindOneAndUpdate().SetProjection(q.projection).SetUpsert(ch.Upsert).SetReturnDocument(getFindAndModifyReturn(ch.ReturnNew))
			if q.sort != nil {
				opts.SetSort(getSort(q.sort))
			}
			res = q.coll.FindOneAndUpdate(q.ctx, q.filter, ch.Update, opts)
		} else {
			opts := options.FindOneAndReplace().SetProjection(q.projection).SetUpsert(ch.Upsert).SetReturnDocument(getFindAndModifyReturn(ch.ReturnNew))
			if q.sort != nil {
				opts.SetSort(getSort(q.sort))
			}
			res = q.coll.FindOneAndReplace(q.ctx, q.filter, ch.Update, opts)
		}
		out.Updated++
	} else {
		return nil, errors.New("invalid change defined")
	}

	if err := res.Err(); err != nil {
		return nil, errors.WithStack(err)
	}

	if err := res.Decode(result); err != nil {
		return nil, errors.WithStack(err)
	}

	return out, nil

}

func (q *queryWrapper) exec() error {
	if q.cursor != nil {
		return nil
	}

	if q.filter == nil {
		q.filter = struct{}{}
	}

	opts := options.Find()
	if q.projection != nil {
		opts.SetProjection(q.projection)
	}
	if q.sort != nil {
		opts.SetSort(getSort(q.sort))
	}
	if q.limit > 0 {
		opts.SetLimit(int64(q.limit))
	}
	if q.skip > 0 {
		opts.SetSkip(int64(q.skip))
	}

	var err error

	q.cursor, err = q.coll.Find(q.ctx, q.filter, opts)

	return errors.WithStack(err)
}

func (q *queryWrapper) All(result interface{}) error {
	if err := q.exec(); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(ResolveCursorAll(q.ctx, q.cursor, result))
}

func (q *queryWrapper) One(result interface{}) error {
	q.limit = 1

	if err := q.exec(); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(ResolveCursorOne(q.ctx, q.cursor, result))
}

func (q *queryWrapper) Iter() Iterator {
	if q.cursor != nil {
		return &iteratorWrapper{
			ctx:     q.ctx,
			cursor:  q.cursor,
			catcher: grip.NewCatcher(),
		}
	}

	catcher := grip.NewCatcher()
	catcher.Add(q.exec())
	return &iteratorWrapper{
		ctx:     q.ctx,
		cursor:  q.cursor,
		catcher: catcher,
	}
}

// ResolveCursorAll uses legacy mgo code to resolve a new driver's
// cursor into an array.
func ResolveCursorAll(ctx context.Context, iter *mongo.Cursor, result interface{}) error {
	if iter == nil {
		return errors.New("cannot resolve nil cursor")
	}
	resultv := reflect.ValueOf(result)
	if resultv.Kind() != reflect.Ptr || resultv.Elem().Kind() != reflect.Slice {
		return errors.Errorf("result argument must be a slice address '%T'", result)
	}
	slicev := resultv.Elem()
	slicev = slicev.Slice(0, slicev.Cap())
	elemt := slicev.Type().Elem()
	catcher := grip.NewCatcher()
	i := 0
	for {
		if slicev.Len() == i {
			elemp := reflect.New(elemt)
			if !iter.Next(ctx) {
				if i == 0 {
					return nil
				}
				break
			}

			catcher.Add(iter.Decode(elemp.Interface()))
			slicev = reflect.Append(slicev, elemp.Elem())
			slicev = slicev.Slice(0, slicev.Cap())
		} else {
			if !iter.Next(ctx) {
				break
			}
			catcher.Add(iter.Decode(slicev.Index(i).Addr().Interface()))
		}
		i++
	}
	resultv.Elem().Set(slicev.Slice(0, i))
	catcher.Add(iter.Err())
	catcher.Add(iter.Close(ctx))
	return catcher.Resolve()
}

// ResolveCursorOne decodes the first result in a cursor, for use in
// "FindOne" cases.
func ResolveCursorOne(ctx context.Context, iter *mongo.Cursor, result interface{}) error {
	if iter == nil {
		return errors.New("cannot resolve result from cursor")
	}

	catcher := grip.NewCatcher()
	defer func() {
		catcher.Add(iter.Close(ctx))
	}()

	if !iter.Next(ctx) {
		return errors.WithStack(errNotFound)
	}

	catcher.Add(iter.Decode(result))
	catcher.Add(iter.Err())

	return errors.Wrap(catcher.Resolve(), "problem resolving result")
}

func transformDocument(val interface{}) (bsonx.Doc, error) {
	if val == nil {
		return nil, errors.WithStack(mongo.ErrNilDocument)
	}

	if doc, ok := val.(bsonx.Doc); ok {
		return doc.Copy(), nil
	}

	if bs, ok := val.([]byte); ok {
		// Slight optimization so we'll just use MarshalBSON and not go through the codec machinery.
		val = bson.Raw(bs)
	}

	// TODO(skriptble): Use a pool of these instead.
	buf := make([]byte, 0, 256)
	b, err := bson.MarshalAppendWithRegistry(bson.DefaultRegistry, buf[:0], val)
	if err != nil {
		return nil, mongo.MarshalError{Value: val, Err: err}
	}

	return bsonx.ReadDoc(b)
}

func hasDollarKey(doc bsonx.Doc) bool {
	if len(doc) == 0 {
		return false
	}
	if !strings.HasPrefix(doc[0].Key, "$") {
		return false
	}

	return true
}

func getSort(keys []string) bson.D {
	if len(keys) == 0 {
		return nil
	}

	sort := bson.D{}

	for _, k := range keys {
		if strings.HasPrefix(k, "-") {
			sort = append(sort, bson.E{Key: k[1:], Value: -1})
		} else if strings.HasPrefix(k, "+") {
			sort = append(sort, bson.E{Key: k[1:], Value: 1})
		} else {
			sort = append(sort, bson.E{Key: k, Value: 1})
		}
	}

	return sort
}

func getFindAndModifyReturn(returnNew bool) options.ReturnDocument {
	if returnNew {
		return options.After
	}
	return options.Before
}
