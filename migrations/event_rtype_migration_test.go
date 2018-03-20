package migrations

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser"
	anserdb "github.com/mongodb/anser/db"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

const allLogCollection = "event_log"

type eventRTypeMigrationSuite struct {
	events []anserdb.Document

	migrationSuite
}

func TestEventRTypeMigration(t *testing.T) {
	suite.Run(t, &eventRTypeMigrationSuite{})
}

func (s *eventRTypeMigrationSuite) SetupTest() {
	loc, err := time.LoadLocation("UTC")
	s.NoError(err)
	date, err := time.ParseInLocation(time.RFC3339Nano, "2017-06-20T18:07:24.991Z", loc)
	s.NoError(err)

	c, err := s.session.DB(s.database).C(allLogCollection).RemoveAll(anserdb.Document{})
	s.Require().NoError(err)
	s.Require().NotNil(c)

	s.events = []anserdb.Document{
		anserdb.Document{
			"_id":    bson.ObjectIdHex("5949645c9acd9604fdd202d7"),
			"ts":     date,
			"r_id":   "macos.example.com",
			"e_type": "HOST_TASK_FINISHED",
			"data": anserdb.Document{
				"r_type": "HOST",
				"t_id":   "mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44",
				"t_st":   "success",
			},
		},
		anserdb.Document{
			"_id":          bson.ObjectIdHex("5949645c9acd9604fdd202d8"),
			"ts":           date,
			"r_id":         "macos.example.com",
			"e_type":       "HOST_TASK_FINISHED",
			"r_type":       "HOST",
			"processed_at": time.Time{}.Add(-time.Hour),
			"data": anserdb.Document{
				"t_id": "mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44",
				"t_st": "failed",
			},
		},
		anserdb.Document{
			"_id":    bson.ObjectIdHex("5949645c9acd9604fdd202d9"),
			"ts":     date,
			"r_id":   "macos.example.com",
			"e_type": "SOMETHING_AWESOME",
			"data": anserdb.Document{
				"r_type": "SOMETHINGELSE",
				"other":  "data",
			},
		},
	}
	for _, e := range s.events {
		s.NoError(db.Insert(allLogCollection, e))
	}
}

func (s *eventRTypeMigrationSuite) TestMigration() {
	gen, err := makeEventRTypeMigration(allLogCollection)(anser.GetEnvironment(), s.database, 50)
	s.Require().NoError(err)
	gen.Run()
	s.Require().NoError(gen.Error())

	i := 0
	for j := range gen.Jobs() {
		i++
		j.Run()
		s.NoError(j.Error())
	}
	s.Equal(2, i)

	out := []bson.M{}
	s.Require().NoError(db.FindAllQ(allLogCollection, db.Q{}, &out))
	s.Len(out, 3)

	for _, e := range out {
		eventData, ok := e["data"]
		s.True(ok)

		eventDataBSON, ok := eventData.(bson.M)
		s.True(ok)

		id, ok := e["_id"].(bson.ObjectId)
		s.True(ok)

		if id.Hex() == "5949645c9acd9604fdd202d7" {
			s.Equal("HOST", e["r_type"])
			s.Equal("HOST_TASK_FINISHED", e["e_type"])

			s.Equal("mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44", eventDataBSON["t_id"])
			s.Equal("success", eventDataBSON["t_st"])

		} else if id.Hex() == "5949645c9acd9604fdd202d8" {
			s.Equal("HOST", e["r_type"])
			s.Equal("HOST_TASK_FINISHED", e["e_type"])

			s.Equal("mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44", eventDataBSON["t_id"])
			s.Equal("failed", eventDataBSON["t_st"])
			s.True(e["processed_at"].(time.Time).Equal(time.Time{}.Add(-time.Hour)))

		} else if id.Hex() == "5949645c9acd9604fdd202d9" {
			s.Equal("SOMETHINGELSE", e["r_type"])
			s.Equal("SOMETHING_AWESOME", e["e_type"])

			s.Equal("data", eventDataBSON["other"])

		} else {
			s.T().Error("unknown object id")
		}

		resourceTypeBSON, ok := eventDataBSON["r_type"]
		s.False(ok)
		s.Nil(resourceTypeBSON)
		s.NotPanics(func() {
			s.NotNil(e["processed_at"])
			s.False(e["processed_at"].(time.Time).Equal(time.Time{}))
		})
	}
}
