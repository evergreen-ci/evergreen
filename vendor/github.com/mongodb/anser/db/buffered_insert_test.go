package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	mgo "gopkg.in/mgo.v2"
)

type BufferedInsertSuite struct {
	dbname  string
	session *mgo.Session
	uuid    uuid.UUID
	db      Database
	bi      *anserBufInsertsImpl
	suite.Suite
}

func TestBufferedInsertSuite(t *testing.T) {
	suite.Run(t, new(BufferedInsertSuite))
}

func (s *BufferedInsertSuite) SetupSuite() {
	var err error
	s.uuid, err = uuid.NewV4()
	s.Require().NoError(err)
	s.dbname = fmt.Sprintf("anser_%s", s.uuid)
	s.session, err = mgo.Dial("mongodb://localhost:27017")
	s.Require().NoError(err)
	s.db = WrapSession(s.session).DB(s.dbname)
}

func (s *BufferedInsertSuite) SetupTest() {
	s.bi = &anserBufInsertsImpl{
		docs:    make(chan interface{}, 1),
		err:     make(chan error),
		flusher: make(chan chan error),
		closer:  make(chan chan error),
		db:      WrapSession(s.session).DB(s.dbname),
	}
}

func (s *BufferedInsertSuite) takedown(collection string) {
	s.NoError(s.session.DB(s.dbname).C(collection).DropCollection())
}

func (s *BufferedInsertSuite) kickstart(ctx context.Context, collection string) {
	ctx, s.bi.cancel = context.WithCancel(ctx)
	s.bi.opts.Collection = collection
	s.bi.opts.DB = s.dbname

	if s.bi.opts.Count == 0 {
		s.bi.opts.Count = 10
	}

	if s.bi.opts.Duration == 0 {
		s.bi.opts.Duration = 10 * time.Millisecond
	}

	go s.bi.start(ctx)
}

func (s *BufferedInsertSuite) TearDownSuite() {
	s.Require().NoError(s.session.DB(s.dbname).DropDatabase())
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
		s.bi.Append(Document{"a": i})
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
		s.bi.Append(Document{"a": i})
	}
	s.NoError(s.bi.Close())

	num, err := s.db.C(coll).Count()
	s.NoError(err)
	s.Equal(jobSize, num)

}

func (s *BufferedInsertSuite) TestShouldNoopUsusally() {
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

type BufInsertOptsSuite struct {
	opts BufferedInsertOptions
	suite.Suite
}

func TestBufInsertOptsSuite(t *testing.T) {
	suite.Run(t, new(BufInsertOptsSuite))
}

func (s *BufInsertOptsSuite) SetupTest() {
	s.opts = BufferedInsertOptions{}
}

func (s *BufInsertOptsSuite) TestZeroValueIsNotValid() {
	s.Zero(s.opts)
	s.Error(s.opts.Validate())
}

func (s *BufInsertOptsSuite) makeOptsValid() {
	s.opts.Collection = "foo"
	s.opts.Count = 100
	s.opts.Duration = time.Minute

	s.Require().NoError(s.opts.Validate())
}

func (s *BufInsertOptsSuite) TestMissingCollectionIsNotValid() {
	s.makeOptsValid()
	s.opts.Collection = ""
	s.Error(s.opts.Validate())
}

func (s *BufInsertOptsSuite) TestCountMustBeAtLeast2() {
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

func (s *BufInsertOptsSuite) TestDurationMustBeReasonable() {
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

func (s *BufInsertOptsSuite) TestDatabaseIsNotRequired() {
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
		bi   BufferedInserter
		opts BufferedInsertOptions
	)

	assert.Nil(bi)
	assert.Zero(opts)

	// invalid options propagate error
	bi, err = NewBufferedInserter(ctx, nil, opts)
	assert.Error(err)
	assert.Nil(bi)

	// test valid options construct non-nil object
	opts = BufferedInsertOptions{
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
