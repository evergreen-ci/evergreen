package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/grip"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	mgo "gopkg.in/mgo.v2"
)

type BufferedUpsertSuite struct {
	dbname  string
	factory func(context.Context) Session
	ctx     context.Context
	uuid    uuid.UUID
	db      Database
	bi      *anserBufUpsertImpl
	suite.Suite
}

func TestLegacyBufferedUpsertSuite(t *testing.T) {
	s := new(BufferedUpsertSuite)

	var err error
	s.uuid, err = uuid.NewV4()
	require.NoError(t, err)

	s.dbname = fmt.Sprintf("anser_%s", s.uuid)
	var session *mgo.Session
	session, err = mgo.DialWithTimeout("mongodb://localhost:27017", time.Second)
	require.NoError(t, err)
	s.factory = func(_ context.Context) Session {
		return WrapSession(session)
	}
	s.db = s.factory(nil).DB(s.dbname)

	suite.Run(t, s)
}

func TestBufferedUpsertSuite(t *testing.T) {
	s := new(BufferedUpsertSuite)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	s.uuid, err = uuid.NewV4()
	require.NoError(t, err)

	s.dbname = fmt.Sprintf("anser_%s", s.uuid)
	var client *mongo.Client
	client, err = mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err)

	connCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	err = client.Connect(connCtx)
	require.NoError(t, err)

	s.factory = func(ctx context.Context) Session {
		return WrapClient(ctx, client)
	}
	defer client.Disconnect(ctx)
	s.db = s.factory(ctx).DB(s.dbname)
	suite.Run(t, s)
}

func (s *BufferedUpsertSuite) SetupTest() {
	s.bi = &anserBufUpsertImpl{
		upserts: make(chan upsertOp, 1),
		err:     make(chan error),
		flusher: make(chan chan error),
		closer:  make(chan chan error),
		db:      s.factory(s.ctx).DB(s.dbname),
	}
}

func (s *BufferedUpsertSuite) takedown(collection string) {
	s.NoError(s.db.C(collection).DropCollection())
}

func (s *BufferedUpsertSuite) kickstart(ctx context.Context, collection string) {
	ctx, s.bi.cancel = context.WithCancel(ctx)
	s.bi.opts.Collection = collection
	s.bi.opts.DB = s.dbname
	s.bi.db = s.factory(s.ctx).DB(s.dbname)

	if s.bi.opts.Count == 0 {
		s.bi.opts.Count = 10
	}

	if s.bi.opts.Duration == 0 {
		s.bi.opts.Duration = 100 * time.Millisecond
	}

	go s.bi.start(ctx)
}

func (s *BufferedUpsertSuite) TearDownSuite() {
	s.Require().NoError(s.db.DropDatabase())
}

func (s *BufferedUpsertSuite) TestAppendErrorsForNilDocuments() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	coll := s.uuid.String()
	s.kickstart(ctx, coll)
	defer s.takedown(coll)

	for i := 0; i < 100; i++ {
		if i%2 == 0 {
			s.NoError(s.bi.Append(Document{"_id": i + 1, "a": i}))
			continue
		}
		s.Error(s.bi.Append(nil))
	}
	s.NoError(s.bi.Flush())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(50, num)
}

func (s *BufferedUpsertSuite) TestBasicUpsertsBufferDoesNotFlushOnCancel() {
	ctx, cancel := context.WithCancel(context.Background())
	coll := s.uuid.String()
	s.kickstart(ctx, coll)
	defer s.takedown(coll)

	for i := 0; i < 1000; i++ {
		s.NoError(s.bi.Append(Document{"_id": i + 1, "a": i}))
	}
	cancel()

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.NotEqual(1000, num)
}

func (s *BufferedUpsertSuite) TestBufferFlushes() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coll := s.uuid.String()
	s.kickstart(ctx, coll)
	defer s.takedown(coll)

	for i := 0; i < 100; i++ {
		s.bi.Append(Document{"_id": i + 1, "a": i})
	}
	s.NoError(s.bi.Flush())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(100, num)
}

func (s *BufferedUpsertSuite) TestCloserFlushes() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coll := s.uuid.String()
	s.bi.opts.Count = 100000
	s.bi.opts.Duration = time.Hour
	s.kickstart(ctx, coll)
	defer s.takedown(coll)

	jobSize := 1000
	for i := 0; i < jobSize; i++ {
		s.bi.Append(Document{"_id": i + 1, "a": i})
	}
	s.NoError(s.bi.Close())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(jobSize, num)
}

func (s *BufferedUpsertSuite) TestShouldNoopUsusally() {
	for i := 0; i < 100; i++ {
		s.NoError(s.bi.Close())
	}

	// now we try with a configured sender
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coll := s.uuid.String()
	s.kickstart(ctx, coll)
	// no teardown because nothing happens
	// defer s.takedown(coll)
	for i := 0; i < 100; i++ {
		s.NoError(s.bi.Close())
	}
	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(0, num)
}

func (s *BufferedUpsertSuite) closeWithPendingDocuments() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coll := s.uuid.String()
	s.bi.opts.Count = 100000
	s.bi.opts.Duration = time.Hour
	s.kickstart(ctx, coll)
	defer s.takedown(coll)

	catcher := grip.NewBasicCatcher()
	jobSize := 10
	for i := 0; i < jobSize; i++ {
		s.bi.Append(Document{"_id": 1 + i, "a": i})
	}

	s.NoError(s.bi.Close())
	s.NoError(catcher.Resolve())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(jobSize, num)
}

func (s *BufferedUpsertSuite) flushWithPendingDocuments() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coll := s.uuid.String()
	s.bi.opts.Count = 100000
	s.bi.opts.Duration = time.Hour
	s.kickstart(ctx, coll)
	defer s.takedown(coll)

	catcher := grip.NewBasicCatcher()
	jobSize := 10
	for i := 0; i < jobSize; i++ {
		s.bi.Append(Document{"_id": 1 + i, "a": i})
	}

	for i := 0; i < jobSize; i++ {
		s.NoError(s.bi.Flush())
	}

	s.NoError(catcher.Resolve())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(jobSize, num)
}

func (s *BufferedUpsertSuite) TestCloseWithPending00() { s.closeWithPendingDocuments() }
func (s *BufferedUpsertSuite) TestCloseWithPending01() { s.closeWithPendingDocuments() }
func (s *BufferedUpsertSuite) TestCloseWithPending02() { s.closeWithPendingDocuments() }
func (s *BufferedUpsertSuite) TestCloseWithPending03() { s.closeWithPendingDocuments() }
func (s *BufferedUpsertSuite) TestCloseWithPending04() { s.closeWithPendingDocuments() }

func (s *BufferedUpsertSuite) TestFlushWithPending00() { s.flushWithPendingDocuments() }
func (s *BufferedUpsertSuite) TestFlushWithPending01() { s.flushWithPendingDocuments() }
func (s *BufferedUpsertSuite) TestFlushWithPending02() { s.flushWithPendingDocuments() }
func (s *BufferedUpsertSuite) TestFlushWithPending03() { s.flushWithPendingDocuments() }
func (s *BufferedUpsertSuite) TestFlushWithPending04() { s.flushWithPendingDocuments() }

func (s *BufferedUpsertSuite) TestFlushBeforeTimerExpires() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coll := s.uuid.String()
	s.bi.opts.Count = 100000
	s.bi.opts.Duration = time.Nanosecond
	s.kickstart(ctx, coll)
	defer s.takedown(coll)

	jobSize := 100
	for i := 0; i < jobSize; i++ {
		s.bi.Append(Document{"_id": 2 + i, "a": i})
	}

	s.NoError(s.bi.Flush())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(jobSize, num)
}

func TestBufferedUpsertConstructors(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		err  error
		bi   BufferedWriter
		opts BufferedWriteOptions
	)

	assert.Nil(bi)
	assert.Zero(opts)

	// invalid options propagate error
	bi, err = NewBufferedUpsertByID(ctx, nil, opts)
	assert.Error(err)
	assert.Nil(bi)

	// test valid options construct non-nil object
	opts = BufferedWriteOptions{
		Collection: "foo",
		Count:      10,
		Duration:   time.Second,
	}
	bi, err = NewBufferedUpsertByID(ctx, nil, opts)
	assert.Error(err)
	assert.Nil(bi)

	// from session should error without database names
	bi, err = NewBufferedSessionUpsertByID(ctx, &mgo.Session{}, opts)
	assert.Error(err)
	assert.Nil(bi)

	opts.DB = "bar"
	bi, err = NewBufferedSessionUpsertByID(ctx, nil, opts)
	assert.Error(err)
	assert.Nil(bi)

	bi, err = NewBufferedSessionUpsertByID(ctx, &mgo.Session{}, opts)
	assert.NoError(err)
	assert.NotNil(bi)
}
