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

func (s *eventSuite) TestMarshallAndUnarshallingStructsHaveSameTags() {
	realt := reflect.TypeOf(&Event{}).Elem()
	tempt := reflect.TypeOf(&unmarshalEvent{}).Elem()

	s.Require().Equal(realt.NumField(), tempt.NumField())
	for i := 0; i < realt.NumField(); i++ {
		s.True(reflect.DeepEqual(realt.Field(i).Tag, tempt.Field(i).Tag))
	}
}

func (s *eventSuite) TestTerribleUnmarshaller() {
	logger := NewDBEventLogger(AllLogCollection)
	for k, v := range eventRegistry {
		event := Event{
			ResourceId: "TEST1",
			EventType:  "TEST2",
			Timestamp:  time.Now().Round(time.Millisecond).Truncate(time.Millisecond),
			Data:       v(),
		}
		found, rTypeTag := findResourceTypeIn(event.Data)
		s.True(found)
		s.Equal(k, rTypeTag)
		if e, ok := event.Data.(*rawAdminEventData); ok {
			// sad violin is sad. bson.Raw cannot be empty, so
			// we set the Kind to 0x0A, which is a BSON null
			e.Changes.Before = bson.Raw{
				Kind: 0x0A,
			}
			e.Changes.After = bson.Raw{
				Kind: 0x0A,
			}
		}

		s.Equal("TEST1", event.ResourceId)
		s.Equal("TEST2", event.EventType)
		s.NoError(logger.LogEvent(event))
		fetchedEvents, err := Find(AllLogCollection, db.Query(bson.M{}))
		s.NoError(err)
		s.Require().Len(fetchedEvents, 1)
		s.True(fetchedEvents[0].Data.IsValid())
		s.IsType(v(), fetchedEvents[0].Data)
		s.NotNil(fetchedEvents[0].Data)
		s.NoError(db.ClearCollections(AllLogCollection))
	}
}

func (s *eventSuite) TestEventRegistry() {
	for k, _ := range eventRegistry {
		event := NewEventFromType(k)
		s.NotNil(event)
		found, rTypeTag := findResourceTypeIn(event)

		t := reflect.TypeOf(event)
		s.True(found, `'%s' does not have a bson:"r_type" tag`, t.String())
		s.Equal(k, rTypeTag, "'%s''s r_type does not match the registry key", t.String())

		// ensure all fields have bson and json tags
		elem := t.Elem()
		for i := 0; i < elem.NumField(); i++ {
			f := elem.Field(i)
			bsonTag := f.Tag.Get("bson")
			jsonTag := f.Tag.Get("json")
			s.NotEmpty(bsonTag, "struct %s: field '%s' must have bson tag", t.Name(), f.Name)
			s.NotEmpty(jsonTag, "struct %s: field '%s' must have json tag", t.Name(), f.Name)

		}
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
