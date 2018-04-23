package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser"
	anserdb "github.com/mongodb/anser/db"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type eventSetProcessedAtSuite struct {
	events []anserdb.Document
	left   time.Time
	right  time.Time

	migrationSuite
}

func TestEventSetProcessedAtMigration(t *testing.T) {
	suite.Run(t, &eventSetProcessedAtSuite{})
}

func (s *eventSetProcessedAtSuite) SetupTest() {
	const (
		idKey          = "_id"
		tsKey          = "ts"
		processedAtKey = "processed_at"
	)
	loc, err := time.LoadLocation("UTC")
	s.NoError(err)

	s.left = time.Now().In(loc).Truncate(0).Round(time.Millisecond).Add(-24 * time.Hour)
	s.right = time.Now().In(loc).Truncate(0).Round(time.Millisecond).Add(-12 * time.Hour)

	c, err := s.session.DB(s.database).C(allLogCollection).RemoveAll(anserdb.Document{})
	s.Require().NoError(err)
	s.Require().NotNil(c)

	s.events = []anserdb.Document{
		// within the time range, but already processed
		anserdb.Document{
			idKey:          bson.ObjectIdHex("5949645c9acd9604fdd202d7"),
			tsKey:          s.right.Add(-time.Hour),
			processedAtKey: s.right.Add(-time.Hour),
		},
		// too old
		anserdb.Document{
			idKey:          bson.ObjectIdHex("5949645c9acd9604fdd202d8"),
			tsKey:          s.left.Add(-time.Hour),
			processedAtKey: time.Time{},
		},
		// too new
		anserdb.Document{
			idKey:          bson.ObjectIdHex("5949645c9acd9604fdd202d9"),
			tsKey:          s.right.Add(time.Hour),
			processedAtKey: time.Time{},
		},
		// just right
		anserdb.Document{
			idKey:          bson.ObjectIdHex("5949645c9acd9604fdd202dA"),
			tsKey:          s.right.Add(-time.Hour),
			processedAtKey: time.Time{},
		},
		// right ts, but ineligible b/c field is missing
		anserdb.Document{
			idKey: bson.ObjectIdHex("5949645c9acd9604fdd202dB"),
			tsKey: s.right.Add(-time.Hour),
		},
	}
	for _, e := range s.events {
		s.NoError(db.Insert(allLogCollection, e))
	}
}

func (s *eventSetProcessedAtSuite) TestMigration() {
	args := migrationGeneratorFactoryOptions{
		db:    s.database,
		limit: 50,
		id:    migrationEventSetProcessedTime,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gen, err := makeEventSetProcesedTimeMigration(allLogCollection, s.left, s.right)(anser.GetEnvironment(), args)
	s.Require().NoError(err)
	gen.Run(ctx)
	s.Require().NoError(gen.Error())

	i := 0
	for j := range gen.Jobs() {
		i++
		j.Run(ctx)
		s.NoError(j.Error())
	}
	s.Equal(1, i)

	out := struct {
		ID          bson.ObjectId `bson:"_id"`
		ProcessedAt time.Time     `bson:"processed_at"`
	}{}
	s.Require().NoError(db.FindOneQ(allLogCollection, db.Query(bson.M{
		"_id": bson.ObjectIdHex("5949645c9acd9604fdd202dA"),
	}), &out))

	loc, _ := time.LoadLocation("UTC")
	bttf, err := time.ParseInLocation(time.RFC3339, unsubscribableTime, loc)
	s.NoError(err)
	s.True(bttf.Equal(out.ProcessedAt))

	s.Require().NoError(db.FindOneQ(allLogCollection, db.Query(bson.M{
		"_id": bson.ObjectIdHex("5949645c9acd9604fdd202dB"),
	}), &out))

	s.True(out.ProcessedAt.IsZero())
}

func (s *eventSetProcessedAtSuite) TestMigrationPicksUpEverythingWithZeroTime() {
	args := migrationGeneratorFactoryOptions{
		db:    s.database,
		limit: 50,
		id:    migrationEventSetProcessedTime,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gen, err := makeEventSetProcesedTimeMigration(allLogCollection, time.Time{}, time.Time{})(anser.GetEnvironment(), args)
	s.Require().NoError(err)
	gen.Run(ctx)
	s.Require().NoError(gen.Error())

	i := 0
	for j := range gen.Jobs() {
		i++
		j.Run(ctx)
		s.NoError(j.Error())
	}
	s.Equal(3, i)
}
