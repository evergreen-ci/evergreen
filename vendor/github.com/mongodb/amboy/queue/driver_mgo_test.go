package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2"
)

type MongoDBDriverSuite struct {
	driver      *mgoDriver
	session     *mgo.Session
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
	s.dbName = "amboy_test"

	var err error
	s.session, err = mgo.Dial(s.uri)
	s.Require().NoError(err)
}

func (s *MongoDBDriverSuite) SetupTest() {
	name := uuid.NewV4().String()
	s.driver = NewMgoDriver(name, DefaultMongoDBOptions()).(*mgoDriver)
	s.driver.opts.DB = s.dbName
	s.collections = append(s.collections, addJobsSuffix(name), name+".locks")

	db := s.session.DB(s.dbName)
	count, err := db.C(s.collections[0]).Count()
	s.Require().NoError(err)
	s.Equal(count, 0)
}

func (s *MongoDBDriverSuite) TearDownSuite() {
	db := s.session.DB(s.dbName)
	for _, coll := range s.collections {
		grip.Warning(db.C(coll).DropCollection())
	}
	s.session.Close()
}

func (s *MongoDBDriverSuite) TearDownTest() {
	s.Equal(s.dbName, s.driver.opts.DB)
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

func (s *MongoDBDriverSuite) TestSaveStatusSavesTimeInfo() {
	ctx := context.Background()
	s.NoError(s.driver.Open(ctx))
	j := newMockJob()
	jobID := fmt.Sprintf("%d.%s.%d", 1, "mock-job", job.GetNumber())
	j.SetID(jobID)
	s.NoError(s.driver.Put(ctx, j))
	j2, err := s.driver.Get(ctx, jobID)
	s.Require().NoError(err)
	s.Zero(j2.TimeInfo().Start)
	now := time.Now().Round(time.Millisecond)
	ti := amboy.JobTimeInfo{Start: now}
	j.UpdateTimeInfo(ti)
	s.NoError(s.driver.SaveStatus(ctx, j, j.Status()))
	j3, err := s.driver.Get(ctx, jobID)
	s.NoError(err)
	s.Equal(now, j3.TimeInfo().Start)
}
