package event

import (
	"encoding/json"
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
	realt := reflect.TypeOf(&EventLogEntry{}).Elem()
	tempt := reflect.TypeOf(&unmarshalEventLogEntry{}).Elem()

	s.Require().Equal(realt.NumField(), tempt.NumField())
	for i := 0; i < realt.NumField(); i++ {
		s.True(reflect.DeepEqual(realt.Field(i).Tag, tempt.Field(i).Tag))
	}
}

func (s *eventSuite) TestTerribleUnmarshaller() {
	logger := NewDBEventLogger(AllLogCollection)
	for k, v := range eventRegistry {
		event := EventLogEntry{
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

const expectedJSON = "{\"timestamp\":\"2017-06-20T18:07:24.991Z\",\"resource_id\":\"macos.example.com\",\"event_type\":\"HOST_TASK_FINISHED\",\"data\":{\"resource_type\":\"HOST\",\"task_id\":\"mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44\",\"task_status\":\"success\",\"successful\":false,\"duration\":0}}"

func (s *eventSuite) TestWithRealData() {
	loc, err := time.LoadLocation("UTC")
	s.NoError(err)
	date, err := time.ParseInLocation(time.RFC3339Nano, "2017-06-20T18:07:24.991Z", loc)
	s.NoError(err)
	s.Equal("2017-06-20T18:07:24.991Z", date.Format(time.RFC3339Nano))
	data := bson.M{
		"_id":    bson.ObjectIdHex("5949645c9acd9604fdd202d7"),
		"ts":     date,
		"r_id":   "macos.example.com",
		"e_type": "HOST_TASK_FINISHED",
		"data": bson.M{
			"r_type": "HOST",
			"t_id":   "mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44",
			"t_st":   "success",
		},
	}
	s.NoError(db.Insert(AllLogCollection, data))
	entries, err := Find(AllLogCollection, db.Query(bson.M{"_id": bson.ObjectIdHex("5949645c9acd9604fdd202d7")}))
	s.NoError(err)
	s.Len(entries, 1)
	s.NotPanics(func() {
		s.Equal("HOST_TASK_FINISHED", entries[0].EventType)
		s.Equal("macos.example.com", entries[0].ResourceId)

		eventData, ok := entries[0].Data.(*HostEventData)
		s.True(ok)

		s.Equal("HOST", eventData.ResourceType)
		s.Equal("mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44", eventData.TaskId)
		s.Equal("success", eventData.TaskStatus)

		// Verify that JSON unmarshals as expected
		entries[0].Timestamp = entries[0].Timestamp.In(loc)
		bytes, err := json.Marshal(&entries[0])
		s.NoError(err)
		s.Equal(expectedJSON, string(bytes))
	})
}

func (s *eventSuite) TestEventWithNilData() {
	logger := NewDBEventLogger(AllLogCollection)
	event := EventLogEntry{
		ResourceId: "TEST1",
		EventType:  "TEST2",
		Timestamp:  time.Now().Round(time.Millisecond).Truncate(time.Millisecond),
	}
	s.Errorf(logger.LogEvent(event), "event log entry cannot have nil Data")

	s.NotPanics(func() {
		s.NoError(db.Insert(AllLogCollection, event))
		fetchedEvents, err := Find(AllLogCollection, db.Query(bson.M{}))
		s.Error(err)
		s.Empty(fetchedEvents)
	})
}

func (s *eventSuite) TestEventRegistryItemsAreSane() {
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

			if _, ok := event.(*rawAdminEventData); !ok {
				s.NotEmpty(jsonTag, "struct %s: field '%s' must have json tag", t.Name(), f.Name)
				if bsonTag == resourceTypeKey {
					s.Equal("resource_type", jsonTag)
				}
			}
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
