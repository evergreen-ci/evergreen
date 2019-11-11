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

type BufferedInsertSuite struct {
	dbname  string
	factory func(context.Context) Session
	uuid    uuid.UUID
	db      Database
	ctx     context.Context
	bi      *anserBufInsertsImpl
	suite.Suite
}

func TestLegacyBufferedInsertSuite(t *testing.T) {
	s := new(BufferedInsertSuite)

	var err error
	s.uuid, err = uuid.NewV4()
	require.NoError(t, err)

	s.dbname = fmt.Sprintf("anser_%s", s.uuid)
	var session *mgo.Session
	session, err = mgo.DialWithTimeout("mongodb://localhost:27017", time.Second)
	require.NoError(t, err)
	defer session.Close()
	s.factory = func(_ context.Context) Session {
		return WrapSession(session)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.db = s.factory(ctx).DB(s.dbname)
	suite.Run(t, s)
}

func TestBufferedInsertSuite(t *testing.T) {
	s := new(BufferedInsertSuite)
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
	defer func() { require.NoError(t, client.Disconnect(ctx)) }()
	s.db = s.factory(ctx).DB(s.dbname)
	suite.Run(t, s)
}

func (s *BufferedInsertSuite) SetupTest() {
	s.bi = &anserBufInsertsImpl{
		docs:    make(chan interface{}, 1),
		err:     make(chan error),
		flusher: make(chan chan error),
		closer:  make(chan chan error),
		db:      s.factory(s.ctx).DB(s.dbname),
	}
}

func (s *BufferedInsertSuite) takedown(collection string) {
	s.NoError(s.db.C(collection).DropCollection())
}

func (s *BufferedInsertSuite) kickstart(ctx context.Context, collection string) {
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

func (s *BufferedInsertSuite) TearDownSuite() {
	s.Require().NoError(s.db.DropDatabase())
}

func (s *BufferedInsertSuite) TestAppendErrorsForNilDocuments() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	coll := s.uuid.String()
	s.kickstart(ctx, coll)
	defer s.takedown(coll)

	for i := 0; i < 100; i++ {
		if i%2 == 0 {
			s.NoError(s.bi.Append(Document{"a": i}))
			continue
		}
		s.Error(s.bi.Append(nil))
	}
	s.NoError(s.bi.Flush())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(50, num)
}

func (s *BufferedInsertSuite) TestBasicInsertsBufferDoesNotFlushOnCancel() {
	ctx, cancel := context.WithCancel(context.Background())
	coll := s.uuid.String()
	s.kickstart(ctx, coll)
	defer s.takedown(coll)

	for i := 0; i < 10000; i++ {
		s.NoError(s.bi.Append(Document{"a": i}))
	}
	cancel()

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.NotEqual(10000, num)
}

func (s *BufferedInsertSuite) TestBufferFlushes() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coll := s.uuid.String()
	s.kickstart(ctx, coll)
	defer s.takedown(coll)

	for i := 0; i < 100; i++ {
		s.NoError(s.bi.Append(Document{"a": i}))
	}
	s.NoError(s.bi.Flush())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(100, num)
}

func (s *BufferedInsertSuite) TestCloserFlushes() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coll := s.uuid.String()
	s.bi.opts.Count = 100000
	s.bi.opts.Duration = time.Hour
	s.kickstart(ctx, coll)
	defer s.takedown(coll)

	jobSize := 1000
	for i := 0; i < jobSize; i++ {
		s.NoError(s.bi.Append(Document{"a": i}))
	}
	s.NoError(s.bi.Close())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(jobSize, num)

}

func (s *BufferedInsertSuite) TestShouldNoopUsually() {
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

func (s *BufferedInsertSuite) closeWithPendingDocuments() {
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
		s.NoError(s.bi.Append(Document{"a": i}))
	}

	s.NoError(s.bi.Close())
	s.NoError(catcher.Resolve())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(jobSize, num)
}

func (s *BufferedInsertSuite) flushWithPendingDocuments() {
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
		s.NoError(s.bi.Append(Document{"a": i}))
	}

	for i := 0; i < jobSize; i++ {
		s.NoError(s.bi.Flush())
	}

	s.NoError(catcher.Resolve())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(jobSize, num)
}

func (s *BufferedInsertSuite) TestCloseWithPending00() { s.closeWithPendingDocuments() }
func (s *BufferedInsertSuite) TestCloseWithPending01() { s.closeWithPendingDocuments() }
func (s *BufferedInsertSuite) TestCloseWithPending02() { s.closeWithPendingDocuments() }
func (s *BufferedInsertSuite) TestCloseWithPending03() { s.closeWithPendingDocuments() }
func (s *BufferedInsertSuite) TestCloseWithPending04() { s.closeWithPendingDocuments() }

func (s *BufferedInsertSuite) TestFlushWithPending00() { s.flushWithPendingDocuments() }
func (s *BufferedInsertSuite) TestFlushWithPending01() { s.flushWithPendingDocuments() }
func (s *BufferedInsertSuite) TestFlushWithPending02() { s.flushWithPendingDocuments() }
func (s *BufferedInsertSuite) TestFlushWithPending03() { s.flushWithPendingDocuments() }
func (s *BufferedInsertSuite) TestFlushWithPending04() { s.flushWithPendingDocuments() }

func (s *BufferedInsertSuite) TestFlushBeforeTimerExpires() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coll := s.uuid.String()
	s.bi.opts.Count = 100000
	s.bi.opts.Duration = time.Nanosecond
	s.kickstart(ctx, coll)
	defer s.takedown(coll)

	jobSize := 100
	for i := 0; i < jobSize; i++ {
		s.NoError(s.bi.Append(Document{"a": i}))
	}

	s.NoError(s.bi.Flush())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(jobSize, num)
}

type BufWriteOptsSuite struct {
	opts BufferedWriteOptions
	suite.Suite
}

func TestBufWriteOptsSuite(t *testing.T) {
	suite.Run(t, new(BufWriteOptsSuite))
}

func (s *BufWriteOptsSuite) SetupTest() {
	s.opts = BufferedWriteOptions{}
}

func (s *BufWriteOptsSuite) TestZeroValueIsNotValid() {
	s.Zero(s.opts)
	s.Error(s.opts.Validate())
}

func (s *BufWriteOptsSuite) makeOptsValid() {
	s.opts.Collection = "foo"
	s.opts.Count = 100
	s.opts.Duration = time.Minute

	s.Require().NoError(s.opts.Validate())
}

func (s *BufWriteOptsSuite) TestMissingCollectionIsNotValid() {
	s.makeOptsValid()
	s.opts.Collection = ""
	s.Error(s.opts.Validate())
}

func (s *BufWriteOptsSuite) TestCountMustBeAtLeast2() {
	s.makeOptsValid()
	s.opts.Count = 0
	s.Error(s.opts.Validate())

	s.opts.Count = 1
	s.Error(s.opts.Validate())

	s.opts.Count = -10
	s.Error(s.opts.Validate())

	s.opts.Count = 2
	s.NoError(s.opts.Validate())

}

func (s *BufWriteOptsSuite) TestDurationMustBeReasonable() {
	s.makeOptsValid()
	s.opts.Duration = 0
	s.Error(s.opts.Validate())
	s.opts.Duration = time.Nanosecond
	s.Error(s.opts.Validate())

	s.opts.Duration = time.Millisecond
	s.Error(s.opts.Validate())

	s.opts.Duration = 5 * time.Millisecond
	s.Error(s.opts.Validate())

	s.opts.Duration = 10 * time.Millisecond
	s.NoError(s.opts.Validate())

}

func (s *BufWriteOptsSuite) TestDatabaseIsNotRequired() {
	s.makeOptsValid()
	s.NoError(s.opts.Validate())
	s.Equal("", s.opts.DB)
}

func TestBufferedInsertConstructors(t *testing.T) {
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
	bi, err = NewBufferedInserter(ctx, nil, opts)
	assert.Error(err)
	assert.Nil(bi)

	// test valid options construct non-nil object
	opts = BufferedWriteOptions{
		Collection: "foo",
		Count:      10,
		Duration:   time.Second,
	}
	bi, err = NewBufferedInserter(ctx, nil, opts)
	assert.NoError(err)
	assert.NotNil(bi)

	// from session should error without database names
	bi, err = NewBufferedSessionInserter(ctx, &mgo.Session{}, opts)
	assert.Error(err)
	assert.Nil(bi)

	opts.DB = "bar"
	bi, err = NewBufferedSessionInserter(ctx, nil, opts)
	assert.Error(err)
	assert.Nil(bi)

	bi, err = NewBufferedSessionInserter(ctx, &mgo.Session{}, opts)
	assert.NoError(err)
	assert.NotNil(bi)
}
