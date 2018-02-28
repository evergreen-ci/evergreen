package event

import (
	"reflect"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type eventSuite struct {
	suite.Suite
}

func TestEventSuite(t *testing.T) {
	suite.Run(t, &eventSuite{})
}

func (s *eventSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *eventSuite) SetupTest() {
	s.NoError(db.ClearCollections(AllLogCollection))
}

func (s *eventSuite) TestTerribleUnmarshaller() {
	logger := NewDBEventLogger(AllLogCollection)
	for k, v := range eventRegistry {
		event := Event{
			Timestamp: time.Now().Round(time.Millisecond).Truncate(time.Millisecond),
			Data:      DataWrapper{v()},
		}
		found, rTypeTag := findResourceTypeIn(event.Data.Data)
		s.True(found)
		s.Equal(k, rTypeTag)
		if e, ok := event.Data.Data.(*rawAdminEventData); ok {
			// sad violin is sad. bson.Raw cannot be empty type, so
			// we set the Kind to 0x0A, which is a BSON null
			e.Changes.Before = bson.Raw{
				Kind: 0x0A,
			}
			e.Changes.After = bson.Raw{
				Kind: 0x0A,
			}
		}

		s.NoError(logger.LogEvent(event))
		fetchedEvents, err := Find(AllLogCollection, db.Query(bson.M{}))
		s.NoError(err)
		s.Require().Len(fetchedEvents, 1)
		s.True(fetchedEvents[0].Data.IsValid())
		s.IsType(v(), fetchedEvents[0].Data.Data)
		s.NotNil(fetchedEvents[0].Data.Data)
		s.NoError(db.ClearCollections(AllLogCollection))
	}
}

func (s *eventSuite) TestEventRegistry() {
	for k, _ := range eventRegistry {
		event := NewEventFromType(k)
		s.NotNil(event)
		found, rTypeTag := findResourceTypeIn(event)
		s.True(found, `'%s' does not have a bson:"r_type" tag`, reflect.TypeOf(event).String())
		s.Equal(k, rTypeTag, "'%s''s r_type does not match the registry key", reflect.TypeOf(event).String())
	}
}

func (s *eventSuite) TestFindResourceTypeIn() {
	succeedStruct := struct {
		Type string `bson:"r_type"`
	}{Type: "something"}
	failStruct := struct {
		WrongThing string `bson:"type"`
	}{WrongThing: "wrong"}
	failStruct2 := struct {
		WrongThing int `bson:"r_type"`
	}{WrongThing: 1}

	found, data := findResourceTypeIn(&succeedStruct)
	s.True(found)
	s.Equal("something", data)

	found, data = findResourceTypeIn(&failStruct)
	s.False(found)
	s.Empty(data)

	found, data = findResourceTypeIn(&failStruct2)
	s.False(found)
	s.Empty(data)

	found, data = findResourceTypeIn(nil)
	s.False(found)
	s.Empty(data)
}

func findResourceTypeIn(t interface{}) (bool, string) {
	if t == nil {
		return false, ""
	}

	elem := reflect.TypeOf(t).Elem()

	for i := 0; i < elem.NumField(); i++ {
		f := elem.Field(i)
		bsonTag := f.Tag.Get("bson")
		if len(bsonTag) == 0 {
			continue
		}

		if bsonTag == "r_type" {
			if f.Type.String() != "string" {
				return false, ""
			}

			structData := reflect.ValueOf(t).Elem().Field(i).String()
			return true, structData
		}
	}

	return false, ""
}
