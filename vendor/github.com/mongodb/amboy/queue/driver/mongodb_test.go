package driver

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2"
)

type MongoDBDriverSuite struct {
	driver      *MongoDB
	require     *require.Assertions
	collections []string
	uri         string
	dbName      string
	suite.Suite
}

func TestMongoDBDriverSuite(t *testing.T) {
	suite.Run(t, new(MongoDBDriverSuite))
}

func (s *MongoDBDriverSuite) SetupSuite() {
	s.uri = "mongodb://localhost:27017"
	s.dbName = "amboy"
	s.require = s.Require()
}

func (s *MongoDBDriverSuite) SetupTest() {
	name := uuid.NewV4().String()
	s.driver = NewMongoDB(name, DefaultMongoDBOptions())
	s.driver.dbName = s.dbName
	s.collections = append(s.collections, name+".jobs", name+".locks")
}

func (s *MongoDBDriverSuite) TearDownSuite() {
	session, err := mgo.Dial(s.uri)
	s.NoError(err)
	defer session.Close()

	db := session.DB(s.dbName)
	for _, coll := range s.collections {
		grip.CatchWarning(db.C(coll).DropCollection())
	}
}

func (s *MongoDBDriverSuite) TearDownTest() {
	s.Equal(s.dbName, s.driver.dbName)
}

func (s *MongoDBDriverSuite) TestOpenCloseAffectState() {
	ctx := context.Background()

	s.Nil(s.driver.canceler)
	s.NoError(s.driver.Open(ctx))
	s.NotNil(s.driver.canceler)

	s.driver.Close()
	s.NotNil(s.driver.canceler)
	// sleep to give it a chance to switch to close the connection
	time.Sleep(10 * time.Millisecond)
}

func (s *MongoDBDriverSuite) TestNextIsBlocking() {
	ctx := context.Background()
	s.NoError(s.driver.Open(ctx))
	var cancel context.CancelFunc

	startAt := time.Now()
	ctx, cancel = context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	s.Nil(s.driver.Next(ctx))
	s.True(time.Since(startAt) >= 2*time.Second)
}
