package migrations

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type bsonObjectIDToStringSuite struct {
	docs       []db.Document
	found      map[string]bool
	collection string
	migrationSuite
}

func TestSubscriptionIDToString(t *testing.T) {
	suite.Run(t, &bsonObjectIDToStringSuite{collection: event.SubscriptionsCollection})
}

func (s *bsonObjectIDToStringSuite) SetupTest() {
	c := s.session.DB(s.database).C(s.collection)
	_, err := c.RemoveAll(db.Document{})
	s.NoError(err)

	s.docs = []db.Document{
		{
			"_id": bson.ObjectIdHex("aaaaaaaaaaaaaaaaaaaaaaa0"),
		},
		{
			"_id": bson.ObjectIdHex("aaaaaaaaaaaaaaaaaaaaaaa1"),
		},
		{
			"_id": bson.ObjectIdHex("aaaaaaaaaaaaaaaaaaaaaaa2"),
		},
		{
			"_id": bson.ObjectIdHex("aaaaaaaaaaaaaaaaaaaaaaa3"),
		},
		{
			"_id": bson.ObjectIdHex("aaaaaaaaaaaaaaaaaaaaaaa4"),
		},
		{
			"_id": "s0",
		},
		{
			"_id": "s1",
		},
		{
			"_id": "s2",
		},
		{
			"_id": "s3",
		},
		{
			"_id": "s4",
		},
	}

	s.found = map[string]bool{}
	for i := range s.docs {
		s.NoError(c.Insert(s.docs[i]))
		switch v := s.docs[i]["_id"].(type) {
		case string:
			s.found[v] = false

		case bson.ObjectId:
			s.found[v.Hex()] = false
		}
	}
}

func (s *bsonObjectIDToStringSuite) TestMigration() {
	args := migrationGeneratorFactoryOptions{
		db:    s.database,
		limit: 50,
		id:    "subscription-id-to-string",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gen, err := makeBSONObjectIDToStringGenerator(event.SubscriptionsCollection)(anser.GetEnvironment(), args)
	s.Require().NoError(err)
	gen.Run(ctx)
	s.Require().NoError(gen.Error())

	i := 0
	for j := range gen.Jobs() {
		i++
		j.Run(ctx)
		s.NoError(j.Error())
	}

	s.Equal(5, i)

	q := s.session.DB(s.database).C(s.collection).Find(db.Document{
		"_id": db.Document{
			"$type": "string",
		},
	})

	out := []struct {
		ID string `bson:"_id"`
	}{}
	s.NoError(q.All(&out))
	s.Len(out, 10)

	for i := range out {
		_, ok := s.found[out[i].ID]
		s.True(ok)
		s.found[out[i].ID] = true
	}
	for k, v := range s.found {
		s.True(v, k)
	}

}
