package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser"
	adb "github.com/mongodb/anser/db"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

type eventRTypeMigrationSuite struct {
	events []adb.Document

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

	c, err := s.session.DB(s.database).C(allLogCollection).RemoveAll(adb.Document{})
	s.Require().NoError(err)
	s.Require().NotNil(c)

	s.events = []adb.Document{
		adb.Document{
			"_id":    mgobson.ObjectIdHex("5949645c9acd9604fdd202d7"),
			"ts":     date,
			"r_id":   "macos.example.com",
			"e_type": "HOST_TASK_FINISHED",
			"data": adb.Document{
				"r_type": "HOST",
				"t_id":   "mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44",
				"t_st":   "success",
			},
		},
		adb.Document{
			"_id":          mgobson.ObjectIdHex("5949645c9acd9604fdd202d8"),
			"ts":           date,
			"r_id":         "macos.example.com",
			"e_type":       "HOST_TASK_FINISHED",
			"r_type":       "HOST",
			"processed_at": time.Time{}.Add(-time.Hour),
			"data": adb.Document{
				"t_id": "mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44",
				"t_st": "failed",
			},
		},
		adb.Document{
			"_id":    mgobson.ObjectIdHex("5949645c9acd9604fdd202d9"),
			"ts":     date,
			"r_id":   "macos.example.com",
			"e_type": "SOMETHING_AWESOME",
			"data": adb.Document{
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
	args := migrationGeneratorFactoryOptions{
		db:    s.database,
		limit: 50,
		id:    "migration-event-rtype-to-root-alllogs",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gen, err := eventRTypeMigration(anser.GetEnvironment(), args)
	s.Require().NoError(err)
	gen.Run(ctx)
	s.Require().NoError(gen.Error())

	i := 0
	for j := range gen.Jobs() {
		i++
		j.Run(ctx)
		s.NoError(j.Error())
	}
	s.Equal(2, i)

	out := []bson.M{}
	s.Require().NoError(db.FindAllQ(allLogCollection, db.Q{}, &out))
	s.Len(out, 3)
	location, err := time.LoadLocation("UTC")
	s.NoError(err)
	expectedTime, err := time.ParseInLocation(time.RFC3339, migrationTime, location)
	s.NoError(err)
	s.NotZero(expectedTime)

	for _, e := range out {
		eventData, ok := e["data"]
		s.True(ok)

		eventDataBSON, ok := eventData.(bson.M)
		s.True(ok)

		id, ok := e["_id"].(mgobson.ObjectId)
		s.True(ok)

		var t time.Time
		if id.Hex() == "5949645c9acd9604fdd202d7" {
			s.Equal("HOST", e["r_type"])
			s.Equal("HOST_TASK_FINISHED", e["e_type"])

			s.Equal("mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44", eventDataBSON["t_id"])
			s.Equal("success", eventDataBSON["t_st"])

			t, ok = e["processed_at"].(time.Time)
			s.True(ok)
			s.True(expectedTime.Equal(t))

		} else if id.Hex() == "5949645c9acd9604fdd202d8" {
			s.Equal("HOST", e["r_type"])
			s.Equal("HOST_TASK_FINISHED", e["e_type"])
			s.Equal("mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44", eventDataBSON["t_id"])
			s.Equal("failed", eventDataBSON["t_st"])

			t, ok = e["processed_at"].(time.Time)
			s.True(ok)
			s.True(t.Equal(time.Time{}.Add(-time.Hour)))

		} else if id.Hex() == "5949645c9acd9604fdd202d9" {
			s.Equal("SOMETHINGELSE", e["r_type"])
			s.Equal("SOMETHING_AWESOME", e["e_type"])
			s.Equal("data", eventDataBSON["other"])

			t, ok = e["processed_at"].(time.Time)
			s.True(ok)
			s.True(expectedTime.Equal(t))

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
